package codex

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strings"
	"time"

	"github.com/openai/codex/sdk/go/protocol"
	"github.com/openai/codex/sdk/go/rpc"
)

const codexVersionProbeTimeout = 2 * time.Second

// Client is the main entrypoint for the Go SDK.
type Client struct {
	client *rpc.Client
	logger *slog.Logger
}

// NewClient creates a Codex client and performs the initialize handshake.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	config := optionConfig{transportKind: clientTransportStdio}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCodexOption(&config)
		}
	}
	logger := resolveLogger(config.logger)

	transport, err := clientTransport(ctx, config, logger)
	if err != nil {
		return nil, err
	}

	client := rpc.NewClient(transport, rpc.ClientOptions{
		Logger:         logger,
		RequestHandler: attachApprovalLogger(config.approvalHandler, logger),
	})

	initOptions := initializeOptions{
		ClientInfo:                     config.clientInfo,
		DisableExperimentalAPI:         config.disableExperimentalAPI,
		OptOutNotificationMethods:      config.optOutNotificationMethods,
		MCPServerOpenAIFormElicitation: config.mcpOpenAIFormElicitation,
		RequestAttestation:             config.requestAttestation,
	}
	if err := initializeClient(ctx, client, initOptions); err != nil {
		_ = client.Close()
		return nil, err
	}

	logger.Info("codex initialized")

	return &Client{client: client, logger: logger}, nil
}

func clientTransport(ctx context.Context, config optionConfig, logger *slog.Logger) (rpc.Transport, error) {
	switch config.transportKind {
	case clientTransportCustom:
		if config.transport == nil {
			return nil, errors.New("custom transport is required")
		}
		logger.Info("codex using custom transport")
		return config.transport, nil
	case clientTransportWebsocket:
		return rpc.DialWebSocket(ctx, config.websocketURL, rpc.WebSocketDialOptions{Header: websocketHeader(config)})
	case clientTransportUnix:
		return rpc.DialUnix(ctx, config.unixPath, rpc.WebSocketDialOptions{Header: config.header})
	case clientTransportStdio:
		codexPath := config.codexPath
		if codexPath == "" {
			codexPath = "codex"
		}
		args := appServerArgs(config, "")
		logger.Info("codex starting app-server", "path", codexPath, "args", strings.Join(args, " "))
		warnIfCodexVersionMismatch(ctx, logger, codexPath)
		stderr := config.stderr
		if stderr == nil {
			stderr = rpc.DefaultStderr()
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// The constructor context is only for initialization; process lifetime is managed by Close.
		return rpc.SpawnStdioWithOptions(context.WithoutCancel(ctx), codexPath, args, rpc.SpawnStdioOptions{
			Stderr: stderr,
			Env:    config.env,
			Cwd:    config.cwd,
		})
	}
	return nil, errors.New("client transport is required")
}

func appServerArgs(config optionConfig, listen string) []string {
	args := []string{"app-server"}
	if listen != "" {
		args = append(args, "--listen", listen)
	}
	if config.wsAuthMode != "" {
		args = append(args, "--ws-auth", config.wsAuthMode)
	}
	if config.wsTokenFile != "" {
		args = append(args, "--ws-token-file", config.wsTokenFile)
	}
	for _, override := range config.configOverrides {
		args = append(args, "--config", override)
	}
	args = append(args, config.extraArgs...)
	return args
}

func websocketHeader(config optionConfig) http.Header {
	if config.header == nil && config.wsToken == "" {
		return nil
	}
	header := http.Header{}
	for name, values := range config.header {
		for _, value := range values {
			header.Add(name, value)
		}
	}
	if config.wsToken != "" {
		header.Set("Authorization", "Bearer "+config.wsToken)
	}
	return header
}

type initializeOptions struct {
	ClientInfo                     protocol.ClientInfo
	DisableExperimentalAPI         bool
	OptOutNotificationMethods      []string
	MCPServerOpenAIFormElicitation bool
	RequestAttestation             bool
}

func initializeClient(ctx context.Context, client *rpc.Client, opts initializeOptions) error {
	info := opts.ClientInfo
	if info.Name == "" {
		info = defaultClientInfo()
	}
	capabilities := protocol.InitializeCapabilities{
		ExperimentalApi:           !opts.DisableExperimentalAPI,
		OptOutNotificationMethods: opts.OptOutNotificationMethods,
		RequestAttestation:        opts.RequestAttestation,
	}
	if opts.MCPServerOpenAIFormElicitation {
		capabilities.MCPServerOpenaiFormElicitation = boolPtr(true)
	}
	if _, err := client.Initialize(ctx, protocol.InitializeParams{
		ClientInfo:   info,
		Capabilities: capabilities,
	}); err != nil {
		return err
	}
	return client.Notify(ctx, "initialized", nil)
}

// Client exposes the underlying RPC client for low-level access.
func (c *Client) Client() *rpc.Client {
	return c.client
}

// Close closes the underlying transport.
func (c *Client) Close() error {
	if err := c.ensureReady(); err != nil {
		return err
	}
	return c.client.Close()
}

// StartThread starts a new thread using the app-server.
func (c *Client) StartThread(ctx context.Context, options ThreadStartOptions) (*Thread, error) {
	if err := c.ensureReady(); err != nil {
		return nil, err
	}
	params, err := options.toParams()
	if err != nil {
		return nil, err
	}
	var response protocol.ThreadStartResponse
	if err := c.client.Call(ctx, "thread/start", params, &response); err != nil {
		return nil, err
	}
	threadID, err := threadIDFromResponse(response.ThreadID, response.Thread)
	if err != nil {
		return nil, err
	}
	c.logger.Info("codex thread started", "thread_id", threadID)
	return &Thread{client: c.client, id: threadID, logger: c.logger}, nil
}

// ResumeThread resumes an existing thread.
func (c *Client) ResumeThread(ctx context.Context, options ThreadResumeOptions) (*Thread, error) {
	if err := c.ensureReady(); err != nil {
		return nil, err
	}
	params, err := options.toParams()
	if err != nil {
		return nil, err
	}
	var response protocol.ThreadResumeResponse
	if err := c.client.Call(ctx, "thread/resume", params, &response); err != nil {
		return nil, err
	}
	threadID, err := threadIDFromResponse(response.ThreadID, response.Thread)
	if err != nil {
		return nil, err
	}
	c.logger.Info("codex thread resumed", "thread_id", threadID)
	return &Thread{client: c.client, id: threadID, logger: c.logger}, nil
}

func defaultClientInfo() protocol.ClientInfo {
	version := "dev"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		version = info.Main.Version
	}
	return protocol.ClientInfo{
		Name:    "codex-go-sdk",
		Title:   stringPtr("Codex Go SDK"),
		Version: version,
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func warnIfCodexVersionMismatch(ctx context.Context, logger *slog.Logger, codexPath string) {
	generatedVersion := protocol.GeneratedCodexVersion
	if generatedVersion == "" {
		return
	}
	runtimeVersion, err := probeCodexVersion(ctx, codexPath)
	if err != nil {
		logger.Warn(
			"codex binary version could not be verified",
			"path", codexPath,
			"generated_version", generatedVersion,
			"generated_commit", protocol.GeneratedCodexCommit,
			"error", err,
		)
		return
	}
	if runtimeVersion == "" {
		logger.Warn(
			"codex binary version could not be verified",
			"path", codexPath,
			"generated_version", generatedVersion,
			"generated_commit", protocol.GeneratedCodexCommit,
			"error", "version unavailable",
		)
		return
	}
	if runtimeVersion == generatedVersion {
		return
	}
	logger.Warn(
		"codex binary version differs from generated protocol version",
		"path", codexPath,
		"runtime_version", runtimeVersion,
		"generated_version", generatedVersion,
		"generated_commit", protocol.GeneratedCodexCommit,
	)
}

func probeCodexVersion(parent context.Context, codexPath string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, codexVersionProbeTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, codexPath, "--version").Output()
	if err != nil {
		return "", err
	}
	return parseCodexVersionOutput(string(out)), nil
}

func parseCodexVersionOutput(output string) string {
	for _, field := range strings.Fields(output) {
		field = strings.TrimPrefix(field, "v")
		if isDottedVersion(field) {
			return field
		}
	}
	return ""
}

func isDottedVersion(value string) bool {
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func threadIDFromResponse(threadID string, thread *protocol.Thread) (string, error) {
	if threadID != "" {
		return threadID, nil
	}
	if thread != nil && thread.ID != "" {
		return thread.ID, nil
	}
	return "", errors.New("thread id not found in response")
}

func (c *Client) ensureReady() error {
	if c == nil {
		return errors.New("codex client is nil")
	}
	if c.client == nil {
		return errors.New("codex client is not initialized")
	}
	return nil
}
