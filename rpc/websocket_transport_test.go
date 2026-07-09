package rpc

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestWebSocketTransportReadWrite(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		request, err := http.ReadRequest(reader)
		if err != nil {
			serverDone <- err
			return
		}
		key := request.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			serverDone <- errMissingWebSocketKey
			return
		}
		response := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + websocketAccept(key) + "\r\n\r\n"
		if _, err := io.WriteString(conn, response); err != nil {
			serverDone <- err
			return
		}
		if _, err := conn.Write(testWebSocketFrame(websocketOpcodeText, []byte(`{"method":"server"}`))); err != nil {
			serverDone <- err
			return
		}
		opcode, payload, err := readClientWebSocketFrame(reader)
		if err != nil {
			serverDone <- err
			return
		}
		if opcode != websocketOpcodeText || string(payload) != `{"method":"client"}` {
			serverDone <- errUnexpectedWebSocketPayload
			return
		}
		serverDone <- nil
	}()

	transport, err := DialWebSocket(context.Background(), "ws://"+listener.Addr().String(), WebSocketDialOptions{})
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer transport.Close()

	line, err := transport.ReadLine()
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if line != `{"method":"server"}` {
		t.Fatalf("unexpected websocket line: %s", line)
	}
	if err := transport.WriteLine(`{"method":"client"}`); err != nil {
		t.Fatalf("write line: %v", err)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("server error: %v", err)
	}
}

type websocketTestError string

func (err websocketTestError) Error() string {
	return string(err)
}

const (
	errMissingWebSocketKey        websocketTestError = "missing websocket key"
	errUnexpectedWebSocketPayload websocketTestError = "unexpected websocket payload"
)

func testWebSocketFrame(opcode byte, payload []byte) []byte {
	length := len(payload)
	var frame []byte
	switch {
	case length < 126:
		frame = []byte{0x80 | opcode, byte(length)}
	case length <= 65535:
		frame = make([]byte, 4)
		frame[0] = 0x80 | opcode
		frame[1] = 126
		binary.BigEndian.PutUint16(frame[2:], uint16(length))
	default:
		frame = make([]byte, 10)
		frame[0] = 0x80 | opcode
		frame[1] = 127
		binary.BigEndian.PutUint64(frame[2:], uint64(length))
	}
	return append(frame, payload...)
}

func readClientWebSocketFrame(reader *bufio.Reader) (byte, []byte, error) {
	first, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	second, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	opcode := first & 0x0f
	length := int(second & 0x7f)
	switch length {
	case 126:
		var buf [2]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint16(buf[:]))
	case 127:
		var buf [8]byte
		if _, err := io.ReadFull(reader, buf[:]); err != nil {
			return 0, nil, err
		}
		length = int(binary.BigEndian.Uint64(buf[:]))
	}
	var mask [4]byte
	if _, err := io.ReadFull(reader, mask[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return opcode, payload, nil
}
