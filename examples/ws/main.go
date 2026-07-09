package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mel2oo/codex-sdk-go"
)

const capabilityToken = "codex-go-sdk-ws-example-token"

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	tokenFile, cleanup, err := writeCapabilityTokenFile(capabilityToken)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	server, err := codex.NewServer(ctx,
		codex.WithWs("ws://127.0.0.1:0"),
		codex.WithWsAuthMode("capability-token"),
		codex.WithWsTokenFile(tokenFile),
		codex.WithCodexPath("codex"),
		codex.WithLogger(logger),
		codex.WithStderr(os.Stderr),
	)
	if err != nil {
		panic(err)
	}
	defer server.Close()

	client, err := codex.NewClient(ctx,
		codex.WithWs(server.URL()),
		codex.WithWsToken(capabilityToken),
		codex.WithLogger(logger),
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	thread, err := client.StartThread(ctx, codex.ThreadStartOptions{})
	if err != nil {
		panic(err)
	}

	result, err := thread.Run(ctx, "Say hello from the WebSocket example in one sentence.", nil)
	if err != nil {
		panic(err)
	}

	fmt.Println(result.FinalResponse)
}

func writeCapabilityTokenFile(token string) (string, func(), error) {
	file, err := os.CreateTemp("", "codex-go-sdk-ws-token-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	if _, err := file.WriteString(token + "\n"); err != nil {
		_ = file.Close()
		cleanup()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}
