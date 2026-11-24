package traffic

import (
	"io"
	"net"
	"sync"
)

// Shared plumbing of the byte relays in this package (scope_relay.go,
// proxy_tls_front.go, tls_relay.go): a bounded set of open connections that
// Close can tear down, and a full-duplex splice.

// connSet tracks a relay's open connections and caps how many client
// connections it serves at once. The zero value is not usable; use newConnSet.
type connSet struct {
	mu    sync.Mutex
	done  bool // closeAll ran; no connection is tracked from then
	conns map[net.Conn]struct{}
	slots chan struct{}
}

func newConnSet(max int) *connSet {
	return &connSet{conns: map[net.Conn]struct{}{}, slots: make(chan struct{}, max)}
}

// acquire takes a client slot; false when the set is full.
func (s *connSet) acquire() bool {
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *connSet) release() { <-s.slots }

// track adds c to the open connections; false once the set is closed.
func (s *connSet) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return false
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *connSet) untrack(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
}

// closeAll closes every tracked connection and refuses new ones.
func (s *connSet) closeAll() {
	s.mu.Lock()
	s.done = true
	for c := range s.conns {
		_ = c.Close()
	}
	s.mu.Unlock()
}

// serveConns accepts on ln until it closes and runs handle for each client
// connection in its own goroutine; a connection over the set's cap is closed
// unserved. handle owns the connection.
func (s *connSet) serveConns(ln net.Listener, handle func(net.Conn)) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		if !s.acquire() {
			_ = c.Close()
			continue
		}
		go func() {
			defer s.release()
			handle(c)
		}()
	}
}

// splice tracks a and b and copies bytes both ways until both directions end,
// half-closing each side when its source ends. It returns at once, closing
// nothing, when the set is already closed; the caller closes a and b.
func (s *connSet) splice(a, b net.Conn) {
	if !s.track(a) {
		return
	}
	defer s.untrack(a)
	if !s.track(b) {
		return
	}
	defer s.untrack(b)
	done := make(chan struct{}, 2)
	pipe := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		}
		done <- struct{}{}
	}
	go pipe(a, b)
	go pipe(b, a)
	<-done
	<-done
}
