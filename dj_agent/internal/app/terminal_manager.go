package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/nats-io/nats.go"
)

type termCommand struct {
	SessionID string `json:"session_id"`
	Op        string `json:"op"`
	Data      string `json:"data"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

type termSession struct {
	cmd     *exec.Cmd
	ptyFile *os.File
}

type terminalManager struct {
	agentID  string
	nc       *nats.Conn
	mu       sync.Mutex
	sessions map[string]*termSession
}

func newTerminalManager(agentID string, nc *nats.Conn) *terminalManager {
	return &terminalManager{
		agentID:  agentID,
		nc:       nc,
		sessions: make(map[string]*termSession),
	}
}

func (m *terminalManager) Handle(cmd termCommand) {
	sessionID := strings.TrimSpace(cmd.SessionID)
	if sessionID == "" {
		return
	}

	op := strings.ToLower(strings.TrimSpace(cmd.Op))
	switch op {
	case "open":
		m.open(sessionID, cmd)
	case "input":
		m.input(sessionID, cmd.Data)
	case "resize":
		m.resize(sessionID, cmd.Cols, cmd.Rows)
	case "close":
		m.close(sessionID, "session closed")
	default:
		m.publishEvent(sessionID, "error", map[string]any{"message": "unsupported term op"})
	}
}

func (m *terminalManager) CloseAll() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for sessionID := range m.sessions {
		ids = append(ids, sessionID)
	}
	m.mu.Unlock()

	for _, sessionID := range ids {
		m.close(sessionID, "agent shutting down")
	}
}

func (m *terminalManager) open(sessionID string, cmd termCommand) {
	m.close(sessionID, "session replaced")

	shell := strings.TrimSpace(os.Getenv("DJ_AGENT_TERM_SHELL"))
	if shell == "" {
		shell = "/bin/bash"
	}

	homeDir := resolveTerminalHomeDir()

	// Force login+interactive shell so both profile and interactive rc are loaded.
	shellArgs := []string{"-il"}
	execCmd := exec.Command(shell, shellArgs...)
	execCmd.Env = append(os.Environ(), "HOME="+homeDir)
	execCmd.Dir = homeDir

	cols := toValidDim(cmd.Cols, 120)
	rows := toValidDim(cmd.Rows, 32)

	ptyFile, err := pty.StartWithSize(execCmd, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	if err != nil {
		m.publishEvent(sessionID, "error", map[string]any{"message": fmt.Sprintf("open pty failed: %v", err)})
		return
	}

	m.mu.Lock()
	m.sessions[sessionID] = &termSession{cmd: execCmd, ptyFile: ptyFile}
	m.mu.Unlock()

	m.publishEvent(sessionID, "connected", map[string]any{
		"agent_id":          m.agentID,
		"supports_file_ops": false,
		"home_dir":          execCmd.Dir,
	})

	go m.readLoop(sessionID, ptyFile)
	go m.waitProcess(sessionID, execCmd)
}

func (m *terminalManager) input(sessionID, data string) {
	session := m.getSession(sessionID)
	if session == nil || data == "" {
		return
	}
	_, err := session.ptyFile.WriteString(data)
	if err != nil {
		m.publishEvent(sessionID, "error", map[string]any{"message": fmt.Sprintf("write pty failed: %v", err)})
		m.close(sessionID, "write failed")
	}
}

func (m *terminalManager) resize(sessionID string, cols, rows int) {
	session := m.getSession(sessionID)
	if session == nil {
		return
	}
	ws := &pty.Winsize{Cols: uint16(toValidDim(cols, 120)), Rows: uint16(toValidDim(rows, 32))}
	if err := pty.Setsize(session.ptyFile, ws); err != nil {
		m.publishEvent(sessionID, "error", map[string]any{"message": fmt.Sprintf("resize pty failed: %v", err)})
	}
}

func (m *terminalManager) close(sessionID, message string) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if ok {
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if !ok || session == nil {
		return
	}

	if session.ptyFile != nil {
		_ = session.ptyFile.Close()
	}
	if session.cmd != nil && session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}

	m.publishEvent(sessionID, "closed", map[string]any{"message": message})
}

func (m *terminalManager) readLoop(sessionID string, ptyFile *os.File) {
	buf := make([]byte, 4096)
	for {
		n, err := ptyFile.Read(buf)
		if n > 0 {
			m.publishEvent(sessionID, "output", map[string]any{"data": string(buf[:n])})
		}
		if err != nil {
			if !isExpectedPtyCloseError(err) {
				slog.Warn("term read error", "session_id", sessionID, "err", err)
			}
			m.close(sessionID, "pty closed")
			return
		}
	}
}

func isExpectedPtyCloseError(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, io.EOF) {
		return true
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errno, ok := pathErr.Err.(syscall.Errno); ok && errno == syscall.EIO {
			return true
		}
	}

	return false
}

func (m *terminalManager) waitProcess(sessionID string, cmd *exec.Cmd) {
	_ = cmd.Wait()
	// Wait a brief moment to let buffered output flush before close event.
	time.Sleep(50 * time.Millisecond)
	m.close(sessionID, "process exited")
}

func (m *terminalManager) publishEvent(sessionID, eventType string, payload map[string]any) {
	if m.nc == nil {
		return
	}
	data := map[string]any{
		"type":       eventType,
		"session_id": sessionID,
		"agent_id":   m.agentID,
		"ts":         time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range payload {
		data[k] = v
	}

	body, err := json.Marshal(data)
	if err != nil {
		return
	}
	subject := fmt.Sprintf("evt.term.%s", sessionID)
	if err = m.nc.Publish(subject, body); err != nil {
		slog.Warn("publish term event failed", "subject", subject, "err", err)
	}
}

func (m *terminalManager) getSession(sessionID string) *termSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionID]
}

func toValidDim(v int, fallback int) int {
	if v <= 0 {
		return fallback
	}
	if v > 500 {
		return 500
	}
	return v
}

func resolveTerminalHomeDir() string {
	customHome := strings.TrimSpace(os.Getenv("DJ_AGENT_TERM_HOME"))
	if customHome != "" {
		return customHome
	}

	homeDir, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(homeDir) != "" {
		return homeDir
	}

	// 极端场景兜底：仍保证不返回空路径。
	return "/"
}
