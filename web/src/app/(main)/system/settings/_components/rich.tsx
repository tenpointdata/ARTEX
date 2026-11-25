import type * as React from "react";

// Renders a translated string, turning <b>…</b> and <code>…</code> spans into elements, so a sentence with emphasis stays one key.
export function rich(s: string): React.ReactNode[] {
  return s.split(/(<b>.*?<\/b>|<code>.*?<\/code>)/).map((part, i) => {
    const b = /^<b>(.*)<\/b>$/.exec(part);
    if (b) return <b key={i}>{b[1]}</b>;
    const c = /^<code>(.*)<\/code>$/.exec(part);
    if (c) return <code key={i}>{c[1]}</code>;
    return part;
  });
}
