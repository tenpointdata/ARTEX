"use client";

import * as React from "react";
import { toast } from "sonner";
import {
  ListFilterIcon,
  PencilIcon,
  PlusIcon,
  ShieldAlertIcon,
  Trash2Icon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Card, CardContent } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { useCan } from "@/hooks/use-session";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import type {
  InterceptAction,
  InterceptRule,
  Tool,
} from "@/lib/types";

import { JudgeCard } from "./_components/judge-card";
import { Field, tRich } from "./_components/shared";

// ---- tool scope ----

// SDK tools are intentionally not seeded into the DB (they apply to every agent
// and have no per-agent binding). We hardcode them here so they still appear in
// the scope dialog.
function sdkTool(key: string, description: string): Tool {
  return { key, system: true, description, schema: {}, agents: [], enabled: true, kind: "builtin" };
}

const SDK_EXEC: Tool[] = [
  sdkTool("Bash",        t("Execute commands in a shell")),
  sdkTool("WebFetch",    t("Make HTTP/HTTPS requests (with proxy support)")),
  sdkTool("web_search",  t("Web search")),
  sdkTool("shell_open",  t("Open a persistent PTY interactive session")),
  sdkTool("shell_send",  t("Send input to an interactive session")),
  sdkTool("shell_read",  t("Read output from an interactive session")),
  sdkTool("shell_close", t("Close an interactive session")),
  sdkTool("shell_list",  t("List all interactive sessions")),
];

const SDK_WRITE: Tool[] = [
  sdkTool("Write",     t("Write files")),
  sdkTool("Edit",      t("Edit files (exact replacement)")),
  sdkTool("MultiEdit", t("Edit files in batch")),
];

const SDK_KEYS = new Set([...SDK_EXEC, ...SDK_WRITE].map((tool) => tool.key));

function groupTools(dbTools: Tool[]) {
  const sys: Tool[] = [], custom: Tool[] = [];
  for (const tool of dbTools) {
    if (SDK_KEYS.has(tool.key)) continue; // already covered by hardcoded groups
    if (tool.system) sys.push(tool);
    else             custom.push(tool);
  }
  return [
    { label: t("Execution"),    tools: SDK_EXEC },
    { label: t("Write/Edit"),   tools: SDK_WRITE },
    { label: t("System tools"), tools: sys },
    { label: t("Custom tools"), tools: custom },
  ].filter((g) => g.tools.length > 0);
}

// ---- form state ----

type RuleForm = {
  name: string;
  enabled: boolean;
  priority: number;
  match_target: "tool_name" | "tool_input";
  match_type: "string" | "regex" | "argv";
  pattern: string;
  action: InterceptAction;
  message: string;
  timeout_enabled: boolean;
  timeout_seconds: number;
  timeout_action: "deny" | "allow";
};

const defaultForm = (): RuleForm => ({
  name: "",
  enabled: true,
  priority: 0,
  match_target: "tool_name",
  match_type: "string",
  pattern: "",
  action: "deny",
  message: "",
  timeout_enabled: true,
  timeout_seconds: 60,
  timeout_action: "deny",
});

// An argv pattern (JSON array of tokens) shown as a prefix: ["nmap"] -> "nmap *".
function argvLabel(pattern: string): string {
  try {
    const tokens: unknown = JSON.parse(pattern);
    if (Array.isArray(tokens)) return `${tokens.join(" ")} *`;
  } catch {
    // not a JSON array: show the raw pattern
  }
  return pattern;
}

// ---- small components ----

function ActionBadge({ action }: { action: InterceptAction }) {
  if (action === "allow") return <Badge variant="secondary">{t("Allow")}</Badge>;
  if (action === "deny")  return <Badge variant="destructive">{t("Block")}</Badge>;
  return <Badge variant="outline" className="border-amber-400 text-amber-600">{t("Ask")}</Badge>;
}

export default function InterceptPage() {
  const [rules, setRules]     = React.useState<InterceptRule[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [open, setOpen]       = React.useState(false);
  const [editing, setEditing] = React.useState<InterceptRule | null>(null);
  const [form, setForm]       = React.useState<RuleForm>(defaultForm());
  // Rules, scope and the judge are admin-only to change (server/rbac_policy.go); other roles read them.
  const canAdmin = useCan("admin");
  const [saving, setSaving]   = React.useState(false);
  const [regexErr, setRegexErr] = React.useState("");
  const [regexWarn, setRegexWarn] = React.useState(false); // true = JS cannot parse it but it may be valid Go syntax

  // ---- tool scope dialog ----
  const [scopeOpen, setScopeOpen]       = React.useState(false);
  const [allTools, setAllTools]         = React.useState<Tool[]>([]);
  const [enabledTools, setEnabledTools] = React.useState<Set<string>>(new Set());
  const [scopeLoading, setScopeLoading] = React.useState(false);
  const [scopeSaving, setScopeSaving]   = React.useState(false);
  const [scopeTools, setScopeTools]     = React.useState<string[]>([]); // header info bar: tools currently under interception

  // ---- data ----

  const loadScope = React.useCallback(async () => {
    try {
      const cfg = await api.interceptGetToolConfig();
      setScopeTools(cfg.enabled_tools);
    } catch {
      // Info bar is non-critical; stay silent on failure
    }
  }, []);

  const load = React.useCallback(async () => {
    try {
      const r = await api.interceptRules();
      setRules(r);
    } catch {
      toast.error(t("Failed to load interception rules"));
    } finally {
      setLoading(false);
    }
  }, []);

  React.useEffect(() => { load(); loadScope(); }, [load, loadScope]);

  React.useEffect(() => {
    if (form.match_type !== "regex" || !form.pattern) { setRegexErr(""); setRegexWarn(false); return; }
    try {
      new RegExp(form.pattern);
      setRegexErr("");
      setRegexWarn(false);
    } catch {
      // JS RegExp does not support Go RE2 extended syntax (e.g. the (?i) inline flag).
      // This is only a preview validation failure, not proof of invalidity on the Go side; the server does the final check.
      setRegexErr("");
      setRegexWarn(true);
    }
  }, [form.pattern, form.match_type]);

  // ---- rule handlers ----

  function set(patch: Partial<RuleForm>) { setForm(f => ({ ...f, ...patch })); }

  function openNew() {
    setEditing(null);
    setForm(defaultForm());
    setRegexErr("");
    setOpen(true);
  }

  function openEdit(rule: InterceptRule) {
    setEditing(rule);
    setForm({
      name: rule.name, enabled: rule.enabled, priority: rule.priority,
      match_target: rule.match_target, match_type: rule.match_type,
      pattern: rule.pattern, action: rule.action, message: rule.message,
      timeout_enabled: rule.timeout_enabled, timeout_seconds: rule.timeout_seconds,
      timeout_action: rule.timeout_action,
    });
    setRegexErr("");
    setOpen(true);
  }

  async function handleSave() {
    if (!form.name.trim())    { toast.error(t("Name cannot be empty")); return; }
    if (!form.pattern.trim()) { toast.error(t("Pattern cannot be empty")); return; }
    if (regexErr)             { toast.error(t("Invalid regex syntax")); return; }
    setSaving(true);
    try {
      if (editing) {
        await api.updateInterceptRule(editing.id, form);
        toast.success(t("Rule updated"));
      } else {
        await api.createInterceptRule(form);
        toast.success(t("Rule created"));
      }
      setOpen(false);
      load();
    } catch (e) {
      toast.error(errorText(e));
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    try {
      await api.deleteInterceptRule(id);
      toast.success(t("Rule deleted"));
      load();
    } catch (e) {
      toast.error(errorText(e));
    }
  }

  async function handleToggle(rule: InterceptRule) {
    try {
      await api.toggleInterceptRule(rule.id, !rule.enabled);
      load();
    } catch (e) {
      toast.error(errorText(e));
    }
  }

  // ---- scope handlers ----

  async function openScope() {
    setScopeOpen(true);
    setScopeLoading(true);
    try {
      const [tools, cfg] = await Promise.all([api.tools(), api.interceptGetToolConfig()]);
      setAllTools(tools);
      setEnabledTools(new Set(cfg.enabled_tools));
    } catch (e) {
      toast.error(t("Load failed: {error}", { error: errorText(e) }));
    } finally {
      setScopeLoading(false);
    }
  }

  function toggleTool(key: string, val: boolean) {
    setEnabledTools(prev => {
      const next = new Set(prev);
      if (val) next.add(key); else next.delete(key);
      return next;
    });
  }

  async function saveScope() {
    setScopeSaving(true);
    try {
      await api.interceptSetToolConfig([...enabledTools]);
      toast.success(t("Interception scope saved"));
      setScopeTools([...enabledTools]);
      setScopeOpen(false);
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: errorText(e) }));
    } finally {
      setScopeSaving(false);
    }
  }

  const toolGroups = React.useMemo(() => groupTools(allTools), [allTools]);

  // ---- render ----

  return (
    <div className="flex flex-1 flex-col gap-5 p-6">

      {/* ---- header ---- */}
      <div className="flex items-center gap-2.5">
        <ShieldAlertIcon className="h-5 w-5 shrink-0" />
        <div>
          <h1 className="text-lg font-semibold leading-tight">{t("Command interception")}</h1>
          <p className="text-sm text-muted-foreground mt-0.5">
            {t("Tools are matched against interception rules before execution; unmatched commands can be judged by the model fallback")}
          </p>
        </div>
      </div>

      {/* ---- interception scope info bar (shared by rule matching and the model fallback: tools outside the scope are handled by neither) ---- */}
      <div
        className={`flex items-center justify-between gap-3 rounded-lg border px-4 py-2.5 ${
          scopeTools.length === 0
            ? "border-amber-400/60 bg-amber-50/50 dark:bg-amber-950/20"
            : "bg-muted/40"
        }`}
      >
        <div className="flex min-w-0 items-center gap-2 text-sm">
          <ListFilterIcon className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="shrink-0 font-medium">{t("Interception scope")}</span>
          {scopeTools.length === 0 ? (
            <span className="text-amber-700 dark:text-amber-500">
              {t("No tools enabled — neither interception rules nor the model fallback take effect")}
            </span>
          ) : (
            <>
              <Badge variant="secondary" className="shrink-0">{t("{count} tools", { count: scopeTools.length })}</Badge>
              <span className="truncate text-muted-foreground" title={scopeTools.join(", ")}>
                {scopeTools.join(", ")}
              </span>
            </>
          )}
        </div>
        <Button
          variant={scopeTools.length === 0 ? "default" : "outline"}
          size="sm"
          className="shrink-0"
          onClick={openScope}
        >
          <ListFilterIcon className="h-4 w-4" />
          {t("Adjust scope")}
        </Button>
      </div>

      <Tabs defaultValue="rules" className="flex-1">
        <TabsList>
          <TabsTrigger value="rules">{t("Interception rules")}</TabsTrigger>
          <TabsTrigger value="judge">{t("Model config")}</TabsTrigger>
        </TabsList>

        {/* ---- tab: interception rules ---- */}
        <TabsContent value="rules" className="mt-4 flex flex-col gap-4">
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">
              {t("Rules are matched one by one in priority order (higher numbers first); the first match wins")}
            </p>
            {canAdmin && (
              <Button onClick={openNew} size="sm" className="shrink-0">
                <PlusIcon className="h-4 w-4" />
                {t("Create rule")}
              </Button>
            )}
          </div>

          <Card>
            <CardContent className="p-0">
          {loading ? (
            <p className="p-6 text-sm text-muted-foreground">{t("Loading…")}</p>
          ) : rules.length === 0 ? (
            <div className="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <ShieldAlertIcon className="h-8 w-8 text-muted-foreground/40" />
              <p className="text-sm text-muted-foreground">{t("No rules yet")}</p>
              {canAdmin && (
                <Button size="sm" variant="outline" onClick={openNew}>
                  <PlusIcon className="h-4 w-4" />
                  {t("Create the first rule")}
                </Button>
              )}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent">
                  <TableHead className="w-[72px]">{t("Priority")}</TableHead>
                  <TableHead>{t("Name")}</TableHead>
                  <TableHead className="w-[90px]">{t("Target")}</TableHead>
                  <TableHead className="w-[80px]">{t("Type")}</TableHead>
                  <TableHead>{t("Pattern")}</TableHead>
                  <TableHead className="w-[72px]">{t("Policy")}</TableHead>
                  <TableHead className="w-[64px] text-center">{t("Enabled")}</TableHead>
                  <TableHead className="w-[80px]" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
                  <TableRow key={rule.id} className={!rule.enabled ? "opacity-40" : ""}>
                    <TableCell>
                      <span className="font-mono text-xs tabular-nums">{rule.priority}</span>
                    </TableCell>
                    <TableCell className="font-medium text-sm">{rule.name}</TableCell>
                    <TableCell>
                      <span className="text-xs text-muted-foreground">
                        {rule.match_target === "tool_name" ? t("Tool name") : t("Input content")}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="text-xs text-muted-foreground">
                        {rule.match_type === "regex" ? t("Regex") : rule.match_type === "argv" ? t("Argv prefix") : t("String")}
                      </span>
                    </TableCell>
                    <TableCell className="max-w-[220px]">
                      <code className="block truncate rounded bg-muted px-1.5 py-0.5 text-xs font-mono">
                        {rule.match_type === "argv" ? argvLabel(rule.pattern) : rule.pattern}
                      </code>
                    </TableCell>
                    <TableCell>
                      <ActionBadge action={rule.action} />
                    </TableCell>
                    <TableCell className="text-center">
                      <Switch
                        checked={rule.enabled}
                        disabled={!canAdmin}
                        onCheckedChange={() => handleToggle(rule)}
                      />
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center justify-end gap-0.5">
                        <Button
                          size="icon" variant="ghost" className="h-7 w-7"
                          onClick={() => openEdit(rule)}
                        >
                          <PencilIcon className="h-3.5 w-3.5" />
                        </Button>
                        {canAdmin && (
                          <Button
                            size="icon" variant="ghost"
                            className="h-7 w-7 text-destructive hover:text-destructive"
                            onClick={() => handleDelete(rule.id)}
                          >
                            <Trash2Icon className="h-3.5 w-3.5" />
                          </Button>
                        )}
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
            </CardContent>
          </Card>
        </TabsContent>

        {/* ---- tab: model config ---- */}
        <TabsContent value="judge" className="mt-4">
          <JudgeCard />
        </TabsContent>
      </Tabs>

      {/* ---- editor sheet ---- */}
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="right" className="flex flex-col gap-0 p-0 sm:max-w-md">
          <SheetHeader className="border-b px-6 py-4">
            <SheetTitle>{editing ? t("Edit rule") : t("Create rule")}</SheetTitle>
            <SheetDescription className="text-xs">
              {t("Higher priority matches first; the first matching rule wins and the rest are skipped")}
            </SheetDescription>
          </SheetHeader>

          <fieldset disabled={!canAdmin} className="flex-1 min-h-0 min-w-0 overflow-y-auto px-6 py-5 space-y-5">
            <Field label={t("Name")}>
              <Input
                placeholder={t("Give the rule a name")}
                value={form.name}
                onChange={(e) => set({ name: e.target.value })}
              />
            </Field>

            <Field label={t("Priority (higher numbers match first)")}>
              <Input
                type="number"
                value={form.priority}
                onChange={(e) => set({ priority: parseInt(e.target.value) || 0 })}
              />
            </Field>

            <Separator />

            <Field label={t("Match target")}>
              <Select
                value={form.match_target}
                onValueChange={(v) => set({ match_target: v as RuleForm["match_target"] })}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="tool_name">{t("Tool name (tool_name)")}</SelectItem>
                  <SelectItem value="tool_input">{t("Input content (tool_input JSON)")}</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            <Field label={t("Match type")}>
              <Select
                value={form.match_type}
                onValueChange={(v) => {
                  const match_type = v as RuleForm["match_type"];
                  // An argv rule always matches the Bash command in tool_input.
                  set(match_type === "argv" ? { match_type, match_target: "tool_input" } : { match_type });
                }}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="string">{t("String contains")}</SelectItem>
                  <SelectItem value="regex">{t("Regular expression")}</SelectItem>
                  <SelectItem value="argv">{t("Argv prefix (Bash only)")}</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            <Field label={t("Pattern")}>
              <Input
                placeholder={form.match_type === "regex" ? "^Bash$" : form.match_type === "argv" ? '["git","push"]' : "rm -rf"}
                value={form.pattern}
                onChange={(e) => set({ pattern: e.target.value })}
                className={regexErr ? "border-destructive focus-visible:ring-destructive" : ""}
              />
              {regexErr && (
                <p className="text-xs text-destructive mt-1">{regexErr}</p>
              )}
              {regexWarn && (
                <p className="text-xs text-amber-600 mt-1">
                  {tRich("Contains Go RE2 extended syntax (e.g. {example}); the browser cannot preview it, and the server validates on submission", {
                    example: <code className="font-mono">(?i)</code>,
                  })}
                </p>
              )}
              {form.match_type === "argv" && (
                <p className="text-xs text-amber-600 mt-1">
                  {t("JSON array of 1 to 8 tokens: the program and its subcommands, no flags.")}{" "}
                  {tRich("Matches a single simple Bash command whose prefix is exactly {prefix}.", {
                    prefix: <code className="font-mono">{argvLabel(form.pattern)}</code>,
                  })}{" "}
                  {tRich("Flags are not constrained: {pattern} also covers {command}.", {
                    pattern: <code className="font-mono">[&quot;git&quot;,&quot;push&quot;]</code>,
                    command: <code className="font-mono">git push --force</code>,
                  })}
                </p>
              )}
            </Field>

            <Separator />

            <Field label={t("Interception policy")}>
              <Select
                value={form.action}
                onValueChange={(v) => set({ action: v as InterceptAction })}
              >
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent>
                  <SelectItem value="allow">{t("Allow — pass through directly, skipping the remaining rules")}</SelectItem>
                  <SelectItem value="deny">{t("Block — cut it off and return a denial message to the model")}</SelectItem>
                  <SelectItem value="ask">{t("Ask the user — wait for approval")}</SelectItem>
                </SelectContent>
              </Select>
            </Field>

            {form.action !== "allow" && (
              <Field label={form.action === "deny" ? t("Denial message (returned to the model)") : t("Approval note (optional)")}>
                <Textarea
                  placeholder={form.action === "deny" ? t("Blocked by the security policy") : ""}
                  value={form.message}
                  onChange={(e) => set({ message: e.target.value })}
                  rows={2}
                  className="resize-none"
                />
              </Field>
            )}

            {form.action === "ask" && (
              <>
                <Separator />
                <div className="flex items-center justify-between">
                  <div>
                    <p className="text-sm font-medium">{t("Enable approval timeout")}</p>
                    <p className="text-xs text-muted-foreground">{t("Handle automatically after the timeout instead of waiting")}</p>
                  </div>
                  <Switch
                    checked={form.timeout_enabled}
                    onCheckedChange={(v) => set({ timeout_enabled: v })}
                  />
                </div>
                {form.timeout_enabled && (
                  <div className="flex items-end gap-3">
                    <Field label={t("Timeout (seconds)")}>
                      <Input
                        type="number"
                        min={5}
                        className="w-28"
                        value={form.timeout_seconds}
                        onChange={(e) => {
                          const n = parseInt(e.target.value, 10);
                          if (n > 0) set({ timeout_seconds: n });
                        }}
                      />
                    </Field>
                    <Field label={t("Timeout action")}>
                      <Select
                        value={form.timeout_action}
                        onValueChange={(v) => set({ timeout_action: v as "deny" | "allow" })}
                      >
                        <SelectTrigger className="w-32"><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="deny">{t("Auto-deny")}</SelectItem>
                          <SelectItem value="allow">{t("Auto-allow")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </Field>
                  </div>
                )}
              </>
            )}

            <Separator />

            <div className="flex items-center gap-3">
              <Switch
                id="rule-enabled"
                checked={form.enabled}
                onCheckedChange={(v) => set({ enabled: v })}
              />
              <Label htmlFor="rule-enabled" className="cursor-pointer">{t("Enable this rule")}</Label>
            </div>
          </fieldset>

          {canAdmin && (
            <SheetFooter className="border-t px-6 py-4 flex-row justify-end gap-2">
              <Button variant="outline" onClick={() => setOpen(false)}>{t("Cancel")}</Button>
              <Button onClick={handleSave} disabled={saving || !!regexErr}>
                {saving ? t("Saving…") : t("Save")}
              </Button>
            </SheetFooter>
          )}
        </SheetContent>
      </Sheet>

      {/* ---- scope dialog ---- */}
      <Dialog open={scopeOpen} onOpenChange={setScopeOpen}>
        <DialogContent className="sm:max-w-lg flex flex-col overflow-hidden p-0 gap-0" style={{ maxHeight: "min(80vh, 560px)" }}>
          <DialogHeader className="shrink-0 border-b px-6 py-4">
            <DialogTitle className="flex items-center gap-2">
              <ListFilterIcon className="h-4 w-4" />
              {t("Interception scope")}
            </DialogTitle>
            <DialogDescription className="text-xs">
              {t("Only tools with interception enabled go through rule matching; all other tools pass through directly")}
            </DialogDescription>
          </DialogHeader>

          <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-5">
            {scopeLoading ? (
              <p className="text-sm text-muted-foreground py-4">{t("Loading…")}</p>
            ) : (
              toolGroups.map((group, gi) => (
                <div key={group.label}>
                  {gi > 0 && <Separator className="mb-5" />}
                  <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                    {group.label}
                  </p>
                  <div className="space-y-0.5">
                    {group.tools.map((tool) => (
                      <div key={tool.key} className="flex items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/50">
                        <Switch
                          id={`scope-${tool.key}`}
                          checked={enabledTools.has(tool.key)}
                          disabled={!canAdmin}
                          onCheckedChange={(v) => toggleTool(tool.key, v)}
                        />
                        <label htmlFor={`scope-${tool.key}`} className="flex-1 min-w-0 cursor-pointer">
                          <div className="flex items-center gap-1.5">
                            <span className="font-mono text-sm">{tool.key}</span>
                            {(tool.kind && tool.kind !== "builtin") && (
                              <Badge variant="outline" className="px-1 py-0 text-[10px]">{tool.kind}</Badge>
                            )}
                          </div>
                          {tool.description && (
                            <p className="text-[11px] text-muted-foreground line-clamp-1">{tool.description}</p>
                          )}
                        </label>
                      </div>
                    ))}
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="shrink-0 border-t px-6 py-3 flex justify-end gap-2">
            <Button variant="outline" size="sm" onClick={() => setScopeOpen(false)}>{t("Cancel")}</Button>
            {canAdmin && (
              <Button size="sm" onClick={saveScope} disabled={scopeSaving || scopeLoading}>
                {scopeSaving ? t("Saving…") : t("Save")}
              </Button>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}
