package codex

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openai/codex/sdk/go/protocol"
	"github.com/openai/codex/sdk/go/rpc"
)

func TestNewClientWithWsInitializesAndUsesGeneratedRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- serveWebSocketFixture(listener)
	}()

	ctx := context.Background()
	client, err := NewClient(ctx,
		WithWs("ws://"+listener.Addr().String()),
		WithHeader("X-Codex-Test", "websocket"),
		WithWsToken("secret-token"),
		WithClientInfo(protocol.ClientInfo{Name: "codex-go-test", Version: "test"}),
	)
	if err != nil {
		t.Fatalf("new websocket client: %v", err)
	}
	defer client.Close()

	account, err := client.Account(ctx, AccountOptions{})
	if err != nil {
		t.Fatalf("account over websocket: %v", err)
	}
	if account == nil || account.RequiresOpenaiAuth {
		t.Fatalf("unexpected account response: %#v", account)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("websocket fixture: %v", err)
	}
}

func TestNewClientWithTransportOverridesEarlierTransportOptions(t *testing.T) {
	info := protocol.ClientInfo{Name: "codex-go-test", Version: "test"}
	client, err := NewClient(context.Background(),
		WithStdio(),
		WithTransport(rpc.NewReplayTransport([]rpc.TranscriptEntry{
			writeLine(rpc.JSONRPCRequest{ID: rpc.NewIntRequestID(1), Method: "initialize", Params: mustRaw(initializeParams(info))}),
			readLine(rpc.JSONRPCResponse{ID: rpc.NewIntRequestID(1), Result: mustRaw(map[string]any{})}),
			writeLine(rpc.JSONRPCNotification{Method: "initialized"}),
		})),
		WithClientInfo(info),
	)
	if err != nil {
		t.Fatalf("new stdio client: %v", err)
	}
	defer client.Close()
}

func TestNewServerRequiresExplicitTransport(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	server, err := NewServer(context.Background(),
		WithWs("ws://127.0.0.1:0"),
		WithWsAuthMode("capability-token"),
		WithWsTokenFile("/tmp/codex-ws-token"),
		WithCodexPath(writeFakeWebsocketServerBinary(t)),
		WithEnv("CODEX_GO_SDK_ARGS_FILE="+argsFile),
		WithStderr(io.Discard),
	)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if server.URL() == "" {
		t.Fatalf("expected websocket server URL")
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	want := "app-server\n--listen\nws://127.0.0.1:0\n--ws-auth\ncapability-token\n--ws-token-file\n/tmp/codex-ws-token\n"
	if string(args) != want {
		t.Fatalf("unexpected server args:\n%s", args)
	}
}

func TestNewServerWithUnixOption(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	socketPath := "/tmp/codex-go-sdk.sock"
	server, err := NewServer(context.Background(),
		WithUnix(socketPath),
		WithCodexPath(writeFakeWebsocketServerBinary(t)),
		WithEnv("CODEX_GO_SDK_ARGS_FILE="+argsFile),
		WithStderr(io.Discard),
	)
	if err != nil {
		t.Fatalf("new unix server: %v", err)
	}
	if server.SocketPath() != socketPath {
		t.Fatalf("unexpected socket path: %q", server.SocketPath())
	}
	if server.URL() != "" {
		t.Fatalf("expected no websocket URL for unix server, got %q", server.URL())
	}
	if err := server.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	want := "app-server\n--listen\nunix:///tmp/codex-go-sdk.sock\n"
	if string(args) != want {
		t.Fatalf("unexpected server args:\n%s", args)
	}
}

func TestNewServerRequiresTransportOption(t *testing.T) {
	_, err := NewServer(context.Background(), WithCodexPath("codex"))
	if err == nil || err.Error() != "server transport option is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCopyServerStderrReportsScannerError(t *testing.T) {
	var output bytes.Buffer
	copyServerStderr(errorReader{}, &output, make(chan string, 1))

	if !strings.Contains(output.String(), "error reading codex app-server stderr: read failed") {
		t.Fatalf("expected scanner error in output, got %q", output.String())
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

func serveWebSocketFixture(listener net.Listener) error {
	conn, err := listener.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}
	if got := request.Header.Get("X-Codex-Test"); got != "websocket" {
		return errors.New("missing websocket test header")
	}
	if got := request.Header.Get("Authorization"); got != "Bearer secret-token" {
		return errors.New("missing websocket auth token")
	}
	key := request.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return errors.New("missing websocket key")
	}
	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + websocketAcceptForTest(key) + "\r\n\r\n"
	if _, err := io.WriteString(conn, response); err != nil {
		return err
	}

	for {
		payload, err := readClientWebSocketPayloadForTest(reader)
		if err != nil {
			return err
		}
		var message struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			return err
		}
		switch message.Method {
		case "initialize":
			if len(message.ID) == 0 {
				return errors.New("initialize request missing id")
			}
			if err := writeServerWebSocketPayloadForTest(conn, MustJSON(map[string]any{
				"id":     json.RawMessage(message.ID),
				"result": map[string]any{},
			})); err != nil {
				return err
			}
		case "initialized":
			continue
		case "account/read":
			if len(message.ID) == 0 {
				return errors.New("account/read request missing id")
			}
			return writeServerWebSocketPayloadForTest(conn, MustJSON(map[string]any{
				"id": message.ID,
				"result": map[string]any{
					"requiresOpenaiAuth": false,
				},
			}))
		default:
			return errors.New("unexpected websocket method: " + message.Method)
		}
	}
}

func readClientWebSocketPayloadForTest(reader *bufio.Reader) ([]byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	second, err := reader.ReadByte()
	if err != nil {
		return nil, err
	}
	if first&0x0f != 0x1 {
		return nil, errors.New("expected websocket text frame")
	}
	length := int(second & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return nil, err
		}
		length = int(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return nil, err
		}
		length = int(binary.BigEndian.Uint64(buf[:]))
	}
	var mask [4]byte
	if _, err := io.ReadFull(reader, mask[:]); err != nil {
		return nil, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return payload, nil
}

func writeServerWebSocketPayloadForTest(writer io.Writer, payload []byte) error {
	length := len(payload)
	var frame []byte
	switch {
	case length < 126:
		frame = []byte{0x81, byte(length)}
	case length <= 65535:
		frame = make([]byte, 4)
		frame[0] = 0x81
		frame[1] = 126
		binary.BigEndian.PutUint16(frame[2:], uint16(length))
	default:
		frame = make([]byte, 10)
		frame[0] = 0x81
		frame[1] = 127
		binary.BigEndian.PutUint64(frame[2:], uint64(length))
	}
	frame = append(frame, payload...)
	_, err := writer.Write(frame)
	return err
}

func websocketAcceptForTest(key string) string {
	const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func writeFakeWebsocketServerBinary(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fake-codex-server")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
	printf 'codex-cli 999.999.999\n'
	exit 0
fi

if [ -n "$CODEX_GO_SDK_ARGS_FILE" ]; then
	for arg in "$@"; do
		printf '%s\n' "$arg" >> "$CODEX_GO_SDK_ARGS_FILE"
	done
fi

printf 'codex app-server (WebSockets)\n' >&2
printf '  listening on: ws://127.0.0.1:43210\n' >&2
sleep 30
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex server: %v", err)
	}
	return path
}
