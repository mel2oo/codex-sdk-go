# Codex Go SDK

Embed the Codex app-server in Go workflows.

This SDK speaks JSON-RPC to `codex app-server`. `NewClient` can spawn a stdio app-server or connect to an existing WebSocket/Unix socket server. `NewServer` manages a standalone WebSocket/Unix app-server process.

## Requirements

- Go 1.25+
- `codex` available on your `PATH`

## Install

```bash
go get github.com/mel2oo/codex-sdk-go
```

## Quickstart

```go
package main

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/mel2oo/codex-sdk-go"
)

func main() {
    ctx := context.Background()
    logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
    prompt := "Diagnose the test failure and propose a fix"

    client, err := codex.NewClient(ctx, codex.WithStdio(), codex.WithLogger(logger))
    if err != nil {
        panic(err)
    }
    defer client.Close()

    thread, err := client.StartThread(ctx, codex.ThreadStartOptions{})
    if err != nil {
        panic(err)
    }

    result, err := thread.Run(ctx, prompt, nil)
    if err != nil {
        panic(err)
    }

    fmt.Println(result.FinalResponse)
}
```

`NewClient` uses its `context.Context` for initialization requests (`initialize`/`initialized`).
After `NewClient` returns successfully, any spawned stdio app-server lifetime is managed by `Close`, so canceling the constructor context later does not terminate the process.

## Local and Remote Servers

Use `WithStdio` to start a local `codex app-server` over stdio. Use functional options to control the binary, config overrides, process environment, working directory, and stderr capture.

```go
client, err := codex.NewClient(ctx,
    codex.WithStdio(),
    codex.WithCodexPath("codex"),
    codex.WithConfigOverrides(
        `model="gpt-5-codex"`,
        `sandbox_mode="workspace-write"`,
    ),
    codex.WithEnv("CODEX_HOME=/tmp/codex-home"),
    codex.WithCwd("/path/to/workspace"),
    codex.WithStderr(os.Stderr),
)
```

Use `WithWs` or `WithUnix` to connect to an already-running app-server.

```go
client, err := codex.NewClient(ctx,
    codex.WithWs("ws://127.0.0.1:4500"),
    codex.WithWsToken("capability-token"),
)

client, err = codex.NewClient(ctx, codex.WithUnix("/tmp/codex.sock"))
```

Use `NewServer` when Go should manage a shared app-server process. Select the server transport with `WithWs` or `WithUnix`.

```go
server, err := codex.NewServer(ctx,
    codex.WithWs("ws://127.0.0.1:0"),
    codex.WithWsAuthMode("capability-token"),
    codex.WithWsTokenFile("/tmp/codex-ws-token"),
    codex.WithCodexPath("codex"),
    codex.WithCwd("/path/to/workspace"),
)
if err != nil {
    panic(err)
}
defer server.Close()

client, err := codex.NewClient(ctx, codex.WithWs(server.URL()), codex.WithWsToken("capability-token"))
```

For tests and custom embedding, pass an explicit `rpc.Transport` with `WithTransport`.

## Streaming

Use `RunStreamed` to receive notifications as the turn progresses.

```go
prompt := "Inspect the repo"
stream, err := thread.RunStreamed(ctx, []codex.Input{codex.TextInput(prompt)}, nil)
if err != nil {
    panic(err)
}

defer stream.Close()

for {
    note, err := stream.Next(ctx)
    if err != nil {
        break
    }
    fmt.Printf("%s\n", note.Method)
    if note.Method == "turn/completed" {
        break
    }
}
```

`RunStreamed` returns thread-scoped events plus notifications that omit `threadId` (for example account/session updates) so global events are not silently dropped.

## Turn handles

Use `StartTurn` when you need to steer or interrupt a running turn.

```go
handle, err := thread.StartTurn(ctx, []codex.Input{codex.TextInput("Inspect the repo")}, nil)
if err != nil {
    panic(err)
}

if _, err := handle.Steer(ctx, []codex.Input{codex.TextInput("Focus on tests")}); err != nil {
    panic(err)
}

result, err := handle.Run(ctx)
if err != nil {
    panic(err)
}

fmt.Println(result.FinalResponse)
```

`TurnHandle` owns its notification subscription. Call `Close` if you stop before `Run` returns.

## Account, models, and threads

High-level helpers wrap common app-server operations without requiring direct JSON-RPC calls.

```go
account, err := client.Account(ctx, codex.AccountOptions{})
models, err := client.ListModels(ctx, codex.ListModelsOptions{})
threads, err := client.ListThreads(ctx, codex.ThreadListOptions{})
```

Thread values also expose lifecycle helpers:

```go
if _, err := thread.SetName(ctx, "Investigation"); err != nil {
    panic(err)
}

forked, _, err := thread.Fork(ctx, codex.ThreadForkOptions{})
if err != nil {
    panic(err)
}

_ = forked
```

For lower-level or less stable protocol features, use `client.Client()` and the generated `rpc` package.

## Approvals

Configure approval handling by supplying a handler when constructing the client.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
client, err := codex.NewClient(ctx,
    codex.WithStdio(),
    codex.WithLogger(logger),
    codex.WithApprovalHandler(codex.AutoApproveHandler{Logger: logger}),
)
```

For custom approval logic, implement `rpc.ServerRequestHandler` (from `rpc`).

## Structured Output

Provide a JSON Schema to constrain the final assistant message.

```go
prompt := "Summarize repo status"
schema := codex.MustJSON(map[string]any{
    "type": "object",
    "properties": map[string]any{
        "summary": map[string]any{"type": "string"},
        "status": map[string]any{"type": "string", "enum": []string{"ok", "action_required"}},
    },
    "required": []string{"summary", "status"},
    "additionalProperties": false,
})

_, err := thread.RunInputs(ctx, []codex.Input{codex.TextInput(prompt)}, &codex.TurnOptions{
    OutputSchema: schema,
})
```

## JSON-typed options

Fields like `ApprovalPolicy`, `SandboxPolicy`, `Effort`, `Summary`, and `OutputSchema` accept any JSON-marshalable value. If you already have raw JSON, pass a `json.RawMessage` (or `codex.MustJSON(...)`) to avoid double encoding.

For common values, prefer typed constants from this package:

- `codex.ApprovalPolicyNever`, `codex.ApprovalPolicyOnFailure`, `codex.ApprovalPolicyOnRequest`, `codex.ApprovalPolicyUntrusted`
- `codex.SandboxModeReadOnly`, `codex.SandboxModeWorkspaceWrite`, `codex.SandboxModeDangerFullAccess`
- `codex.ReasoningEffortNone`, `codex.ReasoningEffortMinimal`, `codex.ReasoningEffortLow`, `codex.ReasoningEffortMedium`, `codex.ReasoningEffortHigh`, `codex.ReasoningEffortXHigh`

## Inputs and retryable errors

Use helpers to build structured inputs:

```go
inputs := []codex.Input{
    codex.TextInput("Inspect this file"),
    codex.MentionInput("AGENTS.md"),
}
```

Retry classification uses ordinary Go errors:

```go
if codex.IsRetryable(err) {
    // Retry according to your caller policy.
}
```

## Low-level RPC

Use the RPC client directly for full control.

```go
rpcClient := client.Client()
models, err := rpcClient.ModelList(ctx, protocol.ModelListParams{})
```
