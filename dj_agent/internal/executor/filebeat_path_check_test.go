package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func pathCheckFragment(path, id, service, pattern string) filebeatConfigFile {
	return filebeatConfigFile{Path: path, Content: fmt.Sprintf(
		"- type: filestream\n  id: %s\n  enabled: true\n  paths: [%s]\n  fields:\n    service: %s\n    instance: %s\n    log_name: app.log\n",
		yamlQuote(id), yamlQuote(pattern), yamlQuote(service), yamlQuote(service+"-1"))}
}

func pathCheckParams(t *testing.T, files []filebeatConfigFile) map[string]any {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"files": files, "restart": false, "output": map[string]any{
		"url": "http://es:9200", "username": "", "password": "", "verify_tls": false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func TestFilebeatPathCheckRejectsPhysicalOverlap(t *testing.T) {
	for _, kind := range []string{"exact", "wildcard", "question", "character-class", "symlink", "hardlink", "directory-symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "error.log")
			if err := os.WriteFile(source, []byte("unchanged log"), 0o600); err != nil {
				t.Fatal(err)
			}
			second := source
			switch kind {
			case "wildcard":
				second = filepath.Join(dir, "*.log")
			case "question":
				second = filepath.Join(dir, "error.lo?")
			case "character-class":
				second = filepath.Join(dir, "[e]rror.log")
			case "symlink":
				second = filepath.Join(dir, "link.log")
				if err := os.Symlink(source, second); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				second = filepath.Join(dir, "hard.log")
				if err := os.Link(source, second); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink":
				link := filepath.Join(t.TempDir(), "alias")
				if err := os.Symlink(dir, link); err != nil {
					t.Fatal(err)
				}
				second = filepath.Join(link, "*.log")
			}
			files := []filebeatConfigFile{
				pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", source),
				pathCheckFragment("/etc/filebeat/inputs.d/b.yml", "b", "svc-b", second),
			}
			receipt, err := checkFilebeatPaths(context.Background(), files)
			if err == nil || receipt.Checked {
				t.Fatalf("physical overlap must fail: %+v %v", receipt, err)
			}
			for _, text := range []string{"overlap", "svc-a", "svc-b", source, second} {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("error missing %q: %v", text, err)
				}
			}
		})
	}
}

func TestFilebeatPathCheckDistinctAndFutureFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.log", "b.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := []filebeatConfigFile{
		pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", filepath.Join(dir, "a.log")),
		pathCheckFragment("/etc/filebeat/inputs.d/b.yml", "b", "svc-b", filepath.Join(dir, "b.log")),
		pathCheckFragment("/etc/filebeat/inputs.d/c.yml", "c", "svc-c", filepath.Join(dir, "future", "*.log")),
	}
	receipt, err := checkFilebeatPaths(context.Background(), files)
	if err != nil || !receipt.Checked || receipt.InputsChecked != 3 || receipt.MatchedFiles != 2 ||
		len(receipt.UnmatchedPatterns) != 1 || receipt.FilesDigest != filebeatFilesDigest(files) {
		t.Fatalf("distinct/future paths: %+v err=%v", receipt, err)
	}
	empty, err := checkFilebeatPaths(context.Background(), []filebeatConfigFile{})
	if err != nil || !empty.Checked || empty.InputsChecked != 0 || empty.MatchedFiles != 0 || empty.UnmatchedPatterns == nil {
		t.Fatalf("empty configuration must allow cleanup: %+v %v", empty, err)
	}
}

func TestFilebeatPathCheckRejectsIncompleteScans(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "absent"), broken); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"relative":         "logs/app.log",
		"recursive":        filepath.Join(dir, "**", "*.log"),
		"bad-pattern":      filepath.Join(dir, "[bad"),
		"unresolved-macro": filepath.Join(dir, "${APP_HOME}", "app.log"),
		"directory":        dir,
		"broken-symlink":   broken,
		"broken-parent":    filepath.Join(broken, "app.log"),
		"not-directory":    filepath.Join(dir, "file", "*.log"),
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for name, pattern := range cases {
		t.Run(name, func(t *testing.T) {
			receipt, err := checkFilebeatPaths(context.Background(), []filebeatConfigFile{
				pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", pattern),
			})
			if err == nil || receipt.Checked {
				t.Fatalf("incomplete scan must fail: %+v %v", receipt, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if receipt, err := checkFilebeatPaths(ctx, nil); err == nil || receipt.Checked {
		t.Fatal("canceled scan must fail even with empty inputs")
	}
	for _, budget := range []string{"directories", "entries"} {
		scanner := filebeatPathScanner{ctx: context.Background()}
		if budget == "directories" {
			scanner.dirs = maxFilebeatCheckDirs
		} else {
			scanner.entries = maxFilebeatCheckEntries
		}
		if err := scanner.walk(dir, []string{"*"}, func(string, os.FileInfo) error { return nil }); err == nil {
			t.Fatalf("%s budget exhaustion must not truncate successfully", budget)
		}
	}
}

func TestFilebeatPathCheckPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o700)
	receipt, err := checkFilebeatPaths(context.Background(), []filebeatConfigFile{
		pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", filepath.Join(locked, "*.log")),
	})
	if err == nil || receipt.Checked || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("permission failure must fail closed: %+v %v", receipt, err)
	}
}

func TestFilebeatPathPreflightBuiltinAndStrictFileParams(t *testing.T) {
	files := []filebeatConfigFile{
		pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", filepath.Join(t.TempDir(), "future.log")),
	}
	job := protocol.Job{JobID: "check", Type: protocol.TaskTypeCustom, Action: actionCheckFilebeatPaths, Params: pathCheckParams(t, files)}
	result, err := New(0).Run(context.Background(), job)
	if err != nil || result.Status != protocol.StatusSuccess || result.Data["path_check"] == nil {
		t.Fatalf("preflight action failed: %+v %v", result, err)
	}
	for name, params := range map[string]map[string]any{
		"missing-files":       {},
		"null-files":          {"files": nil},
		"bad-row":             {"files": []any{"invalid"}},
		"outside-managed-dir": pathCheckParams(t, []filebeatConfigFile{{Path: "/tmp/other.yml", Content: files[0].Content}}),
		"duplicate-file":      pathCheckParams(t, []filebeatConfigFile{files[0], files[0]}),
		"broken-yaml":         pathCheckParams(t, []filebeatConfigFile{{Path: files[0].Path, Content: "[:["}}),
	} {
		t.Run(name, func(t *testing.T) {
			job.Params = params
			result, err := New(0).Run(context.Background(), job)
			if err == nil && result.Status != protocol.StatusFailed {
				t.Fatalf("invalid files must return explicit failed action: %+v %v", result, err)
			}
		})
	}
}

func TestFilebeatApplyRechecksBeforeAnyMutation(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "filebeat.yml")
	inputs := filepath.Join(dir, "inputs.d")
	if err := os.Mkdir(inputs, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(inputs, "stale.yml")
	for _, path := range []string{main, stale} {
		if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	logFile := filepath.Join(dir, "app.log")
	if err := os.WriteFile(logFile, []byte("original log"), 0o600); err != nil {
		t.Fatal(err)
	}
	files := []filebeatConfigFile{
		pathCheckFragment(filepath.Join(inputs, "a.yml"), "a", "svc-a", logFile),
		pathCheckFragment(filepath.Join(inputs, "b.yml"), "b", "svc-b", filepath.Join(dir, "alias.log")),
	}
	// A link created after preflight must be caught inside the apply action.
	if receipt, err := checkFilebeatPaths(context.Background(), files); err != nil || !receipt.Checked {
		t.Fatalf("preflight: %+v %v", receipt, err)
	}
	if err := os.Link(logFile, filepath.Join(dir, "alias.log")); err != nil {
		t.Fatal(err)
	}
	result := New(0).applyFilebeatConfigAt(context.Background(), protocol.Job{
		JobID: "apply", Type: protocol.TaskTypeCustom, Action: actionApplyFilebeatConfig, Params: pathCheckParams(t, files),
	}, main, inputs)
	if result.Status != protocol.StatusFailed || !strings.Contains(result.Error, "overlap") {
		t.Fatalf("apply must reject new overlap: %+v", result)
	}
	for _, path := range []string{main, stale} {
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != "original" {
			t.Fatalf("failed check changed existing file %s: %s %v", path, raw, err)
		}
	}
	for _, file := range files {
		if _, err := os.Stat(file.Path); !os.IsNotExist(err) {
			t.Fatalf("failed check created input %s: %v", file.Path, err)
		}
	}
}

func TestFilebeatApplyWritesOutputAndInputsAfterCheck(t *testing.T) {
	dir := t.TempDir()
	main, inputs := filepath.Join(dir, "filebeat.yml"), filepath.Join(dir, "inputs.d")
	files := []filebeatConfigFile{
		pathCheckFragment(filepath.Join(inputs, "a.yml"), "a", "svc-a", filepath.Join(dir, "future.log")),
	}
	result := New(0).applyFilebeatConfigAt(context.Background(), protocol.Job{
		Params: pathCheckParams(t, files),
	}, main, inputs)
	if result.Status != protocol.StatusSuccess || result.Data["path_check"] == nil {
		t.Fatalf("checked apply failed: %+v", result)
	}
	raw, err := os.ReadFile(main)
	if err != nil || !strings.Contains(string(raw), "http://es:9200") {
		t.Fatalf("missing output config: %s %v", raw, err)
	}
	raw, err = os.ReadFile(files[0].Path)
	if err != nil || string(raw) != files[0].Content {
		t.Fatalf("missing inputs: %s %v", raw, err)
	}
	result = New(0).applyFilebeatConfigAt(context.Background(), protocol.Job{
		Params: pathCheckParams(t, []filebeatConfigFile{}),
	}, main, inputs)
	if result.Status != protocol.StatusSuccess {
		t.Fatalf("empty configuration cleanup failed: %+v", result)
	}
	if _, err := os.Stat(files[0].Path); !os.IsNotExist(err) {
		t.Fatalf("empty config must remove old managed fragment: %v", err)
	}
}

func TestFilebeatPathCheckFileAndInputLimits(t *testing.T) {
	dir := t.TempDir()
	for id := 0; id <= maxFilebeatCheckFiles; id++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%04d.log", id)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := []filebeatConfigFile{
		pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", filepath.Join(dir, "*.log")),
	}
	receipt, err := checkFilebeatPaths(context.Background(), files)
	if err == nil || receipt.Checked || !strings.Contains(err.Error(), "file limit exceeded") {
		t.Fatalf("file count must not truncate successfully: %+v %v", receipt, err)
	}
	var content strings.Builder
	future := filepath.Join(dir, "future")
	for id := 0; id <= maxFilebeatCheckInputs; id++ {
		content.WriteString(pathCheckFragment("", fmt.Sprintf("input-%d", id), "svc", future).Content)
	}
	receipt, err = checkFilebeatPaths(context.Background(), []filebeatConfigFile{
		{Path: "/etc/filebeat/inputs.d/a.yml", Content: content.String()},
	})
	if err == nil || receipt.Checked || !strings.Contains(err.Error(), "input limit exceeded") {
		t.Fatalf("input count must not truncate successfully: %+v %v", receipt, err)
	}
	_, err = decodeFilebeatFiles(pathCheckParams(t, []filebeatConfigFile{
		{Path: "/etc/filebeat/inputs.d/a.yml", Content: strings.Repeat("x", maxFilebeatConfigBytes+1)},
	}), filebeatInputsDir)
	if err == nil {
		t.Fatal("oversized config must not be accepted")
	}
}

func TestFilebeatPathCheckDoesNotConflictWithinOneInput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "app.log")
	if err := os.WriteFile(source, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, filepath.Join(dir, "alias.log")); err != nil {
		t.Fatal(err)
	}
	receipt, err := checkFilebeatPaths(context.Background(), []filebeatConfigFile{
		pathCheckFragment("/etc/filebeat/inputs.d/a.yml", "a", "svc-a", filepath.Join(dir, "*.log")),
	})
	if err != nil || !receipt.Checked || receipt.MatchedFiles != 2 {
		t.Fatalf("aliases inside a single input are not competing inputs: %+v %v", receipt, err)
	}
}

func TestFilebeatCheckedApplyValidatesAndRestartsOnce(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "commands.log")
	t.Setenv("DJ_TEST_COMMAND_LOG", logPath)
	t.Setenv("PATH", dir)
	for _, name := range []string{"filebeat", "systemctl"} {
		script := "#!/bin/sh\nprintf '" + name + "\\n' >> \"$DJ_TEST_COMMAND_LOG\"\n"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	main, inputs := filepath.Join(dir, "config", "filebeat.yml"), filepath.Join(dir, "config", "inputs.d")
	params := pathCheckParams(t, []filebeatConfigFile{
		pathCheckFragment(filepath.Join(inputs, "a.yml"), "a", "svc-a", filepath.Join(dir, "future.log")),
	})
	params["restart"] = true
	job := protocol.Job{Params: params}
	result := New(0).applyFilebeatConfigAt(context.Background(), job, main, inputs)
	if result.Status != protocol.StatusSuccess || result.Data["restarted"] != true {
		t.Fatalf("changed unified config must validate and restart: %+v", result)
	}
	result = New(0).applyFilebeatConfigAt(context.Background(), job, main, inputs)
	if result.Status != protocol.StatusSuccess || result.Data["restarted"] != false {
		t.Fatalf("unchanged config must not restart: %+v", result)
	}
	raw, err := os.ReadFile(logPath)
	if err != nil || string(raw) != "filebeat\nsystemctl\n" {
		t.Fatalf("expected one test-config then one restart, got %q err=%v", raw, err)
	}
}
