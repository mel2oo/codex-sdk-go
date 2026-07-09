// Package codex provides an idiomatic Go SDK for the Codex app-server.
//
// The SDK can spawn a stdio `codex app-server`, connect to an existing
// WebSocket or Unix socket app-server, or manage a standalone WebSocket/Unix
// app-server process. It exposes a high-level facade for accounts, models,
// threads, turns, and streaming turn control. For lower-level access, you can
// reach the JSON-RPC client via (*Client).Client().
//
// Typical usage:
//
//	ctx := context.Background()
//	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
//	prompt := "Diagnose the test failure and propose a fix"
//	client, err := codex.NewClient(ctx, codex.WithStdio(), codex.WithLogger(logger))
//	if err != nil {
//		panic(err)
//	}
//	defer client.Close()
//
// The constructor context is used for initialization only. Once NewClient returns
// successfully, the spawned app-server lifetime is managed by Close.
//
// To connect to an already running server:
//
//	client, err := codex.NewClient(ctx, codex.WithWs("ws://127.0.0.1:1456"))
//	client, err := codex.NewClient(ctx, codex.WithUnix("/tmp/codex.sock"))
//
// To manage a shared app-server process:
//
//	server, err := codex.NewServer(ctx, codex.WithWs("ws://127.0.0.1:0"), codex.WithCodexPath("codex"))
//	if err != nil {
//		panic(err)
//	}
//	defer server.Close()
//	client, err := codex.NewClient(ctx, codex.WithWs(server.URL()))
//
//	thread, err := client.StartThread(ctx, codex.ThreadStartOptions{})
//	if err != nil {
//		panic(err)
//	}
//
//	result, err := thread.Run(ctx, prompt, nil)
//	if err != nil {
//		panic(err)
//	}
//	fmt.Println(result.FinalResponse)
//
// For a running turn that needs steering or interruption, start a turn handle:
//
//	handle, err := thread.StartTurn(ctx, []codex.Input{codex.TextInput("Inspect the repo")}, nil)
//	if err != nil {
//		panic(err)
//	}
//	defer handle.Close()
//	_, err = handle.Steer(ctx, []codex.Input{codex.TextInput("Focus on tests")})
//	if err != nil {
//		panic(err)
//	}
//	result, err = handle.Run(ctx)
//	if err != nil {
//		panic(err)
//	}
//
// Account, model, and thread lifecycle helpers cover common app-server calls:
//
//	account, err := client.Account(ctx, codex.AccountOptions{})
//	models, err := client.ListModels(ctx, codex.ListModelsOptions{})
//	threads, err := client.ListThreads(ctx, codex.ThreadListOptions{})
//	_ = account
//	_ = models
//	_ = threads
//
// JSON-typed options (approval policies, sandbox policies, output schemas, etc.)
// accept any JSON-marshalable value. If you already have raw JSON, pass
// json.RawMessage or codex.MustJSON(...) to avoid double encoding.
//
// For common values, prefer typed constants:
//   - codex.ApprovalPolicyNever / codex.ApprovalPolicyOnRequest / ...
//   - codex.SandboxModeReadOnly / codex.SandboxModeWorkspaceWrite / ...
//   - codex.ReasoningEffortLow / codex.ReasoningEffortMedium / ...
//
// Retryable overload errors can be detected with codex.IsRetryable or
// codex.IsOverloaded. Both helpers work with wrapped errors.
package codex
