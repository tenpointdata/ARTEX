// Traffic tools are host tools gated by the global traffic capture switch: bindable, but
// only usable when capture is on. Keep in sync with traffic.SeedToolMetas.
export const TRAFFIC_TOOL_KEYS = new Set(["traffic_search", "traffic_get"]);

// Minimal typed view of the JSON-schema fragments the editor touches.
const SCALAR_TYPES = ["string", "integer", "number", "boolean"];

type JsonRecord = Record<string, unknown>;
export type JsonSchema = JsonRecord;

function isRecord(v: unknown): v is JsonRecord {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function asRecord(v: unknown): JsonRecord {
  return isRecord(v) ? v : {};
}

function asStringList(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

// paramRows flattens schema.properties into an ordered row list for editing.
// name/type/required are read-only (welded to the Go handler); description/default
// are editable. Non-scalar params (array/object) can't carry an editable default.
// parentKey is set for rows that live inside an array param's items.properties.
export type ParamRow = {
  name: string;
  type: string;
  required: boolean;
  description: string;
  defaultStr: string; // "" = no default declared
  scalar: boolean;
  parentKey?: string; // set when this is a sub-field of an array param's items
};

export function toRows(schema: JsonSchema): ParamRow[] {
  const props = asRecord(schema?.properties);
  const required = new Set<string>(asStringList(schema?.required));
  const rows: ParamRow[] = [];
  for (const [name, raw] of Object.entries(props)) {
    const p = asRecord(raw);
    const type = String(p.type ?? "");
    const hasDefault = p.default != null;
    rows.push({
      name,
      type,
      required: required.has(name),
      description: String(p.description ?? ""),
      defaultStr: hasDefault ? String(p.default) : "",
      scalar: SCALAR_TYPES.includes(type),
    });
    // Expand items.properties for array params so sub-fields are editable.
    if (type === "array") {
      const items = asRecord(p.items);
      const itemRequired = new Set<string>(asStringList(items.required));
      if (isRecord(items.properties)) {
        for (const [subName, rawSub] of Object.entries(items.properties)) {
          const subP = asRecord(rawSub);
          const subType = String(subP.type ?? "");
          rows.push({
            name: subName,
            type: subType,
            required: itemRequired.has(subName),
            description: String(subP.description ?? ""),
            defaultStr: subP.default != null ? String(subP.default) : "",
            scalar: SCALAR_TYPES.includes(subType),
            parentKey: name,
          });
        }
      }
    }
  }
  return rows;
}

// coerceDefault turns the edited default string back into a typed JSON value, or
// undefined to drop the "default" key entirely.
function coerceDefault(type: string, raw: string): unknown {
  const s = raw.trim();
  if (s === "") return undefined;
  if (type === "integer" || type === "number") {
    const n = Number(s);
    return Number.isNaN(n) ? undefined : n;
  }
  if (type === "boolean") return s === "true";
  return raw;
}

// applyRows writes edited rows back into a deep-copied schema (structure untouched).

export function applyRows(schema: JsonSchema, rows: ParamRow[]): JsonSchema {
  const next: JsonSchema = structuredClone(schema ?? {});
  const props = asRecord(next.properties);
  for (const r of rows) {
    let target: JsonRecord | undefined;
    if (r.parentKey) {
      // Sub-field of an array param's items.properties
      const sub = asRecord(asRecord(asRecord(props[r.parentKey]).items).properties)[r.name];
      target = isRecord(sub) ? sub : undefined;
    } else {
      const p = props[r.name];
      target = isRecord(p) ? p : undefined;
    }
    if (!target) continue;
    target.description = r.description;
    if (r.scalar) {
      const dv = coerceDefault(r.type, r.defaultStr);
      if (dv === undefined) delete target.default;
      else target.default = dv;
    }
  }
  return next;
}
