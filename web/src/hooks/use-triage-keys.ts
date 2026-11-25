"use client";

import * as React from "react";

import { isTypingTarget, type TriageNav, type TriageVerdict, triageCommandForKey } from "@/lib/triage";

// useTriageKeys binds a queue's triage keys while `active`: its verdict keys and j/k/Esc. Keys are
// ignored while typing, with a modifier held, or while a dialog, menu or listbox is open over the list.
export function useTriageKeys<S extends string>(
  active: boolean,
  verdicts: readonly TriageVerdict<S>[],
  onVerdict: (status: S) => void,
  onNav: (nav: TriageNav) => void,
) {
  const handlers = React.useRef({ verdicts, onVerdict, onNav });
  handlers.current = { verdicts, onVerdict, onNav };
  React.useEffect(() => {
    if (!active) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return;
      if (event.target instanceof HTMLElement && isTypingTarget(event.target)) return;
      if (document.querySelector('[role="dialog"], [role="alertdialog"], [role="listbox"], [role="menu"]')) return;
      const command = triageCommandForKey(event.key, handlers.current.verdicts);
      if (!command) return;
      event.preventDefault();
      if (command.kind === "verdict") handlers.current.onVerdict(command.status);
      else handlers.current.onNav(command.nav);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [active]);
}
