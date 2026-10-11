package pilot_test

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

// This proxy forwards real PostgreSQL protocol messages. It drops the COMMIT
// reply only AFTER the backend reports the receipt transaction committed.
// Authentication bytes and SQL are never logged or retained as artifacts.
type commitProxy struct {
	listener    net.Listener
	target      string
	armed       atomic.Bool
	dropped     chan struct{}
	connections sync.Map
}

func newCommitProxy(t *testing.T, target string) *commitProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("create commit fault proxy")
	}
	p := &commitProxy{listener: listener, target: target, dropped: make(chan struct{})}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			p.connections.Store(conn, true)
			go p.forward(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		p.connections.Range(func(k, v any) bool { _ = k.(net.Conn).Close(); return true })
	})
	return p
}
func packet(conn net.Conn) ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[1:])
	if n < 4 || n > 16<<20 {
		return nil, io.ErrUnexpectedEOF
	}
	data := make([]byte, int(n)+1)
	copy(data, header)
	_, err := io.ReadFull(conn, data[5:])
	return data, err
}
func (p *commitProxy) forward(client net.Conn) {
	defer p.connections.Delete(client)
	defer client.Close()
	backend, err := net.Dial("tcp", p.target)
	if err != nil {
		return
	}
	defer backend.Close()
	var receipt atomic.Bool
	// The fixture DSN has sslmode=disable; its first packet is StartupMessage.
	length := make([]byte, 4)
	if _, err = io.ReadFull(client, length); err != nil {
		return
	}
	n := binary.BigEndian.Uint32(length)
	if n < 8 || n > 1<<20 {
		return
	}
	startup := make([]byte, n)
	copy(startup, length)
	if _, err = io.ReadFull(client, startup[4:]); err != nil {
		return
	}
	if _, err = backend.Write(startup); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer backend.Close()
		for {
			message, err := packet(client)
			if err != nil {
				return
			}
			if (message[0] == 'P' || message[0] == 'Q') && bytes.Contains(message, []byte("INSERT INTO ingestion_receipt(")) {
				receipt.Store(true)
			}
			if _, err = backend.Write(message); err != nil {
				return
			}
		}
	}()
	for {
		message, err := packet(backend)
		if err != nil {
			return
		}
		if message[0] == 'C' && string(message[5:]) == "COMMIT\x00" && receipt.Load() && p.armed.CompareAndSwap(true, false) {
			close(p.dropped)
			return
		}
		if _, err = client.Write(message); err != nil {
			return
		}
		if message[0] == 'Z' && message[5] == 'I' {
			receipt.Store(false)
		}
	}
}
