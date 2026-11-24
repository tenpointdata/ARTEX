import type * as React from "react";

// Renders a translated string, turning <b>…</b> spans into bold elements, so a sentence with emphasis stays one key.
export function rich(s: string): React.ReactNode[] {
  return s.split(/<b>(.*?)<\/b>/).map((part, i) => (i % 2 === 1 ? <b key={i}>{part}</b> : part));
}
