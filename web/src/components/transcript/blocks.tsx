"use client";

import * as React from "react";

import {
  ChevronDown,
  ChevronRight,
  CrosshairIcon,
  Flag,
  MessageSquare,
  PaperclipIcon,
  Terminal,
  UserIcon,
  Wrench,
} from "lucide-react";

import { Markdown } from "@/components/markdown";
import { t } from "@/i18n";
import { reportsBashExit127 } from "@/lib/bash-exit-status";
import type { Activity } from "@/lib/types";

import { ActivityTime, chip, type Group, kindLabel, toolInputText, useInView } from "./helpers";

export function ToolBlock({
  group,
  getDetail,
  showWorker,
  focused = false,
}: {
  group: Extract<Group, { type: "tool" }>;
  getDetail: (seq: number) => Promise<string>;
  showWorker?: boolean;
  focused?: boolean;
}) {
  const [open, setOpen] = React.useState(focused);
  const targetRef = React.useRef<HTMLElement>(null);
  const [detail, setDetail] = React.useState<{ text: string; bashExit127: boolean } | null>(null);
  // what we last loaded, keyed by the underlying step seqs. When the tool result
  // arrives after we expanded mid-run (command only), this key changes and the
  // effect below re-fetches — so the output shows up instead of being cached out.
  const loadedKey = React.useRef<string | null>(null);
  const { use, result } = group;
  const rawToolName = use?.tool || result?.tool || "";
  const toolName = rawToolName || "Tool";
  const ToolIcon = toolName === "Bash" ? Terminal : Wrench;
  const running = !result;
  const ok = !!result && !result.is_error;
  const statusTone = running
    ? "text-muted-foreground"
    : ok
      ? "text-emerald-600 dark:text-emerald-400"
      : "text-red-600 dark:text-red-400";
  const rawCmd =
    use && use.summary.startsWith(toolName) ? use.summary.slice(toolName.length).trimStart() : (use?.summary ?? "");
  const cmd = toolInputText(toolName, rawCmd);
  // status only — the full result lives behind the expand ([Output]), not previewed inline
  const statusText = running ? t("Running…") : ok ? "✓" : t("✕ Failed");

  // key over the seqs we'd load; changes when the result (or command) arrives.
  const detailKey = `${use?.seq ?? ""}:${result?.seq ?? ""}`;
  React.useEffect(() => {
    if (!open || loadedKey.current === detailKey) return;
    let live = true;
    const segs: { label: string; seq: number; command: boolean }[] = [];
    if (use) segs.push({ label: t("Command"), seq: use.seq, command: true });
    if (result) segs.push({ label: t("Output") + (result.is_error ? " ✕" : " ✓"), seq: result.seq, command: false });
    void Promise.all(
      segs.map((x) =>
        getDetail(x.seq)
          .then((d) => d || t("(empty)"))
          .catch(() => t("(failed to load)")),
      ),
    ).then((parts) => {
      if (!live) return;
      const resultText = result ? parts[parts.length - 1] : undefined;
      setDetail({
        text: segs
          .map((x, i) => `[${x.label}]\n${x.command ? toolInputText(toolName, parts[i]) : parts[i]}`)
          .join("\n\n"),
        bashExit127:
          toolName === "Bash" &&
          result?.is_error === true &&
          resultText !== undefined &&
          reportsBashExit127(resultText),
      });
      loadedKey.current = detailKey;
    });
    return () => {
      live = false;
    };
  }, [open, detailKey, use, result, getDetail, toolName]);

  const scrolledRef = React.useRef(false);
  React.useEffect(() => {
    scrolledRef.current = false;
    if (focused) setOpen(true);
  }, [focused]);
  React.useEffect(() => {
    if (!focused || !open || detail === null || scrolledRef.current) return;
    const el = targetRef.current;
    const viewport = el?.closest('[data-slot="scroll-area-viewport"]');
    if (!el || !viewport) return;
    scrolledRef.current = true;
    const center = () => {
      const rect = el.getBoundingClientRect();
      const bounds = viewport.getBoundingClientRect();
      viewport.scrollTop += rect.top + rect.height / 2 - bounds.top - bounds.height / 2;
    };
    // User bubbles load their full text lazily. Allow their initial layout to
    // settle, but stop anchoring immediately if the user interacts or after 2s.
    const observer = new ResizeObserver(center);
    observer.observe(el.parentElement ?? el);
    observer.observe(viewport);
    const stop = () => observer.disconnect();
    viewport.addEventListener("wheel", stop, { passive: true, once: true });
    viewport.addEventListener("touchstart", stop, { passive: true, once: true });
    viewport.addEventListener("pointerdown", stop, { once: true });
    const frame = requestAnimationFrame(center);
    const timer = setTimeout(stop, 2000);
    return () => {
      cancelAnimationFrame(frame);
      clearTimeout(timer);
      stop();
      viewport.removeEventListener("wheel", stop);
      viewport.removeEventListener("touchstart", stop);
      viewport.removeEventListener("pointerdown", stop);
    };
  }, [focused, open, detail]);

  function toggle() {
    setOpen((o) => !o);
  }

  return (
    <section
      ref={targetRef}
      aria-label={focused ? t("Focused tool call #{seq}", { seq: use?.seq ?? "" }) : undefined}
      className={focused ? "rounded-lg border-2 border-primary bg-primary/5 p-3 text-xs" : "text-xs"}
    >
      <button type="button" onClick={toggle} className="flex w-full items-start gap-2 py-1 text-left hover:bg-muted/40">
        <span className="mt-0.5 text-muted-foreground">
          {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        </span>
        <ToolIcon className={"mt-0.5 size-3.5 shrink-0 " + (running ? "text-sky-600 dark:text-sky-400" : statusTone)} />
        {showWorker && <span className={chip(group.worker)}>{group.worker}</span>}
        <span className="shrink-0 font-medium text-sky-600 dark:text-sky-400">{rawToolName || t("Tool")}</span>
        {cmd && <span className="min-w-0 flex-1 truncate font-mono text-muted-foreground">{cmd}</span>}
        <span className={"ml-auto shrink-0 font-medium " + statusTone}>{statusText}</span>
      </button>
      {open && detail?.bashExit127 && (
        <p className="ml-7 mb-1 text-muted-foreground" role="status">
          {t("Bash reported exit code 127. Check whether the command or its interpreter is available in this runtime.")}
        </p>
      )}
      {open && (
        <pre className="ml-7 mb-1 max-h-72 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/50 p-2 font-mono text-[11px] leading-relaxed">
          {detail?.text ?? t("Loading…")}
        </pre>
      )}
    </section>
  );
}

// MessageBlock renders a coalesced agent message (merged streaming text/thinking/
// result fragments into one block) so the conversation reads as messages, not rows.
export function MessageBlock({
  group,
  getDetail,
  showWorker,
}: {
  group: Extract<Group, { type: "msg" }>;
  getDetail: (seq: number) => Promise<string>;
  showWorker?: boolean;
}) {
  const [open, setOpen] = React.useState(false);
  const [detail, setDetail] = React.useState<string | null>(null);
  // like ToolBlock: keyed by the group's step seqs so streamed steps arriving
  // after an early expand re-fetch instead of being cached out.
  const loadedKey = React.useRef<string | null>(null);
  const speak = group.steps.filter((s) => s.kind !== "thinking");
  const hasThinking = group.steps.some((s) => s.kind === "thinking");
  const isError = group.steps.some((s) => s.is_error);
  const body = (speak.length ? speak : group.steps).map((s) => s.summary).join("  ") || "…";
  const Icon = isError ? Flag : MessageSquare;
  const tone = isError ? "text-red-600 dark:text-red-400" : "text-foreground";

  const detailKey = group.steps.map((s) => s.seq).join(",");
  React.useEffect(() => {
    if (!open || loadedKey.current === detailKey) return;
    let live = true;
    void Promise.all(
      group.steps.map((s) =>
        getDetail(s.seq)
          .then((d) => d || s.summary)
          .catch(() => s.summary),
      ),
    ).then((parts) => {
      if (!live) return;
      setDetail(group.steps.map((s, i) => `[${kindLabel(s.kind)}]\n${parts[i]}`).join("\n\n"));
      loadedKey.current = detailKey;
    });
    return () => {
      live = false;
    };
  }, [open, detailKey, group.steps, getDetail]);

  function toggle() {
    setOpen((o) => !o);
  }

  return (
    <div className="text-xs">
      <button type="button" onClick={toggle} className="flex w-full items-start gap-2 py-1 text-left hover:bg-muted/40">
        <span className="mt-0.5 text-muted-foreground">
          {open ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        </span>
        <Icon className={"mt-0.5 size-3.5 shrink-0 " + tone} />
        {showWorker && <span className={chip(group.worker)}>{group.worker}</span>}
        <span className={"min-w-0 flex-1 truncate " + tone}>
          {body}
          {hasThinking && <span className="ml-1 text-[10px] text-muted-foreground">· {t("includes thinking")}</span>}
        </span>
      </button>
      {open && (
        <pre className="ml-7 mb-1 max-h-72 overflow-auto whitespace-pre-wrap break-all rounded bg-muted/50 p-2 font-mono text-[11px] leading-relaxed">
          {detail ?? t("Loading…")}
        </pre>
      )}
    </div>
  );
}

// UserRow renders a right-aligned chat bubble: either a human turn (the message you
// sent the main agent) or, in a worker session, the LLM-generated intent that leads
// it (intent=true) — same bubble, but a target icon instead of the human avatar.
// summary is a truncated first line, so the full message is pulled from the detail
// and shown in full (bubble is whitespace-pre-wrap, so long/multi-line text wraps).
// fmtBytes renders a human file size for attachment chips.
export function fmtBytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

export type MsgAttachment = { name: string; path: string; size: number };

// parseUserBody splits a user turn's body into text + attachments. The backend stores
// Detail as JSON {text, attachments} when files were uploaded, else plain text — so we
// parse defensively and fall back to treating the whole body as text. An ask_operator
// request that names hosts outside the task goal also carries target_derived_hosts (F12).
export function parseUserBody(body: string): {
  text: string;
  attachments: MsgAttachment[];
  targetDerivedHosts?: string[];
} {
  if (body.startsWith("{")) {
    try {
      const p = JSON.parse(body);
      if (p && Array.isArray(p.attachments)) {
        const hosts = Array.isArray(p.target_derived_hosts) ? (p.target_derived_hosts as string[]) : undefined;
        return {
          text: typeof p.text === "string" ? p.text : "",
          attachments: p.attachments,
          targetDerivedHosts: hosts,
        };
      }
    } catch {
      /* plain text that merely starts with "{" */
    }
  }
  return { text: body, attachments: [] };
}

export function UserRow({
  step,
  intent,
  getDetail,
}: {
  step: Activity;
  intent?: boolean;
  getDetail: (seq: number) => Promise<string>;
}) {
  const Icon = intent ? CrosshairIcon : UserIcon;
  const [ref, inView] = useInView();
  // Optimistic echoes carry their detail inline; persisted rows lazy-load it on scroll.
  const inline = step.detail && step.detail.length > 0 ? step.detail : null;
  const [full, setFull] = React.useState<string | null>(inline);
  React.useEffect(() => {
    if (inline) return; // already have the body (optimistic echo)
    if (!inView) return; // fetch the full message only when the bubble nears view
    let live = true;
    getDetail(step.seq)
      .then((d) => {
        if (live) setFull(d || step.summary);
      })
      .catch(() => {
        if (live) setFull(step.summary);
      });
    return () => {
      live = false;
    };
  }, [inView, step.seq, getDetail, step.summary, inline]);
  const { text, attachments, targetDerivedHosts } = parseUserBody(full ?? step.summary);
  return (
    <div ref={ref} className="mt-3 mb-2 flex min-w-0 justify-end gap-2">
      <div className="flex min-w-0 max-w-[85%] flex-col items-end gap-1.5">
        {targetDerivedHosts && targetDerivedHosts.length > 0 && (
          <span
            className="rounded-md border border-amber-500/50 bg-amber-500/10 px-2 py-0.5 text-amber-700 text-xs [overflow-wrap:anywhere] dark:text-amber-400"
            title={t("This request names hosts that are not in the task goal; they may come from target content.")}
          >
            {t("Contains target-derived text: {hosts}", { hosts: targetDerivedHosts.join(", ") })}
          </span>
        )}
        {attachments.length > 0 && (
          <div className="flex min-w-0 max-w-full flex-wrap justify-end gap-1.5">
            {attachments.map((a) => (
              <div
                key={a.path}
                className="flex min-w-0 max-w-full items-center gap-1.5 rounded-md border bg-card px-2 py-1 text-xs shadow-sm"
                title={a.path}
              >
                <PaperclipIcon className="size-3 shrink-0 text-primary" />
                <span className="max-w-[180px] truncate font-medium">{a.name}</span>
                <span className="shrink-0 text-muted-foreground">{fmtBytes(a.size)}</span>
              </div>
            ))}
          </div>
        )}
        {text && (
          <div className="min-w-0 max-w-full whitespace-pre-wrap rounded-lg rounded-tr-sm bg-primary px-3 py-1.5 text-sm text-primary-foreground [overflow-wrap:anywhere]">
            {text}
          </div>
        )}
        <ActivityTime ts={step.ts} />
      </div>
      <div className="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/10">
        <Icon className="size-3.5 text-primary" />
      </div>
    </div>
  );
}

// AnswerBlock renders the agent's FINAL answer (kind="result") in full — never
// collapsed. The summary is a truncated first line, so the full text is pulled
// from the detail and shown inline.
export function AnswerBlock({ step, getDetail }: { step: Activity; getDetail: (seq: number) => Promise<string> }) {
  const [ref, inView] = useInView();
  const [full, setFull] = React.useState<string | null>(null);
  React.useEffect(() => {
    if (!inView) return; // fetch the full answer only when it nears view
    let live = true;
    getDetail(step.seq)
      .then((d) => {
        if (live) setFull(d || step.summary);
      })
      .catch(() => {
        if (live) setFull(step.summary);
      });
    return () => {
      live = false;
    };
  }, [inView, step.seq, getDetail, step.summary]);
  return (
    <div ref={ref} className="mb-2 mt-1 flex min-w-0 flex-col gap-1">
      <div
        className={
          "min-w-0 flex-1 break-words rounded-lg bg-muted px-3 py-2 " +
          (step.is_error ? "text-sm text-red-600 dark:text-red-400" : "")
        }
      >
        {step.is_error ? (
          <span className="whitespace-pre-wrap">{full ?? step.summary}</span>
        ) : (
          <Markdown text={full ?? step.summary} />
        )}
      </div>
      <ActivityTime ts={step.ts} />
    </div>
  );
}

// ExecView renders an agent execution replay (planner / worker / main agent) in
// the compact, grouped, expand-to-detail format — thinking, tool calls/results,
// and (for the main agent) the human turns. Worker lane chips show only when the
// view actually mixes agents.
