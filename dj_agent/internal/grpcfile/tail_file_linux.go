//go:build linux

package grpcfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Walk with directory descriptors so a concurrent symlink replacement cannot
// redirect a permitted log path into a different directory (including on reopen).
func openTailFile(path string) (*os.File, string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || strings.ContainsAny(path, "*?[") {
		return nil, "", fmt.Errorf("exact absolute file path required")
	}
	dir, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(dir, part, flags, 0)
		unix.Close(dir)
		if openErr != nil {
			return nil, "", openErr
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
