import type * as React from "react";

// rich renders a translated string, turning <b>…</b> and <code>…</code> spans into
// elements, so a sentence with emphasis stays one i18n key.
export function rich(s: string): React.ReactNode[] {
  let offset = 0;
  return s.split(/(<b>.*?<\/b>|<code>.*?<\/code>)/).map((part) => {
    // The character offset of each part within the source string is a stable, unique key.
    const key = offset;
    offset += part.length;
    const b = /^<b>(.*)<\/b>$/.exec(part);
    if (b) return <b key={key}>{b[1]}</b>;
    const c = /^<code>(.*)<\/code>$/.exec(part);
    if (c) return <code key={key}>{c[1]}</code>;
    return part;
  });
}
