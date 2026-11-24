// sseUrl returns the URL an EventSource opens for one of the console's streams.
//
// It is always same-origin. The session is an HttpOnly cookie and scripts hold no
// token, so the only credential an EventSource can present is that cookie, and the
// browser sends it only to the page's own origin (an EventSource without
// withCredentials sends no cookie cross-origin, and the backend's CORS answers "*",
// which a credentialed request may not use). A stream URL pointing anywhere else
// is refused with 401 on every connect.
//
// - Embedded UI / static export: the Go backend serves the page and the streams on
//   one origin; behind a reverse proxy the page and /api share the proxy's origin.
// - next dev: /api/* goes through the Next.js rewrite to the backend, which forwards
//   the cookie. next.config.mjs turns compression off in that mode, because gzip
//   would buffer the stream and no frame would reach the browser.
export function sseUrl(path: string): string {
  if (!path.startsWith("/api/")) {
    throw new Error(`sseUrl: stream path must be a same-origin /api/ path, got ${path}`);
  }
  return path;
}
