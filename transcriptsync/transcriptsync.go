// Package transcriptsync mirrors append-only JSONL session transcripts (one
// record per line, as written by norma's transcript.Store) into a shared
// Backend so a session written on one host can be resumed on another.
//
// The local file stays the write path (the agent SDK appends to it); the
// Backend is authoritative across hosts. A run takes a Lease on its session:
//
//   - Begin makes the caller the session's only writer (Backend.Acquire, a
//     fencing token), then reconciles the local file with the stored lines:
//     when the stored lines are a prefix of the local file the local file is
//     kept and its extra lines are pushed (this host was the last writer, or the
//     store is empty: a legacy or restored file is seeded); otherwise the local
//     file is replaced with the stored lines (another host wrote since).
//   - While the lease is open, complete lines appended to the local file are
//     pushed every Interval, and once more by Close.
//   - A later Begin for the same session (on any host) fences this lease out:
//     its pushes return ErrFenced and nothing it writes reaches the store.
//
// Loss on a crash is bounded by Interval: lines not yet pushed when a host dies
// are gone, and the resume starts from the last pushed line (the SDK already
// drops a trailing tool call without its result).
//
// The package knows nothing about tasks, intents or databases; a Backend
// adapts it to storage.
package transcriptsync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrFenced means another holder has acquired the session since this lease
// began; this lease's lines are no longer accepted.
var ErrFenced = errors.New("transcriptsync: session acquired by another holder")

// Backend is the shared, authoritative store of session lines.
type Backend interface {
	// Acquire makes holder the session's only writer (replacing any previous
	// holder) and returns the stored lines in order, without line terminators.
	Acquire(ctx context.Context, session, holder string) ([][]byte, error)
	// Append stores lines at positions from, from+1, ... when holder still holds
	// the session and exactly from lines are stored; ErrFenced otherwise. A
	// retry of an Append that committed although its caller saw an error must
	// succeed (the lines are already stored at from), not report ErrFenced.
	Append(ctx context.Context, session, holder string, from int, lines [][]byte) error
}

// DefaultInterval is the push period when Mirror.Interval is zero.
const DefaultInterval = 2 * time.Second

// pushTimeout bounds one push, which runs detached from the run's context so a
// pause or kill still flushes the last lines.
const pushTimeout = 30 * time.Second

// Mirror opens leases against one Backend.
type Mirror struct {
	Backend  Backend
	Interval time.Duration
	// OnError, when set, receives background push errors (ErrFenced included).
	OnError func(session string, err error)
}

// Lease is one writer's hold on a session. Close it when the run ends.
type Lease struct {
	m       *Mirror
	path    string
	session string
	holder  string

	mu     sync.Mutex
	offset int64 // bytes of the local file already pushed (complete lines only)
	pushed int   // lines already stored
	err    error // first push error; sticky once ErrFenced

	stop chan struct{}
	done chan struct{}
	once sync.Once
}

// Begin acquires session for this caller, reconciles the local file at path
// with the store and starts the background push. On error nothing was started
// and the local file may not reflect the store: the caller must not resume
// from it.
func (m *Mirror) Begin(ctx context.Context, path, session string) (*Lease, error) {
	if m == nil || m.Backend == nil {
		return nil, errors.New("transcriptsync: no backend")
	}
	l := &Lease{m: m, path: path, session: session, holder: newHolder(),
		stop: make(chan struct{}), done: make(chan struct{})}
	stored, err := m.Backend.Acquire(ctx, session, l.holder)
	if err != nil {
		return nil, fmt.Errorf("acquire transcript %q: %w", session, err)
	}
	local, complete, err := readLines(path, 0)
	if err != nil {
		return nil, err
	}
	if isPrefix(stored, local) {
		// Rewrite to the canonical form when the file carries a torn trailing
		// line (a crash mid-append) or empty lines, so offsets match lines.
		if complete != offsetOf(local, len(local)) || complete < fileSize(path) {
			if err := writeLines(path, local); err != nil {
				return nil, err
			}
		}
		l.pushed, l.offset = len(stored), offsetOf(local, len(stored))
	} else {
		if err := writeLines(path, stored); err != nil {
			return nil, err
		}
		l.pushed, l.offset = len(stored), offsetOf(stored, len(stored))
	}
	if err := l.push(ctx); err != nil {
		return nil, err
	}
	go l.loop()
	return l, nil
}

// Session returns the leased session id.
func (l *Lease) Session() string { return l.session }

// Close stops the background push and pushes the remaining complete lines. It
// returns ErrFenced when the lease was taken over, else the final push's error.
func (l *Lease) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() { close(l.stop) })
	<-l.done
	_ = l.push(context.Background())
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.err
}

// Push stores the complete lines appended since the last push.
func (l *Lease) Push(ctx context.Context) error { return l.push(ctx) }

func (l *Lease) loop() {
	defer close(l.done)
	every := l.m.Interval
	if every <= 0 {
		every = DefaultInterval
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-l.stop:
			return
		case <-t.C:
			if err := l.push(context.Background()); err != nil && l.m.OnError != nil {
				l.m.OnError(l.session, err)
			}
		}
	}
}

func (l *Lease) push(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if errors.Is(l.err, ErrFenced) {
		return l.err
	}
	lines, consumed, err := readLines(l.path, l.offset)
	if err == nil && len(lines) > 0 {
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pushTimeout)
		err = l.m.Backend.Append(pctx, l.session, l.holder, l.pushed, lines)
		cancel()
		if err == nil {
			l.pushed += len(lines)
			l.offset += consumed
		}
	}
	// ErrFenced is final; any other error stands only until a push succeeds.
	l.err = err
	return err
}

// Materialize writes the stored lines to path unless they are a prefix of the
// local file already (used to gather a session onto one host, for example
// before archiving). It reads without acquiring, so it never fences a writer.
func Materialize(path string, stored [][]byte) error {
	local, _, err := readLines(path, 0)
	if err != nil {
		return err
	}
	if len(stored) == 0 || isPrefix(stored, local) {
		return nil
	}
	return writeLines(path, stored)
}

// readLines returns the non-empty complete ('\n'-terminated) lines of the file
// from offset on, without terminators, and the number of bytes they span
// (empty lines included). A missing file is empty.
func readLines(path string, offset int64) ([][]byte, int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, 0, err
	}
	end := bytes.LastIndexByte(b, '\n')
	if end < 0 {
		return nil, 0, nil
	}
	var lines [][]byte
	for _, ln := range bytes.Split(b[:end], []byte{'\n'}) {
		if len(ln) > 0 {
			lines = append(lines, bytes.Clone(ln))
		}
	}
	return lines, int64(end + 1), nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

// offsetOf is the byte length of the first n lines in canonical form (each
// followed by '\n', no empty lines), as writeLines and the SDK write them.
func offsetOf(lines [][]byte, n int) int64 {
	var off int64
	for _, ln := range lines[:n] {
		off += int64(len(ln)) + 1
	}
	return off
}

func isPrefix(prefix, of [][]byte) bool {
	if len(prefix) > len(of) {
		return false
	}
	for i := range prefix {
		if !bytes.Equal(prefix[i], of[i]) {
			return false
		}
	}
	return true
}

// writeLines replaces path atomically with lines.
func writeLines(path string, lines [][]byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".transcriptsync-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	var buf bytes.Buffer
	for _, ln := range lines {
		buf.Write(ln)
		buf.WriteByte('\n')
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func newHolder() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
