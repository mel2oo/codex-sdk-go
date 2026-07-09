package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mel2oo/codex-sdk-go"
)

const (
	websocketURL    = "ws://127.0.0.1:8999"
	capabilityToken = "codex-go-sdk-ws-example-token"
)

type threadExample struct {
	Name    string
	Prompt  string
	Options codex.ThreadStartOptions
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	client, err := codex.NewClient(ctx,
		codex.WithWs(websocketURL),
		codex.WithWsToken(capabilityToken),
		codex.WithLogger(logger),
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	if err := runThreadExamples(ctx, client, threadExamples()); err != nil {
		panic(err)
	}
}

func threadExamples() []threadExample {
	config := hnThreadConfig()
	return []threadExample{
		{
			Name:   "thread-1",
			Prompt: "Say hello from workspace-1 in one sentence.",
			Options: codex.ThreadStartOptions{
				Model:          "HengNao-v4",
				Cwd:            "/home/mel2oo/codex/sdk/go/examples/ws/client/workspace-1",
				ApprovalPolicy: codex.ApprovalPolicyNever,
				SandboxPolicy:  codex.SandboxModeDangerFullAccess,
				Config:         config,
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
				Config:         config,
			},
		},
	}
}

func hnThreadConfig() map[string]any {
	return map[string]any{
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
}

func runThreadExamples(ctx context.Context, client *codex.Client, examples []threadExample) error {
	for _, example := range examples {
		if err := runThreadExample(ctx, client, example); err != nil {
			return fmt.Errorf("%s: %w", example.Name, err)
		}
	}
	return nil
}

func runThreadExample(ctx context.Context, client *codex.Client, example threadExample) error {
	thread, err := client.StartThread(ctx, example.Options)
	if err != nil {
		return err
	}

	stream, err := thread.RunStreamed(ctx, []codex.Input{codex.TextInput(example.Prompt)}, nil)
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		note, err := stream.Next(ctx)
		if err != nil {
			return err
		}
		if err := printNotification(example.Name, note.Method, note.Raw); err != nil {
			return err
		}
		if note.Method == "turn/completed" {
			return nil
		}
		if note.Method == "turn/failed" {
			return errors.New("turn failed")
		}
	}
}

func printNotification(threadName string, method string, raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	payload := struct {
		Thread string          `json:"thread"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}{
		Thread: threadName,
		Method: method,
		Params: raw,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
