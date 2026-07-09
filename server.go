package codex

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

const serverCloseTimeout = 2 * time.Second

// Server manages a spawned codex app-server process.
type Server struct {
	cmd        *exec.Cmd
	url        string
	socketPath string
}

// URL returns the WebSocket URL for a WebSocket app-server.
func (s *Server) URL() string {
	if s == nil {
		return ""
	}
	return s.url
}

// SocketPath returns the Unix socket path for a Unix app-server.
func (s *Server) SocketPath() string {
	if s == nil {
		return ""
	}
	return s.socketPath
}

// Close terminates the spawned app-server process.
func (s *Server) Close() error {
	if s == nil || s.cmd == nil {
		return nil
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- s.cmd.Wait()
	}()
	select {
	case err := <-waitCh:
		return err
	case <-time.After(serverCloseTimeout):
		if s.cmd.Process != nil {
			if err := s.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return err
			}
		}
		<-waitCh
		return nil
	}
}

// NewServer starts a standalone codex app-server with the requested transport option.
func NewServer(ctx context.Context, opts ...Option) (*Server, error) {
	config := optionConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.applyCodexOption(&config)
		}
	}
	listen, url, socketPath, err := serverTransport(config)
	if err != nil {
		return nil, err
	}
	logger := resolveLogger(config.logger)
	codexPath := config.codexPath
	if codexPath == "" {
		codexPath = "codex"
	}
	args := appServerArgs(config, listen)
	logger.Info("codex starting app-server", "path", codexPath, "args", strings.Join(args, " "))
	warnIfCodexVersionMismatch(ctx, logger, codexPath)

	cmd := exec.CommandContext(context.WithoutCancel(ctx), codexPath, args...)
	cmd.Dir = config.cwd
	if len(config.env) > 0 {
		cmd.Env = append(os.Environ(), config.env...)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	stderr := config.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	server := &Server{
		cmd:        cmd,
		url:        url,
		socketPath: socketPath,
	}
	urlCh := make(chan string, 1)
	go copyServerStderr(stderrPipe, stderr, urlCh)
	if server.url == "ws://127.0.0.1:0" || server.url == "ws://localhost:0" {
		select {
		case url := <-urlCh:
			server.url = url
		case <-ctx.Done():
			_ = server.Close()
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			_ = server.Close()
			return nil, errors.New("timed out waiting for websocket app-server URL")
		}
	}
	return server, nil
}

func serverTransport(config optionConfig) (listen string, url string, socketPath string, err error) {
	switch config.transportKind {
	case clientTransportWebsocket:
		return config.websocketURL, config.websocketURL, "", nil
	case clientTransportUnix:
		return "unix://" + config.unixPath, "", config.unixPath, nil
	case clientTransportStdio, clientTransportCustom:
		return "", "", "", errors.New("server transport option is required")
	}
	return "", "", "", errors.New("server transport option is required")
}

var websocketURLPattern = regexp.MustCompile(`ws://[^\s]+`)

func copyServerStderr(reader io.Reader, writer io.Writer, urlCh chan<- string) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if writer != nil {
			_, _ = fmt.Fprintln(writer, line)
		}
		if match := websocketURLPattern.FindString(line); match != "" {
			select {
			case urlCh <- match:
			default:
			}
		}
	}
	if err := scanner.Err(); err != nil && writer != nil {
		_, _ = fmt.Fprintf(writer, "error reading codex app-server stderr: %v\n", err)
	}
}
