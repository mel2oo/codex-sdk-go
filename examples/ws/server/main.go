package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mel2oo/codex-sdk-go"
)

const (
	websocketURL          = "ws://127.0.0.1:8999"
	capabilityToken       = "codex-go-sdk-ws-example-token"
	capabilityTokenSha256 = "ad26ba1e118300f710156819514d134e224a2670166802bf628843f54d9049e2"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	server, err := codex.NewServer(ctx,
		codex.WithWs(websocketURL),
		codex.WithWsAuthMode("capability-token"),
		codex.WithWsTokenSha256(capabilityTokenSha256),
		codex.WithCodexPath("codex"),
		codex.WithLogger(logger),
		codex.WithStderr(os.Stderr),
	)
	if err != nil {
		panic(err)
	}
	defer server.Close()

	fmt.Printf("CODEX_WS_URL=%s\n", server.URL())
	fmt.Printf("CODEX_WS_TOKEN=%s\n", capabilityToken)
	fmt.Fprintln(os.Stderr, "codex websocket app-server is running; press Ctrl+C to stop")

	<-ctx.Done()
}
