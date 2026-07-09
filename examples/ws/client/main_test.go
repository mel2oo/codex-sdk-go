package main

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/mel2oo/codex-sdk-go"
)

func TestFixedWebsocketConfig(t *testing.T) {
	if websocketURL != "ws://127.0.0.1:8999" {
		t.Fatalf("unexpected websocket URL: %q", websocketURL)
	}
	if capabilityToken != "codex-go-sdk-ws-example-token" {
		t.Fatalf("unexpected capability token: %q", capabilityToken)
	}
}

func TestPrintNotification(t *testing.T) {
	var output bytes.Buffer
	stdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = writer
	defer func() {
		os.Stdout = stdout
	}()

	if err := printNotification("thread-1", "turn/started", json.RawMessage(`{"threadId":"thr"}`)); err != nil {
		t.Fatalf("print notification: %v", err)
	}
	_ = writer.Close()
	if _, err := output.ReadFrom(reader); err != nil {
		t.Fatalf("read output: %v", err)
	}

	got := output.String()
	if !strings.Contains(got, `"thread": "thread-1"`) ||
		!strings.Contains(got, `"method": "turn/started"`) ||
		!strings.Contains(got, `"threadId": "thr"`) {
		t.Fatalf("unexpected output: %s", got)
	}
}

func TestThreadExamples(t *testing.T) {
	examples := threadExamples()
	if len(examples) != 2 {
		t.Fatalf("unexpected example count: %d", len(examples))
	}

	want := []threadExample{
		{
			Name:   "thread-1",
			Prompt: "Say hello from workspace-1 in one sentence.",
			Options: codex.ThreadStartOptions{
				Model:          "HengNao-v4",
				Cwd:            "/home/mel2oo/codex/sdk/go/examples/ws/client/workspace-1",
				ApprovalPolicy: codex.ApprovalPolicyNever,
				SandboxPolicy:  codex.SandboxModeDangerFullAccess,
				Config:         hnThreadConfig(),
			},
		},
		{
			Name:   "thread-2",
			Prompt: "Say hello from workspace-2 in one sentence.",
			Options: codex.ThreadStartOptions{
				Model:          "HengNao-r1",
				Cwd:            "/home/mel2oo/codex/sdk/go/examples/ws/client/workspace-2",
				ApprovalPolicy: codex.ApprovalPolicyNever,
				SandboxPolicy:  codex.SandboxModeDangerFullAccess,
				Config:         hnThreadConfig(),
			},
		},
	}

	for i := range want {
		if examples[i].Name != want[i].Name ||
			examples[i].Prompt != want[i].Prompt ||
			examples[i].Options.Model != want[i].Options.Model ||
			examples[i].Options.Cwd != want[i].Options.Cwd ||
			examples[i].Options.ApprovalPolicy != want[i].Options.ApprovalPolicy ||
			examples[i].Options.SandboxPolicy != want[i].Options.SandboxPolicy ||
			!reflect.DeepEqual(examples[i].Options.Config, want[i].Options.Config) {
			t.Fatalf("unexpected example at %d: %#v", i, examples[i])
		}
	}
}

func TestHNThreadConfig(t *testing.T) {
	want := map[string]any{
		"model_provider": "hn",
		"model_providers": map[string]any{
			"hn": map[string]any{
				"base_url":                  "http://10.50.10.18:8995/v1",
				"experimental_bearer_token": "sk-HJfJZVLgxWWffNLCtDmUfeECSapdPngs",
				"name":                      "hn",
				"requires_openai_auth":      false,
				"wire_api":                  "responses",
			},
		},
	}

	if got := hnThreadConfig(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected config: %#v", got)
	}
}
