package audit

import (
	"strings"
	"testing"
)

func TestCleanTerminal(t *testing.T) {
	raw := "\x1b[31mred\x1b[0m\n\x1b]0;secret-title\x07echo ok\x00"
	cleaned := cleanTerminal(raw)
	if strings.Contains(cleaned, "\x1b") || strings.ContainsRune(cleaned, '\x00') {
		t.Fatalf("control sequence remained: %q", cleaned)
	}
	if !strings.Contains(cleaned, "red") || !strings.Contains(cleaned, "echo ok") {
		t.Fatalf("visible content was removed: %q", cleaned)
	}
}

func TestReadableCommands(t *testing.T) {
	commands := readableCommands(" ls -la\r\n\n whoami ")
	if len(commands) != 2 || commands[0] != "ls -la" || commands[1] != "whoami" {
		t.Fatalf("unexpected commands: %v", commands)
	}
}
