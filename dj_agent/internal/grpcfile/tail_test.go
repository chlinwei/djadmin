//go:build linux

package grpcfile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/chlinwei/djadmin/dj_agent/internal/grpcfile/pb"
	"golang.org/x/sys/unix"
)

func tailTestDir(t *testing.T) string {
	t.Helper()
	name := fmt.Sprintf(".tail-test-%d", time.Now().UnixNano())
	if err := os.Mkdir(name, 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(name) })
	abs, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestTailUTF8PrefixBoundaries(t *testing.T) {
	for _, text := range []string{"日志", "a🙂", "λ", strings.Repeat("x", tailChunk-1) + "日"} {
		bytes := []byte(text)
		for split := max(0, len(bytes)-4); split <= len(bytes); split++ {
			end, err := tailTextPrefix(bytes[:split])
			if err != nil || !utf8.Valid(bytes[:end]) || split-end > 3 {
				t.Fatalf("invalid UTF-8 boundary at %d: end=%d err=%v", split, end, err)
			}
		}
	}
	if _, err := tailTextPrefix([]byte{0xff}); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	if _, err := tailTextPrefix([]byte{0}); err == nil {
		t.Fatal("NUL accepted")
	}
}

func TestTailManagerStopClosesDescriptorAndReleasesTask(t *testing.T) {
	dir := tailTestDir(t)
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess := &session{tailOutput: make(chan *pb.TailEvent, 8)}
	sess.handleTail(ctx, &pb.TailRequest{RequestId: "cleanup", Action: "prepare", Path: path})
	select {
	case event := <-sess.tailOutput:
		if event.Type != "ready" {
			t.Fatalf("unexpected preparation: %+v", event)
		}
		sess.handleTail(ctx, &pb.TailRequest{RequestId: "cleanup", Action: "start", Identity: event.Identity})
	case <-time.After(time.Second):
		t.Fatal("prepare timed out")
	}
	sess.handleTail(ctx, &pb.TailRequest{RequestId: "cleanup", Action: "stop"})
	sess.handleTail(ctx, &pb.TailRequest{RequestId: "cleanup", Action: "stop"})
	deadline := time.Now().Add(time.Second)
	for {
		sess.tails.mu.Lock()
		count := len(sess.tails.tasks)
		sess.tails.mu.Unlock()
		if count == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stop leaked task")
		}
		time.Sleep(time.Millisecond)
	}
	// Linux procfs verifies actual closure, not just UI or map state.
	descriptors, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for _, descriptor := range descriptors {
		target, _ := os.Readlink("/proc/self/fd/" + descriptor.Name())
		if target == path {
			t.Fatal("stop leaked open file descriptor")
		}
	}
}

func TestTailHostBudgetAndOutputQueueAreBounded(t *testing.T) {
	manager := &tailManager{}
	if !manager.allow(1024*1024) || manager.allow(1) {
		t.Fatal("host budget not bounded to 1 MiB/window")
	}
	sess := &session{tailOutput: make(chan *pb.TailEvent, 8)}
	for i := 0; i < 8; i++ {
		if !sess.queueTail(&pb.TailEvent{Data: make([]byte, tailChunk)}) {
			t.Fatal("queue full too early")
		}
	}
	if sess.queueTail(&pb.TailEvent{Data: make([]byte, tailChunk)}) {
		t.Fatal("unbounded output queue")
	}
}

func TestTailSecureOpenAndPhysicalIdentity(t *testing.T) {
	dir := tailTestDir(t)
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	file, identity, err := openTailFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err = os.Link(path, filepath.Join(dir, "alias.log")); err != nil {
		t.Fatal(err)
	}
	file, aliasIdentity, err := openTailFile(filepath.Join(dir, "alias.log"))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if aliasIdentity != identity {
		t.Fatal("hardlink alias has different physical identity")
	}
	if err = os.Symlink(path, filepath.Join(dir, "symlink.log")); err != nil {
		t.Fatal(err)
	}
	file, aliasIdentity, err = openTailFile(filepath.Join(dir, "symlink.log"))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if aliasIdentity != identity {
		t.Fatal("symlink alias has different physical identity")
	}
	if err = os.Symlink(dir, filepath.Join(dir, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	file, aliasIdentity, err = openTailFile(filepath.Join(dir, "linked-dir", "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if aliasIdentity != identity {
		t.Fatal("symlink parent has different physical identity")
	}
	if err = os.Symlink("missing.log", filepath.Join(dir, "broken.log")); err != nil {
		t.Fatal(err)
	}
	if file, _, err = openTailFile(filepath.Join(dir, "broken.log")); !os.IsNotExist(err) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("want missing target error, got %v", err)
	}
	if err = os.Symlink("loop.log", filepath.Join(dir, "loop.log")); err != nil {
		t.Fatal(err)
	}
	if file, _, err = openTailFile(filepath.Join(dir, "loop.log")); err == nil {
		file.Close()
		t.Fatal("symlink loop accepted")
	}
	if err = os.Symlink(dir, filepath.Join(dir, "directory.log")); err != nil {
		t.Fatal(err)
	}
	if file, _, err = openTailFile(filepath.Join(dir, "directory.log")); err == nil {
		file.Close()
		t.Fatal("symlink to directory accepted")
	}
	if file, _, err = openTailFile(dir); err == nil {
		file.Close()
		t.Fatal("directory accepted")
	}
	fifo := filepath.Join(dir, "pipe.log")
	if err = unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	if file, _, err = openTailFile(fifo); err == nil {
		file.Close()
		t.Fatal("FIFO accepted")
	}
	if err = os.Symlink(fifo, filepath.Join(dir, "linked-pipe.log")); err != nil {
		t.Fatal(err)
	}
	if file, _, err = openTailFile(filepath.Join(dir, "linked-pipe.log")); err == nil {
		file.Close()
		t.Fatal("symlink to FIFO accepted")
	}
}

func TestTailSnapshotAppendTruncateRotationAndCleanup(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("symlinks=%t", linked), func(t *testing.T) {
			testTailSnapshotAppendTruncateRotationAndCleanup(t, linked)
		})
	}
}

func testTailSnapshotAppendTruncateRotationAndCleanup(t *testing.T, linked bool) {
	t.Helper()
	dir := tailTestDir(t)
	physicalPath := filepath.Join(dir, "common", "log", "app.log")
	if err := os.MkdirAll(filepath.Dir(physicalPath), 0700); err != nil {
		t.Fatal(err)
	}
	path := physicalPath
	if linked {
		if err := os.MkdirAll(filepath.Join(dir, "release", "run"), 0700); err != nil {
			t.Fatal(err)
		}
		for link, target := range map[string]string{
			filepath.Join(dir, "run"):                          filepath.Join(dir, "release", "run"),
			filepath.Join(dir, "release", "run", "log"):        "../../common/log",
			filepath.Join(dir, "common", "log", "current.log"): "app.log",
		} {
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
		}
		path = filepath.Join(dir, "run", "log", "current.log")
	}
	if err := os.WriteFile(physicalPath, []byte("initial\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	task := &tailTask{path: path, start: make(chan string, 1)}
	task.lease.Store(time.Now().UnixNano())
	events := make(chan *pb.TailEvent, 32)
	done := make(chan error, 1)
	go func() {
		done <- followTail(ctx, task, func(event *pb.TailEvent) bool {
			if event.Type == "ready" {
				task.start <- event.Identity
			}
			events <- event
			return true
		}, func(int) bool { return true })
	}()
	await := func(kind, text string) *pb.TailEvent {
		t.Helper()
		timer := time.NewTimer(4 * time.Second)
		defer timer.Stop()
		for {
			select {
			case event := <-events:
				if event.Type == kind && strings.Contains(string(event.Data)+event.Message, text) {
					return event
				}
			case err := <-done:
				t.Fatalf("tail ended early: %v", err)
			case <-timer.C:
				t.Fatalf("missing %s event %q", kind, text)
			}
		}
	}
	firstIdentity := await("ready", "").Identity
	await("data", "initial")
	file, err := os.OpenFile(physicalPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteString("append\n")
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	await("data", "append")
	if err = os.WriteFile(physicalPath, []byte("reset\n"), 0600); err != nil {
		t.Fatal(err)
	}
	await("changed", "truncated")
	await("data", "reset")
	if err = os.Rename(physicalPath, physicalPath+".1"); err != nil {
		t.Fatal(err)
	}
	await("waiting", "waiting")
	if err = os.WriteFile(physicalPath, []byte("rotated\n"), 0600); err != nil {
		t.Fatal(err)
	}
	await("changed", "rotated")
	rotatedIdentity := await("ready", "").Identity
	if rotatedIdentity == firstIdentity {
		t.Fatal("rotation did not admit a new physical identity")
	}
	await("data", "rotated")
	if linked {
		nextPath := filepath.Join(filepath.Dir(physicalPath), "next.log")
		if err = os.WriteFile(nextPath, []byte("link switched\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.Symlink("next.log", path+".next"); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(path+".next", path); err != nil {
			t.Fatal(err)
		}
		await("changed", "rotated")
		switchedIdentity := await("ready", "").Identity
		if switchedIdentity == rotatedIdentity {
			t.Fatal("link switch did not admit a new physical identity")
		}
		await("data", "link switched")
		nextDir := filepath.Join(dir, "next-logs")
		if err = os.Mkdir(nextDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(nextDir, "current.log"), []byte("directory switched\n"), 0600); err != nil {
			t.Fatal(err)
		}
		logLink := filepath.Join(dir, "release", "run", "log")
		if err = os.Symlink(nextDir, logLink+".next"); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(logLink+".next", logLink); err != nil {
			t.Fatal(err)
		}
		await("changed", "rotated")
		if await("ready", "").Identity == switchedIdentity {
			t.Fatal("directory switch did not admit a new physical identity")
		}
		await("data", "directory switched")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancel did not release reader")
	}
}

func TestTailSparseSnapshotIsBoundedAndLeaseExpires(t *testing.T) {
	dir := tailTestDir(t)
	path := filepath.Join(dir, "large.log")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	const size = 4 * 1024 * 1024 * 1024
	if err = file.Truncate(size); err != nil {
		t.Fatal(err)
	}
	_, err = file.WriteAt([]byte(strings.Repeat("x\n", 128*1024)), size-256*1024)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	task := &tailTask{path: path, start: make(chan string, 1)}
	task.lease.Store(time.Now().UnixNano())
	total := 0
	err = followTail(ctx, task, func(event *pb.TailEvent) bool {
		if event.Type == "ready" {
			task.start <- event.Identity
		}
		total += len(event.Data)
		if event.Type == "data" {
			task.lease.Store(time.Now().Add(-46 * time.Second).UnixNano())
		}
		return true
	}, func(int) bool { return true })
	if err == nil || !strings.Contains(err.Error(), "lease expired") {
		t.Fatalf("expected expired lease, got %v", err)
	}
	if total > 400 || total == 0 {
		t.Fatalf("snapshot read %d bytes, want <=200 short lines", total)
	}
}
