//go:build linux

package grpcfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Resolve legitimate release/log links on every open, then walk the resolved
// path with pinned descriptors and O_NOFOLLOW so replacements cannot redirect it.
func openTailFile(path string) (*os.File, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "*?[") {
		return nil, "", fmt.Errorf("exact absolute file path required")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, "", err
	}
	dir, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	parts := strings.Split(strings.TrimPrefix(resolved, "/"), "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(dir, part, flags, 0)
		unix.Close(dir)
		if openErr != nil {
			return nil, "", &os.PathError{Op: "open live-tail", Path: path, Err: openErr}
		}
		dir = next
	}
	file := os.NewFile(uintptr(dir), path)
	var stat unix.Stat_t
	if err = unix.Fstat(dir, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		file.Close()
		return nil, "", fmt.Errorf("only readable regular files are supported")
	}
	return file, fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
