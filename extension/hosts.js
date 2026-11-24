// ARTEX Traffic Capture — hosts that are never captured.
//
// Operator-driven browsing often bounces through a single-sign-on provider. Those
// exchanges carry the operator's IdP session cookies and bearer/SAML/OIDC tokens,
// which are credentials for the operator's whole organization, not evidence about
// the target. We therefore skip such exchanges entirely (no request, no response,
// no headers): redacting only the Cookie / Set-Cookie / Authorization values
// would still leak tokens in URLs, form posts and response bodies.
//
// Loaded as a classic script by both the service worker (importScripts) and the
// popup (<script>); it publishes one global so neither needs a module loader.

(function (root) {
  // Pattern syntax: "host.example" matches that exact host; "*.example.com"
  // matches example.com and every subdomain of it.
  const DEFAULT_SKIP_HOSTS = [
    "accounts.google.com",
    "login.microsoftonline.com",
    "login.microsoft.com",
    "login.windows.net",
    "login.live.com",
    "*.okta.com",
    "*.oktapreview.com",
    "*.auth0.com",
    "*.onelogin.com",
    "*.duosecurity.com",
    "appleid.apple.com",
  ];

  // Parse the operator's editable list: one pattern per line (commas also work),
  // blank lines and "#" comments ignored, lowercased, trailing dots removed.
  function parseHostList(text) {
    return String(text || "")
      .split(/[\n,]/)
      .map((l) => l.replace(/#.*$/, "").trim().toLowerCase().replace(/\.$/, ""))
      .filter(Boolean);
  }

  function hostMatches(hostname, pattern) {
    if (pattern.startsWith("*.")) {
      const base = pattern.slice(2);
      return hostname === base || hostname.endsWith("." + base);
    }
    return hostname === pattern;
  }

  // True when the URL's host is on the built-in or the operator's skip list.
  // An unparseable URL is skipped: if we cannot tell where it goes, we do not keep it.
  function isSkippedUrl(url, extraPatterns) {
    let hostname;
    try {
      hostname = new URL(url).hostname.toLowerCase().replace(/\.$/, "");
    } catch (_) {
      return true;
    }
    return DEFAULT_SKIP_HOSTS.concat(extraPatterns || []).some((p) => hostMatches(hostname, p));
  }

  // The Chrome match pattern for a page's host, used for the optional host
  // permission. Match patterns ignore ports; only http(s) pages can be captured.
  function originPattern(url) {
    try {
      const u = new URL(url);
      if (u.protocol !== "http:" && u.protocol !== "https:") return null;
      return u.protocol + "//" + u.hostname + "/*";
    } catch (_) {
      return null;
    }
  }

  root.ArtexHosts = { DEFAULT_SKIP_HOSTS, parseHostList, hostMatches, isSkippedUrl, originPattern };
})(globalThis);
