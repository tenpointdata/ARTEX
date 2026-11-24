"use client";

import * as React from "react";

import { CheckIcon, PencilIcon, PlusIcon, ShieldCheckIcon, Trash2Icon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import type { AssetInterceptKind, AssetInterceptRule } from "@/lib/types";

const TASK_RULE_KIND_OPTIONS: { value: AssetInterceptKind; label: string; placeholder: string }[] = [
  { value: "exact_domain", label: t("Domain (exact)"), placeholder: "example.gov.cn" },
  { value: "exact_ip", label: t("IP (exact)"), placeholder: "203.0.113.10" },
  { value: "exact_url", label: t("URL (exact)"), placeholder: "https://example.com/login" },
  { value: "fuzzy_domain", label: t("Domain (fuzzy)"), placeholder: ".gov.cn" },
  { value: "fuzzy_ip", label: t("IP (fuzzy)"), placeholder: "203.0.113." },
  { value: "fuzzy_url", label: t("URL (fuzzy)"), placeholder: "/admin" },
  { value: "cidr", label: "CIDR", placeholder: "192.168.0.0/16" },
];

const TASK_RULE_KIND_LABEL: Record<AssetInterceptKind, string> = Object.fromEntries(
  TASK_RULE_KIND_OPTIONS.map((o) => [o.value, o.label]),
) as Record<AssetInterceptKind, string>;

// TaskInterceptRulesCard manages "task-level asset intercept / allow rules" on the task detail overview:
// list + add + inline edit + delete + enable toggle. Rules apply to this task only and never enter the global table.
export function TaskInterceptRulesCard({ taskId }: { taskId: string }) {
  const isAdmin = useCan("admin"); // rule mutations are admin-only (server/rbac_policy.go)
  const [rules, setRules] = React.useState<AssetInterceptRule[]>([]);
  const [busy, setBusy] = React.useState(false);
  const [err, setErr] = React.useState("");
  const [newAction, setNewAction] = React.useState<"block" | "allow">("block");
  const [newKind, setNewKind] = React.useState<AssetInterceptKind>("fuzzy_domain");
  const [newPattern, setNewPattern] = React.useState("");
  const [newNote, setNewNote] = React.useState("");
  const [editId, setEditId] = React.useState<number | null>(null);
  const [editAction, setEditAction] = React.useState<"block" | "allow">("block");
  const [editKind, setEditKind] = React.useState<AssetInterceptKind>("fuzzy_domain");
  const [editPattern, setEditPattern] = React.useState("");
  const [editNote, setEditNote] = React.useState("");

  const load = React.useCallback(async () => {
    try {
      setRules(await api.taskInterceptRules(taskId));
    } catch {
      // Ignore transient errors
    }
  }, [taskId]);

  React.useEffect(() => {
    void load();
  }, [load]);

  async function add() {
    if (!newPattern.trim()) return;
    setBusy(true);
    setErr("");
    try {
      await api.createTaskInterceptRule(taskId, {
        action: newAction,
        kind: newKind,
        pattern: newPattern.trim(),
        note: newNote.trim(),
        enabled: true,
      });
      setNewPattern("");
      setNewNote("");
      await load();
    } catch (e) {
      setErr(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  function startEdit(r: AssetInterceptRule) {
    setEditId(r.id);
    setEditAction(r.action ?? "block");
    setEditKind(r.kind);
    setEditPattern(r.pattern);
    setEditNote(r.note);
    setErr("");
  }

  async function saveEdit(r: AssetInterceptRule) {
    if (!editPattern.trim()) return;
    setBusy(true);
    setErr("");
    try {
      await api.updateTaskInterceptRule(taskId, r.id, {
        action: editAction,
        kind: editKind,
        pattern: editPattern.trim(),
        note: editNote.trim(),
        enabled: r.enabled,
      });
      setEditId(null);
      await load();
    } catch (e) {
      setErr(errorText(e));
    } finally {
      setBusy(false);
    }
  }

  async function remove(r: AssetInterceptRule) {
    setRules((prev) => prev.filter((x) => x.id !== r.id));
    try {
      await api.deleteTaskInterceptRule(taskId, r.id);
    } catch {
      await load();
    }
  }

  async function toggle(r: AssetInterceptRule) {
    setRules((prev) => prev.map((x) => (x.id === r.id ? { ...x, enabled: !x.enabled } : x)));
    try {
      await api.toggleTaskInterceptRule(taskId, r.id, !r.enabled);
    } catch {
      await load();
    }
  }

  const placeholder = TASK_RULE_KIND_OPTIONS.find((o) => o.value === newKind)?.placeholder ?? "";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheckIcon className="size-4 text-sky-500" /> {t("Task-level asset intercept / allow")}
          <span className="text-muted-foreground text-xs font-normal">
            {t("(this task only, never global; block rules evaluate before allow rules, {count} in total)", {
              count: rules.length,
            })}
          </span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {/* Add form */}
        {isAdmin && (
          <div className="flex flex-wrap items-center gap-2">
            <NativeSelect
              size="sm"
              value={newAction}
              onChange={(e) => setNewAction(e.target.value as "block" | "allow")}
            >
              <NativeSelectOption value="block">{t("Block")}</NativeSelectOption>
              <NativeSelectOption value="allow">{t("Allow")}</NativeSelectOption>
            </NativeSelect>
            <NativeSelect size="sm" value={newKind} onChange={(e) => setNewKind(e.target.value as AssetInterceptKind)}>
              {TASK_RULE_KIND_OPTIONS.map((o) => (
                <NativeSelectOption key={o.value} value={o.value}>
                  {o.label}
                </NativeSelectOption>
              ))}
            </NativeSelect>
            <Input
              className="h-7 min-w-56 flex-1 text-sm"
              placeholder={placeholder}
              value={newPattern}
              onChange={(e) => setNewPattern(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") void add();
              }}
              disabled={busy}
            />
            <Input
              className="h-7 w-36 text-sm"
              placeholder={t("Note (optional)")}
              value={newNote}
              onChange={(e) => setNewNote(e.target.value)}
              disabled={busy}
            />
            <Button size="sm" variant="outline" disabled={busy || !newPattern.trim()} onClick={() => void add()}>
              <PlusIcon className="size-3.5" /> {t("Add")}
            </Button>
            {err && <span className="text-xs text-red-500">{err}</span>}
          </div>
        )}
        {/* Rule list */}
        {rules.length > 0 ? (
          <div className="flex flex-col gap-1.5">
            {rules.map((r) =>
              editId === r.id ? (
                <div key={r.id} className="flex flex-wrap items-center gap-2 rounded-md border px-2.5 py-1.5">
                  <NativeSelect
                    size="sm"
                    value={editAction}
                    onChange={(e) => setEditAction(e.target.value as "block" | "allow")}
                  >
                    <NativeSelectOption value="block">{t("Block")}</NativeSelectOption>
                    <NativeSelectOption value="allow">{t("Allow")}</NativeSelectOption>
                  </NativeSelect>
                  <NativeSelect
                    size="sm"
                    value={editKind}
                    onChange={(e) => setEditKind(e.target.value as AssetInterceptKind)}
                  >
                    {TASK_RULE_KIND_OPTIONS.map((o) => (
                      <NativeSelectOption key={o.value} value={o.value}>
                        {o.label}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                  <Input
                    className="h-7 min-w-56 flex-1 text-sm"
                    value={editPattern}
                    onChange={(e) => setEditPattern(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void saveEdit(r);
                      if (e.key === "Escape") setEditId(null);
                    }}
                    disabled={busy}
                    autoFocus
                  />
                  <Input
                    className="h-7 w-36 text-sm"
                    placeholder={t("Note (optional)")}
                    value={editNote}
                    onChange={(e) => setEditNote(e.target.value)}
                    disabled={busy}
                  />
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 shrink-0 px-1.5"
                    disabled={busy || !editPattern.trim()}
                    onClick={() => void saveEdit(r)}
                  >
                    <CheckIcon className="size-3.5 text-emerald-500" />
                  </Button>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 shrink-0 px-1.5"
                    disabled={busy}
                    onClick={() => setEditId(null)}
                  >
                    <XIcon className="size-3.5" />
                  </Button>
                </div>
              ) : (
                <div key={r.id} className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                  <span
                    className={`shrink-0 rounded px-1.5 py-0.5 text-xs ${
                      r.action === "allow"
                        ? "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400"
                        : "bg-red-500/15 text-red-600 dark:text-red-400"
                    }`}
                  >
                    {r.action === "allow" ? t("Allow") : t("Block")}
                  </span>
                  <span className="text-muted-foreground shrink-0 text-xs">{TASK_RULE_KIND_LABEL[r.kind]}</span>
                  <code className="bg-muted min-w-0 flex-1 truncate rounded px-1.5 py-0.5 text-xs">{r.pattern}</code>
                  {r.note && (
                    <span className="text-muted-foreground max-w-[120px] shrink-0 truncate text-xs">{r.note}</span>
                  )}
                  <Switch checked={r.enabled} onCheckedChange={() => void toggle(r)} disabled={!isAdmin} />
                  {isAdmin && (
                    <>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 shrink-0 px-1.5"
                        disabled={busy}
                        onClick={() => startEdit(r)}
                      >
                        <PencilIcon className="size-3.5" />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 shrink-0 px-1.5"
                        disabled={busy}
                        onClick={() => void remove(r)}
                      >
                        <Trash2Icon className="size-3.5 text-red-500" />
                      </Button>
                    </>
                  )}
                </div>
              ),
            )}
          </div>
        ) : (
          <p className="text-muted-foreground text-sm">
            {t(
              'No task-level rules. A "Block" hit forbids testing; "Allow" is a whitelist — once configured, this task only permits assets that match an allow rule (no whitelist when unconfigured).',
            )}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
