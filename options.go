package codex

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/openai/codex/sdk/go/protocol"
	"github.com/openai/codex/sdk/go/rpc"
)

type clientTransportKind int

const (
	clientTransportStdio clientTransportKind = iota
	clientTransportWebsocket
	clientTransportUnix
	clientTransportCustom
)

type optionConfig struct {
	transportKind             clientTransportKind
	transport                 rpc.Transport
	websocketURL              string
	unixPath                  string
	wsAuthMode                string
	wsTokenFile               string
	wsToken                   string
	header                    http.Header
	codexPath                 string
	configOverrides           []string
	extraArgs                 []string
	env                       []string
	cwd                       string
	stderr                    io.Writer
	logger                    *slog.Logger
	clientInfo                protocol.ClientInfo
	disableExperimentalAPI    bool
	optOutNotificationMethods []string
	mcpOpenAIFormElicitation  bool
	requestAttestation        bool
	approvalHandler           rpc.ServerRequestHandler
}

// Option configures clients and servers created by this package.
type Option interface {
	applyCodexOption(*optionConfig)
}

type optionFunc func(*optionConfig)

func (f optionFunc) applyCodexOption(config *optionConfig) {
	f(config)
}

// WithStdio configures NewClient to spawn and connect to codex app-server over stdio.
func WithStdio() Option {
	return optionFunc(func(config *optionConfig) {
		config.transportKind = clientTransportStdio
	})
}

// WithWs configures NewClient to connect to an existing WebSocket app-server, or NewServer to listen on a WebSocket URL.
func WithWs(url string) Option {
	return optionFunc(func(config *optionConfig) {
		config.transportKind = clientTransportWebsocket
		config.websocketURL = url
	})
}

// WithUnix configures NewClient to connect to an existing Unix socket app-server.
func WithUnix(path string) Option {
	return optionFunc(func(config *optionConfig) {
		config.transportKind = clientTransportUnix
		config.unixPath = path
	})
}

// WithWsAuthMode sets the websocket auth mode for spawned WebSocket app-server processes.
func WithWsAuthMode(mode string) Option {
	return optionFunc(func(config *optionConfig) {
		config.wsAuthMode = mode
	})
}

// WithWsTokenFile sets the capability-token file for spawned WebSocket app-server processes.
func WithWsTokenFile(path string) Option {
	return optionFunc(func(config *optionConfig) {
		config.wsTokenFile = path
	})
}

// WithWsToken sets the bearer token used when connecting to WebSocket app-servers.
func WithWsToken(token string) Option {
	return optionFunc(func(config *optionConfig) {
		config.wsToken = token
	})
}

// WithTransport injects a custom transport, primarily for tests.
func WithTransport(transport rpc.Transport) Option {
	return optionFunc(func(config *optionConfig) {
		config.transportKind = clientTransportCustom
		config.transport = transport
	})
}

// WithCodexPath sets the codex binary path used for spawned processes.
func WithCodexPath(path string) Option {
	return optionFunc(func(config *optionConfig) {
		config.codexPath = path
	})
}

// WithCwd sets the spawned process working directory.
func WithCwd(cwd string) Option {
	return optionFunc(func(config *optionConfig) {
		config.cwd = cwd
	})
}

// WithEnv appends environment variables for spawned processes.
func WithEnv(env ...string) Option {
	return optionFunc(func(config *optionConfig) {
		config.env = append(config.env, env...)
	})
}

// WithConfigOverride appends one --config override for spawned app-server processes.
func WithConfigOverride(override string) Option {
	return optionFunc(func(config *optionConfig) {
		config.configOverrides = append(config.configOverrides, override)
	})
}

// WithConfigOverrides appends --config overrides for spawned app-server processes.
func WithConfigOverrides(overrides ...string) Option {
	return optionFunc(func(config *optionConfig) {
		config.configOverrides = append(config.configOverrides, overrides...)
	})
}

// WithExtraArgs appends raw command-line args for spawned app-server processes.
func WithExtraArgs(args ...string) Option {
	return optionFunc(func(config *optionConfig) {
		config.extraArgs = append(config.extraArgs, args...)
	})
}

// WithStderr sets stderr capture for spawned app-server processes.
func WithStderr(writer io.Writer) Option {
	return optionFunc(func(config *optionConfig) {
		config.stderr = writer
	})
}

// WithLogger sets the SDK logger.
func WithLogger(logger *slog.Logger) Option {
	return optionFunc(func(config *optionConfig) {
		config.logger = logger
	})
}

// WithClientInfo sets app-server initialize client metadata.
func WithClientInfo(info protocol.ClientInfo) Option {
	return optionFunc(func(config *optionConfig) {
		config.clientInfo = info
	})
}

// WithApprovalHandler sets the server request handler for client connections.
func WithApprovalHandler(handler rpc.ServerRequestHandler) Option {
	return optionFunc(func(config *optionConfig) {
		config.approvalHandler = handler
	})
}

// WithExperimentalAPI enables or disables experimental app-server API access.
func WithExperimentalAPI(enabled bool) Option {
	return optionFunc(func(config *optionConfig) {
		config.disableExperimentalAPI = !enabled
	})
}

// WithOptOutNotifications suppresses exact server notification methods for this connection.
func WithOptOutNotifications(methods ...string) Option {
	return optionFunc(func(config *optionConfig) {
		config.optOutNotificationMethods = append(config.optOutNotificationMethods, methods...)
	})
}

// WithMCPServerOpenAIFormElicitation advertises OpenAI MCP form elicitation support.
func WithMCPServerOpenAIFormElicitation(enabled bool) Option {
	return optionFunc(func(config *optionConfig) {
		config.mcpOpenAIFormElicitation = enabled
	})
}

// WithRequestAttestation advertises app-server attestation request support.
func WithRequestAttestation(enabled bool) Option {
	return optionFunc(func(config *optionConfig) {
		config.requestAttestation = enabled
	})
}

// WithHeader appends one HTTP header value for WebSocket handshakes.
func WithHeader(name string, value string) Option {
	return optionFunc(func(config *optionConfig) {
		if config.header == nil {
			config.header = http.Header{}
		}
		config.header.Add(name, value)
	})
}

// WithHeaders appends HTTP headers for WebSocket handshakes.
func WithHeaders(header http.Header) Option {
	return optionFunc(func(config *optionConfig) {
		if config.header == nil {
			config.header = http.Header{}
		}
		for name, values := range header {
			for _, value := range values {
				config.header.Add(name, value)
			}
		}
	})
}
