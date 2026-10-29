//go:build !linux

package grpcfile

import (
	"fmt"
	"os"
)

func openTailFile(string) (*os.File, string, error) {
	return nil, "", fmt.Errorf("live tail requires Linux")
}
