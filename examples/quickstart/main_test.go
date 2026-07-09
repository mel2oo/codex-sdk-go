package main

import (
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/openai/codex/sdk/go/examples/internal/testutil"
)

func TestMainReplay(t *testing.T) {
	t.Setenv(exampleReplayEnv, "1")

	output := testutil.CaptureOutput(main)
	if strings.TrimSpace(output) != "Hello from replay" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestExampleOptionsDefault(t *testing.T) {
	t.Setenv(exampleReplayEnv, "")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	opts := exampleOptions("prompt", logger)
	if len(opts) != 1 {
		t.Fatalf("expected logger option for default options, got %d", len(opts))
	}
}

func TestMustRawNil(t *testing.T) {
	if raw := mustRaw(nil); raw != nil {
		t.Fatalf("expected nil raw message, got %s", raw)
	}
}
