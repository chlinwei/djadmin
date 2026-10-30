package grpcfile

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
)

const tailChunk = 32 * 1024

func tailTextPrefix(data []byte) (int, error) {
	for i := 0; i < len(data); {
		if !utf8.FullRune(data[i:]) {
			return i, nil
		}
		value, size := utf8.DecodeRune(data[i:])
		if value == 0 || (value == utf8.RuneError && size == 1) {
			return 0, fmt.Errorf("only UTF-8 text logs are supported")
		}
		i += size
	}
	return len(data), nil
}

type tailTask struct {
	path   string
	cancel context.CancelFunc
	start  chan string
	lease  atomic.Int64
}

type tailManager struct {
	mu     sync.Mutex
	tasks  map[string]*tailTask
	window time.Time
	bytes  int
}

func (s *session) handleTail(ctx context.Context, req *pb.TailRequest) {
	s.tails.mu.Lock()
	if s.tails.tasks == nil {
		s.tails.tasks = make(map[string]*tailTask)
	}
	task := s.tails.tasks[req.RequestId]
	if req.Action != "prepare" {
		if task != nil {
			switch req.Action {
			case "stop":
				task.cancel()
			case "renew":
				task.lease.Store(time.Now().UnixNano())
				if !s.queueTail(&pb.TailEvent{RequestId: req.RequestId, Type: "lease"}) {
					task.cancel()
				}
			case "start":
				select {
				case task.start <- req.Identity:
				default:
					task.cancel()
				}
			}
		} else if req.Action == "renew" {
			// A full data queue can also lose the final error. Subsequent
			// renewal must report the missing reader, never silently succeed.
			s.queueTail(&pb.TailEvent{RequestId: req.RequestId, Type: "error", Message: "live-tail reader is no longer active; possible gap"})
		}
		s.tails.mu.Unlock()
		return
	}
	paths := map[string]bool{}
	for _, current := range s.tails.tasks {
		paths[current.path] = true
	}
	if task != nil || len(s.tails.tasks) >= 20 || (!paths[req.Path] && len(paths) >= 10) {
		s.tails.mu.Unlock()
		s.queueTail(&pb.TailEvent{RequestId: req.RequestId, Type: "error", Message: "agent live-tail capacity reached"})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	task = &tailTask{path: req.Path, cancel: cancel, start: make(chan string, 1)}
	task.lease.Store(time.Now().UnixNano())
	s.tails.tasks[req.RequestId] = task
	s.tails.mu.Unlock()
	go func() {
		defer cancel()
		defer func() {
			s.tails.mu.Lock()
			delete(s.tails.tasks, req.RequestId)
			s.tails.mu.Unlock()
		}()
		err := followTail(ctx, task, func(event *pb.TailEvent) bool {
			event.RequestId = req.RequestId
			return s.queueTail(event)
		}, s.tails.allow)
		event := &pb.TailEvent{RequestId: req.RequestId, Type: "end", Message: "stopped"}
		if err != nil && !errors.Is(err, context.Canceled) {
			event.Type, event.Message = "error", err.Error()
		}
		s.queueTail(event)
	}()
}

// One bounded queue and sender per gRPC session: a blocked transport cannot
// retain one sending goroutine or unlimited log buffers for every reader.
func (s *session) queueTail(event *pb.TailEvent) bool {
	select {
	case s.tailOutput <- event:
		return true
	default:
		return false
	}
}

func (m *tailManager) allow(n int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if now.Sub(m.window) >= time.Second {
		m.window, m.bytes = now, 0
	}
	if m.bytes+n > 1024*1024 {
		return false
	}
	m.bytes += n
	return true
}

func followTail(ctx context.Context, task *tailTask, emit func(*pb.TailEvent) bool, allow func(int) bool) error {
	for _, extension := range []string{".gz", ".zip", ".xz", ".bz2", ".zst", ".lz4", ".7z"} {
		if strings.HasSuffix(strings.ToLower(task.path), extension) {
			return fmt.Errorf("compressed logs are not supported")
		}
	}
	file, identity, err := openTailFile(task.path)
	if err != nil {
		return err
	}
	defer func() { file.Close() }()
	send := func(kind, message string, data []byte) error {
		if !emit(&pb.TailEvent{Type: kind, Message: message, Data: data, Identity: identity}) {
			return fmt.Errorf("live-tail output congestion; possible gap")
		}
		return nil
	}
	check := func() error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Since(time.Unix(0, task.lease.Load())) > 45*time.Second {
			return fmt.Errorf("live-tail lease expired")
		}
		return nil
	}
	admit := func() error {
		if err := send("ready", "waiting for admission", nil); err != nil {
			return err
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("admission timeout")
		case expected := <-task.start:
			if expected != identity {
				return fmt.Errorf("file identity changed")
			}
			return nil
		}
	}
	if err = admit(); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	// Only read the last 256 KiB, then select at most 200 lines. Snapshot and
	// follow share the same descriptor/cursor, including concurrent appends.
	offset := max(int64(0), info.Size()-256*1024)
	initial := make([]byte, info.Size()-offset)
	n, err := file.ReadAt(initial, offset)
	if err != nil && err != io.EOF {
		return err
	}
	initial = initial[:n]
	if bytes.IndexByte(initial, 0) >= 0 || bytes.HasPrefix(initial, []byte{0x1f, 0x8b}) {
		return fmt.Errorf("binary/compressed logs are not supported")
	}
	offset += int64(n)
	for count, i := 0, len(initial)-2; i >= 0; i-- {
		if initial[i] == '\n' {
			count++
			if count == 200 {
				initial = initial[i+1:]
				break
			}
		}
	}
	for len(initial) > 0 && !utf8.RuneStart(initial[0]) {
		initial = initial[1:]
	}
	complete, err := tailTextPrefix(initial)
	if err != nil {
		return err
	}
	pending := append([]byte(nil), initial[complete:]...)
	initial = initial[:complete]
	if err = send("snapshot", "initial tail is limited to 200 lines / 256 KiB", nil); err != nil {
		return err
	}
	ticker := time.NewTicker(125 * time.Millisecond)
	defer ticker.Stop()
	missingSince := time.Time{}
	rotatingSince := time.Time{}
	budgetDeniedSince := time.Time{}
	budget := func(n int) (bool, error) {
		if allow(n) {
			budgetDeniedSince = time.Time{}
			return true, nil
		}
		if budgetDeniedSince.IsZero() {
			budgetDeniedSince = time.Now()
		}
		if time.Since(budgetDeniedSince) > 3*time.Second {
			return false, fmt.Errorf("host live-tail rate budget congested; possible gap")
		}
		return false, nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
		if err = check(); err != nil {
			return err
		}
		if len(initial) > 0 {
			n := min(len(initial), tailChunk)
			allowed, budgetErr := budget(n)
			if budgetErr != nil {
				return budgetErr
			}
			if !allowed {
				continue
			}
			if err = send("data", "", initial[:n]); err != nil {
				return err
			}
			initial = initial[n:]
			continue
		}
		info, err = file.Stat()
		if err != nil {
			return err
		}
		if info.Size() < offset {
			offset, pending = 0, nil
			if err = send("changed", "file truncated; possible gap", nil); err != nil {
				return err
			}
		}
		if info.Size()-offset > 1024*1024 {
			return fmt.Errorf("reader more than 1 MiB behind; possible gap")
		}
		if info.Size() > offset {
			next, nextIdentity, openErr := openTailFile(task.path)
			if next != nil {
				next.Close()
			}
			if openErr != nil && !os.IsNotExist(openErr) {
				return openErr
			}
			if openErr != nil || nextIdentity != identity {
				if rotatingSince.IsZero() {
					rotatingSince = time.Now()
				}
				if time.Since(rotatingSince) > 5*time.Second {
					return fmt.Errorf("rotated file keeps growing; possible gap")
				}
			}
			allowed, budgetErr := budget(tailChunk)
			if budgetErr != nil {
				return budgetErr
			}
			if !allowed {
				continue
			}
			buf := make([]byte, min(int64(tailChunk-len(pending)), info.Size()-offset))
			n, readErr := file.ReadAt(buf, offset)
			if readErr != nil && readErr != io.EOF {
				return readErr
			}
			offset += int64(n)
			pending = append(pending, buf[:n]...)
			// Retain at most one incomplete UTF-8 rune, never an entire line.
			end, textErr := tailTextPrefix(pending)
			if textErr != nil {
				return textErr
			}
			if err = send("data", "", pending[:end]); err != nil {
				return err
			}
			pending = append([]byte(nil), pending[end:]...)
			continue
		}
		// Resolve the original logical path again so release/log link changes
		// go through the same safe open and identity admission as file rotation.
		next, nextIdentity, openErr := openTailFile(task.path)
		if os.IsNotExist(openErr) {
			if missingSince.IsZero() {
				missingSince = time.Now()
				if err = send("waiting", "waiting for file", nil); err != nil {
					return err
				}
			}
			if time.Since(missingSince) > 60*time.Second {
				return fmt.Errorf("file absent for 60 seconds")
			}
			continue
		}
		if openErr != nil {
			return openErr
		}
		missingSince = time.Time{}
		if nextIdentity == identity {
			next.Close()
			continue
		}
		file.Close()
		file, identity, offset, pending = next, nextIdentity, 0, nil
		rotatingSince = time.Time{}
		if err = send("changed", "file rotated; possible gap", nil); err != nil {
			return err
		}
		if err = admit(); err != nil {
			return err
		}
	}
}
