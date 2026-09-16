package cfprobe

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	wsOpcodeContinuation = 0x0
	wsOpcodeText         = 0x1
	wsOpcodeBinary       = 0x2
	wsOpcodeClose        = 0x8
	wsOpcodePing         = 0x9
	wsOpcodePong         = 0xA

	webSocketGUID           = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	webSocketMaxMessageSize = 1 << 20
)

type webSocketConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

type wsHandshakeError struct {
	StatusCode int
	Headers    http.Header
	Body       string
}

type wsCloseError struct {
	Code   int
	Reason string
}

type wsFrame struct {
	fin     bool
	opcode  byte
	payload []byte
}

func (e *wsHandshakeError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("WSS handshake http=%d", e.StatusCode)
	}
	return fmt.Sprintf("WSS handshake http=%d body=%s", e.StatusCode, strings.TrimSpace(e.Body))
}

func (e *wsCloseError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("WSS close code=%d", e.Code)
	}
	return fmt.Sprintf("WSS close code=%d reason=%s", e.Code, e.Reason)
}

func dialReportWebSocket(ctx context.Context, rawURL string, headers http.Header, usePublicDNS bool, timeout time.Duration) (*webSocketConn, http.Header, time.Time, time.Time, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, nil, time.Time{}, time.Time{}, err
	}
	if u.Host == "" {
		return nil, nil, time.Time{}, time.Time{}, fmt.Errorf("WSS URL missing host")
	}
	if timeout <= 0 {
		timeout = wssHandshakeTimeout
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "ws" && scheme != "wss" {
		return nil, nil, time.Time{}, time.Time{}, fmt.Errorf("unsupported WSS scheme %q", u.Scheme)
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	address := net.JoinHostPort(host, port)

	handshakeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	conn, err := dialReportTCP(handshakeCtx, "tcp", address, timeout, usePublicDNS)
	if err != nil {
		return nil, nil, started, time.Now(), err
	}
	deadline := started.Add(timeout)
	_ = conn.SetDeadline(deadline)
	if scheme == "wss" {
		tlsConn := tls.Client(conn, &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(handshakeCtx); err != nil {
			_ = conn.Close()
			return nil, nil, started, time.Now(), err
		}
		conn = tlsConn
		_ = conn.SetDeadline(deadline)
	}

	key, err := newWebSocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, nil, started, time.Now(), err
	}
	requestURL := *u
	if scheme == "wss" {
		requestURL.Scheme = "https"
	} else {
		requestURL.Scheme = "http"
	}
	req, err := http.NewRequest(http.MethodGet, requestURL.String(), nil)
	if err != nil {
		_ = conn.Close()
		return nil, nil, started, time.Now(), err
	}
	req.Host = u.Host
	req.Header = headers.Clone()
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", key)
	req.Header.Set("Sec-WebSocket-Version", "13")

	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, nil, started, time.Now(), err
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, req)
	received := time.Now()
	if err != nil {
		_ = conn.Close()
		return nil, nil, started, received, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, resp.Header, started, received, &wsHandshakeError{StatusCode: resp.StatusCode, Headers: resp.Header.Clone(), Body: string(body)}
	}
	if !headerHasToken(resp.Header, "Upgrade", "websocket") || !headerHasToken(resp.Header, "Connection", "Upgrade") {
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, resp.Header, started, received, fmt.Errorf("WSS handshake missing upgrade headers")
	}
	if got, want := strings.TrimSpace(resp.Header.Get("Sec-WebSocket-Accept")), expectedWebSocketAccept(key); got != want {
		_ = resp.Body.Close()
		_ = conn.Close()
		return nil, resp.Header, started, received, fmt.Errorf("WSS handshake invalid accept")
	}
	_ = conn.SetDeadline(time.Time{})
	return &webSocketConn{conn: conn, reader: reader}, resp.Header, started, received, nil
}

func dialReportTCP(ctx context.Context, network, addr string, timeout time.Duration, usePublicDNS bool) (net.Conn, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	if !usePublicDNS {
		return dialer.DialContext(ctx, network, addr)
	}
	resolver := newUpdateResolver()
	ips, err := resolver.LookupHost(ctx, host)
	if err != nil {
		ips, err = net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return nil, err
		}
	}
	sortUpdateIPsByPreference(ips)
	var lastErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, fmt.Errorf("no addresses resolved for %s", host)
	}
	return nil, fmt.Errorf("failed to dial %s: %w", host, lastErr)
}

func (c *webSocketConn) ReadDataMessage() ([]byte, byte, error) {
	var message []byte
	var messageOpcode byte
	for {
		frame, err := c.readFrame()
		if err != nil {
			return nil, 0, err
		}
		switch frame.opcode {
		case wsOpcodePing:
			_ = c.writeFrame(wsOpcodePong, frame.payload, wssWriteTimeout)
			continue
		case wsOpcodePong:
			continue
		case wsOpcodeClose:
			code, reason := parseWebSocketClose(frame.payload)
			_ = c.writeFrame(wsOpcodeClose, frame.payload, wssWriteTimeout)
			return nil, 0, &wsCloseError{Code: code, Reason: reason}
		case wsOpcodeText, wsOpcodeBinary:
			if messageOpcode != 0 {
				return nil, 0, fmt.Errorf("WSS fragmented message overlapped")
			}
			messageOpcode = frame.opcode
			message = append(message, frame.payload...)
		case wsOpcodeContinuation:
			if messageOpcode == 0 {
				return nil, 0, fmt.Errorf("WSS continuation without message")
			}
			message = append(message, frame.payload...)
		default:
			return nil, 0, fmt.Errorf("WSS unsupported opcode=%d", frame.opcode)
		}
		if len(message) > webSocketMaxMessageSize {
			return nil, 0, fmt.Errorf("WSS message too large")
		}
		if frame.fin && (messageOpcode == wsOpcodeText || messageOpcode == wsOpcodeBinary) {
			return message, messageOpcode, nil
		}
	}
}

func (c *webSocketConn) WriteText(payload []byte, timeout time.Duration) error {
	return c.writeFrame(wsOpcodeText, payload, timeout)
}

func (c *webSocketConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *webSocketConn) Close() error {
	return c.conn.Close()
}

func (c *webSocketConn) readFrame() (wsFrame, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		return wsFrame{}, err
	}
	fin := header[0]&0x80 != 0
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return wsFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > webSocketMaxMessageSize {
		return wsFrame{}, fmt.Errorf("WSS frame too large")
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, maskKey[:]); err != nil {
			return wsFrame{}, err
		}
	}
	payload := make([]byte, int(length))
	if length > 0 {
		if _, err := io.ReadFull(c.reader, payload); err != nil {
			return wsFrame{}, err
		}
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return wsFrame{fin: fin, opcode: opcode, payload: payload}, nil
}

func (c *webSocketConn) writeFrame(opcode byte, payload []byte, timeout time.Duration) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if timeout > 0 {
		_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
		defer c.conn.SetWriteDeadline(time.Time{})
	}
	maskKey := [4]byte{}
	if _, err := rand.Read(maskKey[:]); err != nil {
		return err
	}
	payloadLen := len(payload)
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode)
	switch {
	case payloadLen < 126:
		header = append(header, 0x80|byte(payloadLen))
	case payloadLen <= 0xffff:
		header = append(header, 0x80|126, byte(payloadLen>>8), byte(payloadLen))
	default:
		header = append(header, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(payloadLen))
		header = append(header, ext[:]...)
	}
	header = append(header, maskKey[:]...)
	masked := make([]byte, payloadLen)
	for i := range payload {
		masked[i] = payload[i] ^ maskKey[i%4]
	}
	if err := writeAll(c.conn, header); err != nil {
		return err
	}
	return writeAll(c.conn, masked)
}

func writeAll(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

func newWebSocketKey() (string, error) {
	var key [16]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key[:]), nil
}

func expectedWebSocketAccept(key string) string {
	sum := sha1.Sum([]byte(key + webSocketGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerHasToken(headers http.Header, name, want string) bool {
	want = strings.ToLower(want)
	for _, value := range headers.Values(name) {
		for _, part := range strings.Split(value, ",") {
			if strings.ToLower(strings.TrimSpace(part)) == want {
				return true
			}
		}
	}
	return false
}

func parseWebSocketClose(payload []byte) (int, string) {
	if len(payload) < 2 {
		return 1005, ""
	}
	code := int(binary.BigEndian.Uint16(payload[:2]))
	return code, string(payload[2:])
}
