package traffic

import (
	"io"
	"net"
	"sync"
)

// Shared plumbing of the byte relays in this package (ScopeRelay,
// CredentialRelay, TLSRelay, ProxyTLSFront): a listener served one goroutine
// per client up to a cap, the set of open connections that close tears down,
// and a full-duplex splice.

// connSet is a relay's listener and connection bookkeeping. It serves each
// accepted connection on its own goroutine, at most max at once (one over the
// cap is closed unserved), and close stops the listener and every connection
// still tracked. Use newConnSet or listenLoopback.
type connSet struct {
	ln    net.Listener
	once  sync.Once
	mu    sync.Mutex
	done  bool // close ran; no connection is tracked from then
	conns map[net.Conn]struct{}
	slots chan struct{} // one per client connection served at once
}

// maxRelayConns caps the client connections one loopback relay (ScopeRelay,
// CredentialRelay) serves at once. Chromium keeps at most 256 sockets per
// profile (and 6 per proxy host per pool), so a browser never meets the cap;
// it bounds what a runaway or hostile client of the loopback port can hold
// open (two sockets and two goroutines a connection).
const maxRelayConns = 256

func newConnSet(ln net.Listener, max int) *connSet {
	return &connSet{ln: ln, conns: map[net.Conn]struct{}{}, slots: make(chan struct{}, max)}
}

// listenLoopback opens a relay's listener on an ephemeral loopback port.
func listenLoopback(max int) (*connSet, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return newConnSet(ln, max), nil
}

// addr is the listener's host:port.
func (s *connSet) addr() string { return s.ln.Addr().String() }

// url is the credential-free plain-HTTP proxy URL of the listener.
func (s *connSet) url() string { return "http://" + s.addr() }

// serve accepts until the listener closes and runs handle for each client
// connection in its own goroutine. handle owns the connection.
func (s *connSet) serve(handle func(net.Conn)) {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		select {
		case s.slots <- struct{}{}:
			go func() {
				defer func() { <-s.slots }()
				handle(c)
			}()
		default:
			_ = c.Close() // over the cap
		}
	}
}

// close stops the listener, closes every tracked connection and refuses new
// ones. Idempotent.
func (s *connSet) close() {
	s.once.Do(func() {
		_ = s.ln.Close()
		s.mu.Lock()
		s.done = true
		for c := range s.conns {
			_ = c.Close()
		}
		s.mu.Unlock()
	})
}

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

// trackPair tracks a and b and returns the function that untracks them; ok is
// false (and nothing stays tracked) once the set is closed.
func (s *connSet) trackPair(a, b net.Conn) (untrack func(), ok bool) {
	if !s.track(a) {
		return nil, false
	}
	if !s.track(b) {
		s.untrack(a)
		return nil, false
	}
	return func() { s.untrack(a); s.untrack(b) }, true
}

// splice tracks a and b and copies bytes both ways until both directions end
// (pipeBoth). It returns at once, closing nothing, when the set is already
// closed; the caller closes a and b.
func (s *connSet) splice(a, b net.Conn) {
	untrack, ok := s.trackPair(a, b)
	if !ok {
		return
	}
	defer untrack()
	pipeBoth(a, a, b, b)
}

// pipeBoth copies fromA to b and fromB to a until both directions end. fromA
// and fromB are a's and b's read sides (a buffered reader that already holds
// bytes read from the connection, or the connection itself). When a source
// ends its destination is half-closed, or closed if it cannot half-close, so
// the peer sees the end and the other direction can finish.
func pipeBoth(a net.Conn, fromA io.Reader, b net.Conn, fromB io.Reader) {
	done := make(chan struct{}, 2)
	pipe := func(dst net.Conn, src io.Reader) {
		_, _ = io.Copy(dst, src)
		if cw, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = cw.CloseWrite()
		} else {
			_ = dst.Close()
		}
		done <- struct{}{}
	}
	go pipe(b, fromA)
	go pipe(a, fromB)
	<-done
	<-done
}
