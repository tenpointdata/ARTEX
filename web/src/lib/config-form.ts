import type { ConfigValues } from "./types";

// Field tables for adapter configs with server-masked credentials.
//
// Shared by notification channels and ticket trackers: both backends mask the
// keys an adapter names in secret_keys ("__masked__…tail") and treat a masked
// value sent back as "keep the stored value" (notify.PrepareConfigUpdateKeys).
// This module is data and conversion only, with no UI or i18n imports, so it
// is unit-testable under plain node.

export type FieldKind = "text" | "password" | "number" | "select" | "textarea" | "switch" | "kv" | "list";

export interface FieldDef {
  key: string;
  label: string;
  kind: FieldKind;
  placeholder?: string;
  help?: string;
  options?: { value: string; label: string }[];
}

export const MASKED_PREFIX = "__masked__";

export function isMasked(v: unknown): v is string {
  return typeof v === "string" && v.startsWith(MASKED_PREFIX);
}

// maskedTail is the recognizable end of a masked credential ("__masked__:…abc" → "abc").
export function maskedTail(v: string): string {
  return v.split("…")[1] ?? "";
}

// parseKV parses one KEY=VALUE per line.
export function parseKV(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const ln = line.trim();
    if (!ln) continue;
    const i = ln.indexOf("=");
    if (i > 0) out[ln.slice(0, i).trim()] = ln.slice(i + 1).trim();
  }
  return out;
}

// parseList splits a comma/whitespace-separated list; an array passes through.
export function parseList(raw: unknown): string[] {
  const parts = Array.isArray(raw) ? raw.map(String) : String(raw ?? "").split(/[\s,]+/);
  return parts.map((s) => s.trim()).filter(Boolean);
}

// buildConfig turns form values into the config to submit.
//
// One rule, two kinds of values:
//   - masked values are sent back verbatim → the backend keeps the stored value;
//   - everything else is the user's input, where an empty string clears the field.
// Credential fields get no "skip when empty" treatment, because then no action
// in the UI could clear a mis-set key. Masked values never appear in inputs
// (see ConfigField), so text in a box always means the user typed it.
export function buildConfig(defs: FieldDef[], values: ConfigValues): ConfigValues {
  const out: ConfigValues = {};
  for (const d of defs) {
    const raw = values[d.key];
    if (d.kind === "switch") {
      out[d.key] = raw === true;
    } else if (isMasked(raw)) {
      out[d.key] = raw;
    } else if (d.kind === "number") {
      const n = Number(raw);
      out[d.key] = Number.isFinite(n) && n > 0 ? n : 0;
    } else if (d.kind === "kv") {
      out[d.key] = typeof raw === "object" && raw !== null && !Array.isArray(raw) ? raw : parseKV(String(raw ?? ""));
    } else if (d.kind === "list") {
      out[d.key] = parseList(raw);
    } else {
      out[d.key] = String(raw ?? "").trim();
    }
  }
  return out;
}
