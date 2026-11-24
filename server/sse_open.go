package server

import (
	"fmt"
	"net/http"
)

// openSSE writes the headers of a Server-Sent Events response and a comment
// line, and flushes them. The comment gives the response a first body byte:
// a proxy that waits for one before passing the headers on (the Next.js dev
// rewrite does) would otherwise hold the stream closed, and the browser's
// EventSource would not open, until the first event or keepalive ping.
// EventSource ignores comment lines.
func openSSE(w http.ResponseWriter, f http.Flusher) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering
	fmt.Fprint(w, ": open\n\n")
	f.Flush()
}
