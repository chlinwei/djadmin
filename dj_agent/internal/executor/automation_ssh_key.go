package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

// syncAutomationSSHKey installs the backend-managed public key for root only.
// The action intentionally cannot choose a destination path or user, preventing
// the agent channel from becoming an arbitrary file-write capability.
func (e *Executor) syncAutomationSSHKey(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	publicKey := strings.TrimSpace(toString(job.Params["public_key"]))
	fields := strings.Fields(publicKey)
	if len(fields) < 2 || strings.ContainsAny(publicKey, "\r\n") ||
		(fields[0] != "ssh-ed25519" && fields[0] != "ssh-rsa" && !strings.HasPrefix(fields[0], "ecdsa-sha2-")) {
		return automationSSHKeyFailure(job, started, fmt.Errorf("invalid SSH public key"))
	}

	select {
	case <-ctx.Done():
		return automationSSHKeyFailure(job, started, ctx.Err())
	default:
	}

	sshDir := "/root/.ssh"
	authorizedKeysPath := filepath.Join(sshDir, "authorized_keys")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("create root SSH directory: %w", err))
	}
	if err := os.Chmod(sshDir, 0o700); err != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("set root SSH directory mode: %w", err))
	}

	existing, err := os.ReadFile(authorizedKeysPath)
	if err != nil && !os.IsNotExist(err) {
		return automationSSHKeyFailure(job, started, fmt.Errorf("read authorized_keys: %w", err))
	}
	keyIdentity := fields[0] + " " + fields[1]
	for _, line := range strings.Split(string(existing), "\n") {
		lineFields := strings.Fields(line)
		if len(lineFields) >= 2 && lineFields[0]+" "+lineFields[1] == keyIdentity {
			return automationSSHKeySuccess(job, started, false)
		}
	}

	file, err := os.OpenFile(authorizedKeysPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("open authorized_keys: %w", err))
	}
	_, writeErr := file.WriteString(publicKey + "\n")
	closeErr := file.Close()
	if writeErr != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("write authorized_keys: %w", writeErr))
	}
	if closeErr != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("close authorized_keys: %w", closeErr))
	}
	if err := os.Chmod(authorizedKeysPath, 0o600); err != nil {
		return automationSSHKeyFailure(job, started, fmt.Errorf("set authorized_keys mode: %w", err))
	}
	return automationSSHKeySuccess(job, started, true)
}

func automationSSHKeySuccess(job protocol.Job, started time.Time, added bool) protocol.JobResult {
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusSuccess,
		ExitCode: 0, StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"added": added},
	}
}

func automationSSHKeyFailure(job protocol.Job, started time.Time, err error) protocol.JobResult {
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusFailed,
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(), Error: err.Error(),
	}
}
