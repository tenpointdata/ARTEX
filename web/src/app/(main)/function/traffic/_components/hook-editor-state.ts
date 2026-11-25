import { intlLocale } from "@/i18n";
import type { buildMatchers } from "@/lib/tamper-hooks";
import {
  DEFAULT_REQUEST_HOOK_SOURCE,
  DEFAULT_RESPONSE_HOOK_SOURCE,
  formatHeaderMatcherLines,
} from "@/lib/tamper-hooks";
import type { TamperHook, TamperHookPhase } from "@/lib/types";

export function fmtDateTime(ts: string) {
  if (!ts) return "";
  return new Date(ts).toLocaleString(intlLocale, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export interface EditorState {
  mode: "create" | "edit";
  id?: number;
  enabled: boolean;
  name: string;
  phase: TamperHookPhase;
  priority: string;
  note: string;
  methods: string;
  host: string;
  urlRegex: string;
  headers: string;
  bodyRegex: string;
  status: string;
  respHeaders: string;
  respBodyRegex: string;
  source: string;
}

export function emptyEditor(phase: TamperHookPhase = "request"): EditorState {
  return {
    mode: "create",
    enabled: false,
    name: "",
    phase,
    priority: "100",
    note: "",
    methods: "",
    host: "",
    urlRegex: "",
    headers: "",
    bodyRegex: "",
    status: "",
    respHeaders: "",
    respBodyRegex: "",
    source: phase === "response" ? DEFAULT_RESPONSE_HOOK_SOURCE : DEFAULT_REQUEST_HOOK_SOURCE,
  };
}

export function editorFromHook(h: TamperHook): EditorState {
  return {
    mode: "edit",
    id: h.id,
    enabled: h.enabled,
    name: h.name,
    phase: h.phase,
    priority: String(h.priority),
    note: h.note ?? "",
    methods: (h.matchers?.methods ?? []).join(", "),
    host: h.matchers?.host ?? "",
    urlRegex: h.matchers?.url_regex ?? "",
    headers: formatHeaderMatcherLines(h.matchers?.headers),
    bodyRegex: h.matchers?.body_regex ?? "",
    status: (h.matchers?.status ?? []).join(", "),
    respHeaders: formatHeaderMatcherLines(h.matchers?.resp_headers),
    respBodyRegex: h.matchers?.resp_body_regex ?? "",
    source: h.source,
  };
}

export interface DryRunTarget {
  label: string;
  hookId?: number;
  inline?: {
    name: string;
    phase: TamperHookPhase;
    matchers: ReturnType<typeof buildMatchers>;
    source: string;
  };
}
