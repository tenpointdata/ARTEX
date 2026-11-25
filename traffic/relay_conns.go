package traffic

import (
	"net"
	"sync"
)

// relayConns is the loopback listener and connection bookkeeping a relay
// (ScopeRelay, CredentialRelay) shares: it serves each accepted connection on
// its own goroutine up to maxRelayConns at once, and close stops the listener
// and every connection still tracked.
type relayConns struct {
	ln    net.Listener
	once  sync.Once
	mu    sync.Mutex
	done  bool // close ran; no connection is tracked from then
	conns map[net.Conn]struct{}
	slots chan struct{} // one per client connection relayed at once (maxRelayConns)
}

// maxRelayConns caps the client connections one relay serves at once; a
// connection over it is closed unserved. Chromium keeps at most 256 sockets
// per profile (and 6 per proxy host per pool), so a browser never meets the
// cap; it bounds what a runaway or hostile client of the loopback port can
// hold open (two sockets and two goroutines a connection).
const maxRelayConns = 256

// listenRelay opens a relay's loopback listener on an ephemeral port.
func listenRelay() (*relayConns, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	return &relayConns{ln: ln, conns: map[net.Conn]struct{}{}, slots: make(chan struct{}, maxRelayConns)}, nil
}

// url is the credential-free proxy URL of the listener.
func (r *relayConns) url() string { return "http://" + r.ln.Addr().String() }

// serve accepts until the listener closes, handing each connection to handle.
func (r *relayConns) serve(handle func(down net.Conn)) {
	for {
		down, err := r.ln.Accept()
		if err != nil {
			return
		}
		select {
		case r.slots <- struct{}{}:
			go func() {
				defer func() { <-r.slots }()
				handle(down)
			}()
		default:
			_ = down.Close() // over maxRelayConns
		}
	}
}

// close stops the listener and closes the open connections.
func (r *relayConns) close() {
	r.once.Do(func() {
		_ = r.ln.Close()
		r.mu.Lock()
		r.done = true
		for c := range r.conns {
			_ = c.Close()
		}
		r.mu.Unlock()
	})
}

// track adds c to the open connections; false once the relay is closed.
func (r *relayConns) track(c net.Conn) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return false
	}
	r.conns[c] = struct{}{}
	return true
}

func (r *relayConns) untrack(c net.Conn) {
	r.mu.Lock()
	delete(r.conns, c)
	r.mu.Unlock()
}
