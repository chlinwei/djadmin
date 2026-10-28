package executor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
	"go.yaml.in/yaml/v3"
)

const (
	maxFilebeatCheckInputs  = 2000
	maxFilebeatCheckFiles   = 5000
	maxFilebeatCheckDirs    = 2000
	maxFilebeatCheckEntries = 100000
	maxFilebeatConfigBytes  = 8 * 1024 * 1024
)

type filebeatConfigFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type filebeatPathReceipt struct {
	Version           int      `json:"version"`
	Checked           bool     `json:"checked"`
	FilesDigest       string   `json:"files_digest"`
	InputsChecked     int      `json:"inputs_checked"`
	MatchedFiles      int      `json:"matched_files"`
	UnmatchedPatterns []string `json:"unmatched_patterns"`
}

func decodeFilebeatFiles(params map[string]any, inputsDir string) ([]filebeatConfigFile, error) {
	raw, ok := params["files"].([]any)
	if !ok {
		return nil, fmt.Errorf("files must be an array")
	}
	files := make([]filebeatConfigFile, 0, len(raw))
	seen := map[string]bool{}
	total := 0
	for _, value := range raw {
		row, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Filebeat file entry")
		}
		path, pathOK := row["path"].(string)
		content, contentOK := row["content"].(string)
		if !pathOK || !contentOK || content == "" || filepath.Dir(path) != inputsDir ||
			filepath.Clean(path) != path || !strings.HasSuffix(path, ".yml") || seen[path] {
			return nil, fmt.Errorf("invalid or duplicate managed Filebeat path %q", path)
		}
		total += len(content)
		if total > maxFilebeatConfigBytes || len(files) >= maxFilebeatCheckInputs {
			return nil, fmt.Errorf("Filebeat config check size limit exceeded")
		}
		seen[path] = true
		files = append(files, filebeatConfigFile{Path: path, Content: content})
	}
	return files, nil
}

func filebeatFilesDigest(files []filebeatConfigFile) string {
	raw, _ := json.Marshal(files)
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

type filebeatCheckedFile struct {
	info  os.FileInfo
	path  string
	owner string
}

type filebeatPathScanner struct {
	ctx     context.Context
	files   []filebeatCheckedFile
	dirs    int
	entries int
	matches int
}

// filepath.Glob ignores directory I/O errors; a safety check must instead fail
// on unreadable directories and budget exhaustion, never return a truncated pass.
func (scanner *filebeatPathScanner) walk(base string, segments []string, visit func(string, os.FileInfo) error) error {
	if err := scanner.ctx.Err(); err != nil {
		return err
	}
	if len(segments) == 0 {
		info, err := os.Stat(base)
		if errors.Is(err, os.ErrNotExist) {
			// A future log is allowed; an existing dangling symlink is an unreadable target.
			if _, linkErr := os.Lstat(base); errors.Is(linkErr, os.ErrNotExist) {
				return nil
			}
		}
		if err != nil {
			return fmt.Errorf("stat log file %s: %w", base, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("log path is not a regular file: %s", base)
		}
		return visit(base, info)
	}
	segment := segments[0]
	if !strings.ContainsAny(segment, "*?[\\") {
		path := filepath.Join(base, segment)
		if len(segments) > 1 {
			info, err := os.Stat(path)
			if errors.Is(err, os.ErrNotExist) {
				if _, linkErr := os.Lstat(path); errors.Is(linkErr, os.ErrNotExist) {
					return nil
				}
			}
			if err != nil {
				return fmt.Errorf("stat log directory %s: %w", path, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("log directory path is not a directory: %s", path)
			}
		}
		return scanner.walk(path, segments[1:], visit)
	}
	scanner.dirs++
	if scanner.dirs > maxFilebeatCheckDirs {
		return fmt.Errorf("log path check directory limit exceeded (%d)", maxFilebeatCheckDirs)
	}
	dir, err := os.Open(base)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("open log directory %s: %w", base, err)
	}
	defer dir.Close()
	for {
		if err := scanner.ctx.Err(); err != nil {
			return err
		}
		entries, err := dir.ReadDir(128)
		if err != nil && err != io.EOF {
			return fmt.Errorf("read log directory %s: %w", base, err)
		}
		for _, entry := range entries {
			scanner.entries++
			if scanner.entries > maxFilebeatCheckEntries {
				return fmt.Errorf("log path check entry limit exceeded (%d)", maxFilebeatCheckEntries)
			}
			matched, matchErr := filepath.Match(segment, entry.Name())
			if matchErr != nil {
				return matchErr
			}
			if !matched {
				continue
			}
			path := filepath.Join(base, entry.Name())
			if len(segments) > 1 {
				info, err := os.Stat(path)
				if err != nil {
					return fmt.Errorf("stat log directory %s: %w", path, err)
				}
				if !info.IsDir() {
					continue
				}
			}
			if err := scanner.walk(path, segments[1:], visit); err != nil {
				return err
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func checkFilebeatPaths(ctx context.Context, files []filebeatConfigFile) (filebeatPathReceipt, error) {
	receipt := filebeatPathReceipt{Version: 1, FilesDigest: filebeatFilesDigest(files), UnmatchedPatterns: []string{}}
	scanner := filebeatPathScanner{ctx: ctx}
	ids := map[string]bool{}
	for _, file := range files {
		var inputs []struct {
			Type    string            `yaml:"type"`
			ID      string            `yaml:"id"`
			Enabled *bool             `yaml:"enabled"`
			Paths   []string          `yaml:"paths"`
			Fields  map[string]string `yaml:"fields"`
		}
		if err := yaml.Unmarshal([]byte(file.Content), &inputs); err != nil {
			return receipt, fmt.Errorf("decode managed Filebeat input %s: %w", file.Path, err)
		}
		if len(inputs) == 0 {
			return receipt, fmt.Errorf("managed Filebeat fragment has no inputs: %s", file.Path)
		}
		for _, input := range inputs {
			if input.Enabled != nil && !*input.Enabled {
				continue
			}
			if input.Type != "filestream" || input.ID == "" || ids[input.ID] || len(input.Paths) != 1 {
				return receipt, fmt.Errorf("invalid or duplicate filestream input in %s: %s", file.Path, input.ID)
			}
			ids[input.ID] = true
			receipt.InputsChecked++
			if receipt.InputsChecked > maxFilebeatCheckInputs {
				return receipt, fmt.Errorf("log path check input limit exceeded (%d)", maxFilebeatCheckInputs)
			}
			pattern := input.Paths[0]
			if !filepath.IsAbs(pattern) || filepath.Clean(pattern) != pattern ||
				strings.Contains(pattern, "**") || strings.Contains(pattern, "${") {
				return receipt, fmt.Errorf("log path must be absolute and normalized without ** or unresolved macros: %s", pattern)
			}
			segments := strings.Split(strings.TrimPrefix(pattern, "/"), "/")
			if len(segments) > 64 {
				return receipt, fmt.Errorf("log path check depth limit exceeded: %s", pattern)
			}
			for _, segment := range segments {
				if _, err := filepath.Match(segment, ""); err != nil {
					return receipt, fmt.Errorf("invalid log path pattern %s: %w", pattern, err)
				}
			}
			owner := fmt.Sprintf("service %s instance %s log %s (input %s, pattern %s)",
				input.Fields["service"], input.Fields["instance"], input.Fields["log_name"], input.ID, pattern)
			before := scanner.matches
			err := scanner.walk("/", segments, func(path string, info os.FileInfo) error {
				scanner.matches++
				if scanner.matches > maxFilebeatCheckFiles {
					return fmt.Errorf("log path check file limit exceeded (%d)", maxFilebeatCheckFiles)
				}
				// SameFile uses the physical identity (device/inode on Unix), so both
				// hard links and symlinked files/directories are checked across inputs.
				for _, previous := range scanner.files {
					if os.SameFile(previous.info, info) {
						if previous.owner == owner {
							return nil
						}
						return fmt.Errorf("log file overlap: %s path %s and %s path %s refer to the same file",
							previous.owner, previous.path, owner, path)
					}
				}
				scanner.files = append(scanner.files, filebeatCheckedFile{info: info, path: path, owner: owner})
				return nil
			})
			if err != nil {
				return receipt, err
			}
			if scanner.matches == before {
				receipt.UnmatchedPatterns = append(receipt.UnmatchedPatterns, pattern)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	receipt.Checked = true
	receipt.MatchedFiles = scanner.matches
	return receipt, nil
}

func (e *Executor) checkFilebeatPaths(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	files, err := decodeFilebeatFiles(job.Params, filebeatInputsDir)
	if err != nil {
		return failedJobResult(job, started, err)
	}
	receipt, err := checkFilebeatPaths(ctx, files)
	if err != nil {
		return failedJobResult(job, started, err)
	}
	finished := time.Now()
	return protocol.JobResult{JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, StartedAt: started, FinishedAt: finished,
		CostMS: finished.Sub(started).Milliseconds(), Data: map[string]any{"path_check": receipt}}
}
