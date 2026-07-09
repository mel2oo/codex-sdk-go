package rpc

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// WebSocketDialOptions configures a WebSocket app-server connection.
type WebSocketDialOptions struct {
	// Header contains optional HTTP headers for the opening handshake.
	Header http.Header
	// Dialer overrides the default network dialer.
	Dialer *net.Dialer
	// TLSConfig overrides TLS settings for wss:// URLs.
	TLSConfig *tls.Config
}

// WebSocketTransport wraps an app-server WebSocket connection.
type WebSocketTransport struct {
	conn   net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
}

// DialWebSocket connects to a running app-server WebSocket listener.
func DialWebSocket(ctx context.Context, rawURL string, options WebSocketDialOptions) (*WebSocketTransport, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
		return nil, fmt.Errorf("unsupported websocket URL scheme %q", parsed.Scheme)
	}
	host := parsed.Host
	if host == "" {
		return nil, errors.New("websocket URL host is empty")
	}
	address := host
	if !strings.Contains(host, ":") {
		if parsed.Scheme == "wss" {
			address = net.JoinHostPort(host, "443")
		} else {
			address = net.JoinHostPort(host, "80")
		}
	}

	dialer := options.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme == "wss" {
		tlsConfig := options.TLSConfig
		if tlsConfig == nil {
			tlsConfig = &tls.Config{ServerName: parsed.Hostname()}
		} else {
			tlsConfig = tlsConfig.Clone()
			if tlsConfig.ServerName == "" {
				tlsConfig.ServerName = parsed.Hostname()
			}
		}
		tlsConn := tls.Client(conn, tlsConfig)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	key, err := websocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := writeWebSocketHandshake(conn, parsed, key, options.Header); err != nil {
		_ = conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := validateWebSocketHandshake(response, key); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &WebSocketTransport{conn: conn, reader: reader}, nil
}

// DialUnix connects to a running app-server Unix socket listener.
func DialUnix(ctx context.Context, socketPath string, options WebSocketDialOptions) (*WebSocketTransport, error) {
	if socketPath == "" {
		return nil, errors.New("unix socket path is empty")
	}
	dialer := options.Dialer
	if dialer == nil {
		dialer = &net.Dialer{}
	}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return nil, err
	}
	parsed := &url.URL{Scheme: "ws", Host: "localhost", Path: "/"}
	key, err := websocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := writeWebSocketHandshake(conn, parsed, key, options.Header); err != nil {
		_ = conn.Close()
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := validateWebSocketHandshake(response, key); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &WebSocketTransport{conn: conn, reader: reader}, nil
}

// ReadLine reads one app-server JSON-RPC WebSocket text frame.
func (t *WebSocketTransport) ReadLine() (string, error) {
	for {
		opcode, payload, err := t.readFrame()
		if err != nil {
			return "", err
		}
		switch opcode {
		case websocketOpcodeText:
			return string(payload), nil
		case websocketOpcodePing:
			_ = t.writeFrame(websocketOpcodePong, payload)
		case websocketOpcodePong:
			continue
		case websocketOpcodeClose:
			_ = t.writeFrame(websocketOpcodeClose, nil)
			return "", io.EOF
		default:
			return "", fmt.Errorf("unsupported websocket frame opcode %d", opcode)
		}
	}
}

// WriteLine writes one app-server JSON-RPC WebSocket text frame.
func (t *WebSocketTransport) WriteLine(line string) error {
	return t.writeFrame(websocketOpcodeText, []byte(line))
}

// Close closes the WebSocket connection.
func (t *WebSocketTransport) Close() error {
	_ = t.writeFrame(websocketOpcodeClose, nil)
	return t.conn.Close()
}

const (
	websocketOpcodeText  = 0x1
	websocketOpcodeClose = 0x8
	websocketOpcodePing  = 0x9
	websocketOpcodePong  = 0xA
)

func websocketKey() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data[:]), nil
}

func writeWebSocketHandshake(conn net.Conn, parsed *url.URL, key string, header http.Header) error {
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	var b strings.Builder
	b.WriteString("GET ")
	b.WriteString(path)
	b.WriteString(" HTTP/1.1\r\n")
	b.WriteString("Host: ")
	b.WriteString(parsed.Host)
	b.WriteString("\r\n")
	b.WriteString("Upgrade: websocket\r\n")
	b.WriteString("Connection: Upgrade\r\n")
	b.WriteString("Sec-WebSocket-Key: ")
	b.WriteString(key)
	b.WriteString("\r\n")
	b.WriteString("Sec-WebSocket-Version: 13\r\n")
	for name, values := range header {
		if isWebSocketReservedHeader(name) {
			continue
		}
		for _, value := range values {
			b.WriteString(name)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString("\r\n")
		}
	}
	b.WriteString("\r\n")
	_, err := io.WriteString(conn, b.String())
	return err
}

func isWebSocketReservedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "host", "upgrade", "connection", "sec-websocket-key", "sec-websocket-version", "sec-websocket-accept":
		return true
	default:
		return false
	}
}

func validateWebSocketHandshake(response *http.Response, key string) error {
	if response.StatusCode != http.StatusSwitchingProtocols {
		return fmt.Errorf("websocket upgrade failed: %s", response.Status)
	}
	if !headerTokenContains(response.Header, "Upgrade", "websocket") {
		return errors.New("websocket upgrade response missing Upgrade: websocket")
	}
	if !headerTokenContains(response.Header, "Connection", "upgrade") {
		return errors.New("websocket upgrade response missing Connection: Upgrade")
	}
	expected := websocketAccept(key)
	if got := response.Header.Get("Sec-WebSocket-Accept"); got != expected {
		return fmt.Errorf("websocket accept mismatch: got %q want %q", got, expected)
	}
	return nil
}

func headerTokenContains(header http.Header, name string, want string) bool {
	for _, value := range header.Values(name) {
		for _, token := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), want) {
				return true
			}
		}
	}
	return false
}

func websocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + websocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func (t *WebSocketTransport) readFrame() (byte, []byte, error) {
	first, err := t.reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	second, err := t.reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}

	fin := first&0x80 != 0
	opcode := first & 0x0f
	masked := second&0x80 != 0
	length := uint64(second & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(t.reader, buf[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(t.reader, buf[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(buf[:])
	}
	if length > uint64(^uint(0)>>1) {
		return 0, nil, errors.New("websocket frame too large")
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(t.reader, mask[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, int(length))
	if _, err := io.ReadFull(t.reader, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}

	if !fin {
		return 0, nil, errors.New("fragmented websocket frames are not supported")
	}
	return opcode, payload, nil
}

func (t *WebSocketTransport) writeFrame(opcode byte, payload []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var header []byte
	first := byte(0x80) | opcode
	length := len(payload)
	switch {
	case length < 126:
		header = []byte{first, 0x80 | byte(length)}
	case length <= 65535:
		header = make([]byte, 4)
		header[0] = first
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:], uint16(length))
	default:
		header = make([]byte, 10)
		header[0] = first
		header[1] = 0x80 | 127
		binary.BigEndian.PutUint64(header[2:], uint64(length))
	}

	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	header = append(header, mask[:]...)
	masked := make([]byte, len(payload))
	for i, b := range payload {
		masked[i] = b ^ mask[i%4]
	}

	if _, err := t.conn.Write(header); err != nil {
		return err
	}
	_, err := t.conn.Write(masked)
	return err
}
