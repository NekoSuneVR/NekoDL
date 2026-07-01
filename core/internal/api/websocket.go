package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// websocketGUID is the fixed handshake suffix defined by RFC 6455 section 1.3.
const websocketGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

var errCloseReceived = errors.New("websocket: close frame received")

// handleEvents upgrades the connection to a WebSocket (RFC 6455, hand-rolled
// against the Go standard library only — no third-party dependency) and
// pushes the current task list once a second until the client disconnects.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade request", http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key header", http.StatusBadRequest)
		return
	}

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "connection does not support hijacking", http.StatusInternalServerError)
		return
	}

	conn, brw, err := hijacker.Hijack()
	if err != nil {
		return // response writer state is undefined after a failed hijack — nothing more we can safely send
	}
	defer conn.Close()

	if err := writeHandshake(brw.Writer, key); err != nil {
		return
	}

	var writeMu sync.Mutex
	writeFrameSafe := func(opcode byte, payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return writeFrame(brw.Writer, opcode, payload)
	}

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			opcode, payload, err := readFrame(brw.Reader)
			if err != nil {
				return
			}
			if opcode == opPing {
				if err := writeFrameSafe(opPong, payload); err != nil {
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-closed:
			return
		case <-ticker.C:
			data, err := json.Marshal(s.scheduler.Records())
			if err != nil {
				return
			}
			if err := writeFrameSafe(opText, data); err != nil {
				return
			}
		}
	}
}

// writeHandshake sends the HTTP 101 Switching Protocols response that
// completes the WebSocket opening handshake.
func writeHandshake(w *bufio.Writer, clientKey string) error {
	sum := sha1.Sum([]byte(clientKey + websocketGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])

	response := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"

	if _, err := w.WriteString(response); err != nil {
		return err
	}
	return w.Flush()
}

// writeFrame writes one unfragmented WebSocket frame. Per RFC 6455, frames
// sent from server to client must NOT be masked (only client-to-server
// frames are), so the mask bit is always 0 here.
func writeFrame(w *bufio.Writer, opcode byte, payload []byte) error {
	if err := w.WriteByte(0x80 | opcode); err != nil { // FIN=1, opcode
		return err
	}

	n := len(payload)
	switch {
	case n <= 125:
		if err := w.WriteByte(byte(n)); err != nil {
			return err
		}
	case n <= 65535:
		if err := w.WriteByte(126); err != nil {
			return err
		}
		var lenBuf [2]byte
		binary.BigEndian.PutUint16(lenBuf[:], uint16(n))
		if _, err := w.Write(lenBuf[:]); err != nil {
			return err
		}
	default:
		if err := w.WriteByte(127); err != nil {
			return err
		}
		var lenBuf [8]byte
		binary.BigEndian.PutUint64(lenBuf[:], uint64(n))
		if _, err := w.Write(lenBuf[:]); err != nil {
			return err
		}
	}

	if _, err := w.Write(payload); err != nil {
		return err
	}
	return w.Flush()
}

// readFrame reads one WebSocket frame from the client and unmasks its
// payload (client-to-server frames are always masked per RFC 6455). It does
// not reassemble fragmented messages — this endpoint only cares about
// control frames (ping/close) from the client, not data frames.
func readFrame(r *bufio.Reader) (opcode byte, payload []byte, err error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	opcode = header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)

	switch length {
	case 126:
		buf := make([]byte, 2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(buf))
	case 127:
		buf := make([]byte, 8)
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(buf)
	}

	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload = make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}

	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	if opcode == opClose {
		return opcode, payload, errCloseReceived
	}
	return opcode, payload, nil
}
