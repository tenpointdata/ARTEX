/**
 * How a Markdown image `src` is rendered. Finding Markdown is influenced by
 * target content, so a remote `![](https://evil/x.png)` must never auto-load:
 * the fetch would tell that server the operator opened the finding (and from
 * where). Only images served by this origin, `data:image/*` and `blob:` load
 * inline; anything else becomes a link the operator may open on purpose.
 */
export type MarkdownImage =
  | { kind: "inline"; src: string }
  | { kind: "remote"; href: string; host: string }
  | { kind: "blocked" };

export function classifyMarkdownImage(src: string | undefined | null, baseOrigin: string): MarkdownImage {
  const raw = (src ?? "").trim();
  if (!raw) {
    return { kind: "blocked" };
  }
  let url: URL;
  try {
    // Resolving against the console origin makes `//host/x` (protocol-relative)
    // and similar tricks come out as the remote URLs they are.
    url = new URL(raw, baseOrigin);
  } catch {
    return { kind: "blocked" };
  }
  const base = new URL(baseOrigin).origin;
  if (url.protocol === "data:") {
    return /^data:image\//i.test(raw) ? { kind: "inline", src: raw } : { kind: "blocked" };
  }
  if (url.protocol === "blob:") {
    return url.origin === base ? { kind: "inline", src: raw } : { kind: "blocked" };
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    return { kind: "blocked" };
  }
  if (url.origin === base) {
    return { kind: "inline", src: url.href };
  }
  return { kind: "remote", href: url.href, host: url.host };
}
