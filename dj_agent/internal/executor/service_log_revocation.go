package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
	"go.yaml.in/yaml/v3"
)

var filebeatConfigMu sync.Mutex

type logRevocation map[string]string

func loadLogRevocations(dir string) ([]logRevocation, error) {
	files, err := filepath.Glob(filepath.Join(dir, ".revoked-services", "*.json"))
	if err != nil {
		return nil, err
	}
	rows := make([]logRevocation, 0, len(files))
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		var row logRevocation
		if err = json.Unmarshal(raw, &row); err != nil {
			return nil, fmt.Errorf("invalid persisted log revocation %s: %w", name, err)
		}
		for _, key := range []string{"project", "business_system", "environment", "service"} {
			if row[key] == "" {
				return nil, fmt.Errorf("incomplete persisted log revocation %s", name)
			}
		}
		id := strings.TrimSuffix(filepath.Base(name), ".json")
		if value, err := strconv.ParseInt(id, 10, 64); err != nil || value < 1 {
			return nil, fmt.Errorf("invalid service ID in log revocation %s", name)
		}
		row["service_id"] = id
		rows = append(rows, row)
	}
	return rows, nil
}

func revokedManagedLogFile(name string, raw []byte, revocations []logRevocation) (bool, error) {
	for _, row := range revocations {
		revoked, err := revokedLogFragment(raw, []logRevocation{row})
		if err != nil {
			return false, err
		}
		pattern := `^.+__` + regexp.QuoteMeta(row["service"]) + `__` + regexp.QuoteMeta(row["service_id"]) + `__.+\.yml$`
		named := regexp.MustCompile(pattern).MatchString(filepath.Base(name))
		if revoked != named {
			return false, fmt.Errorf("service %s ownership disagrees with managed filename %s", row["service_id"], name)
		}
		if revoked {
			return true, nil
		}
	}
	return false, nil
}

// Compare structured fields rather than a filename substring: service codes and log
// names may themselves contain underscores or another service's numeric id.
func revokedLogFragment(raw []byte, revocations []logRevocation) (bool, error) {
	if len(revocations) == 0 {
		return false, nil
	}
	var inputs []struct {
		Fields map[string]string `yaml:"fields"`
	}
	if err := yaml.Unmarshal(raw, &inputs); err != nil {
		return false, fmt.Errorf("decode managed Filebeat input: %w", err)
	}
	matches := 0
	for _, input := range inputs {
		for _, row := range revocations {
			match := true
			for _, key := range []string{"project", "business_system", "environment", "service"} {
				match = match && input.Fields[key] == row[key]
			}
			if match {
				matches++
				break
			}
		}
	}
	if matches > 0 && matches != len(inputs) {
		return false, fmt.Errorf("managed input contains mixed service ownership; refusing whole-file revocation")
	}
	return matches > 0, nil
}

func revokeLogInputs(dir, id string, row logRevocation) error {
	// Both the immutable ID in the managed filename and the YAML ownership must agree.
	// Validate the complete removal set first: a bad fragment must not cause partial cleanup.
	owner := logRevocation{}
	for key, value := range row {
		owner[key] = value
	}
	owner["service_id"] = id
	files, err := filepath.Glob(filepath.Join(dir, "*.yml"))
	if err != nil {
		return err
	}
	var remove []string
	for _, name := range files {
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		revoked, err := revokedManagedLogFile(name, content, []logRevocation{owner})
		if err != nil {
			return fmt.Errorf("inspect %s: %w", name, err)
		}
		if revoked {
			remove = append(remove, name)
		}
	}
	markerDir := filepath.Join(dir, ".revoked-services")
	if err := os.MkdirAll(markerDir, 0o755); err != nil {
		return err
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	// Persist before removing inputs. A concurrent stale apply (serialized by the same
	// mutex), or a queued apply after agent restart, cannot recreate a revoked input.
	marker, err := os.OpenFile(filepath.Join(markerDir, id+".json"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = marker.Write(raw); err == nil {
		err = marker.Sync()
	}
	closeErr := marker.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = syncLogDirectory(markerDir); err != nil {
		return err
	}
	for _, name := range remove {
		if err = os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return syncLogDirectory(dir)
}

func syncLogDirectory(dir string) error {
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (e *Executor) revokeServiceLogs(_ context.Context, job protocol.Job) protocol.JobResult {
	filebeatConfigMu.Lock()
	defer filebeatConfigMu.Unlock()
	started := time.Now()
	id := strings.TrimSpace(fmt.Sprint(job.Params["service_id"]))
	if value, err := strconv.ParseInt(id, 10, 64); err != nil || value < 1 {
		return failedJobResult(job, started, fmt.Errorf("positive service_id required"))
	}
	row := logRevocation{}
	for _, key := range []string{"project", "business_system", "environment", "service"} {
		value, ok := job.Params[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return failedJobResult(job, started, fmt.Errorf("%s required", key))
		}
		row[key] = value
	}
	if err := revokeLogInputs(filebeatInputsDir, id, row); err != nil {
		return failedJobResult(job, started, err)
	}
	// Filebeat's enabled inputs.d reload closes removed harvesters. Never restart,
	// stop or uninstall Filebeat/the application, and never touch source log files.
	finished := time.Now()
	return protocol.JobResult{JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, StartedAt: started, FinishedAt: finished,
		CostMS: finished.Sub(started).Milliseconds()}
}
