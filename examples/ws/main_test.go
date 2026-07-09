package main

import (
	"os"
	"strings"
	"testing"
)

func TestWriteCapabilityTokenFile(t *testing.T) {
	path, cleanup, err := writeCapabilityTokenFile("test-token")
	if err != nil {
		t.Fatalf("write token file: %v", err)
	}
	defer cleanup()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read token file: %v", err)
	}
	if string(content) != "test-token\n" {
		t.Fatalf("unexpected token file content: %q", content)
	}
	if !strings.Contains(path, "codex-go-sdk-ws-token-") {
		t.Fatalf("unexpected token file path: %q", path)
	}
}
