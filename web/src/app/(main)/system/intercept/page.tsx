"use client";

import * as React from "react";

import { ListFilterIcon, PlusIcon, ShieldAlertIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { InterceptRule, Tool } from "@/lib/types";

import { JudgeCard } from "./_components/judge-card";
import { defaultForm, type RuleForm } from "./_components/rule-form";
import { RuleSheet } from "./_components/rule-sheet";
import { RulesTable } from "./_components/rules-table";
import { ScopeDialog } from "./_components/scope-dialog";
import { groupTools } from "./_components/tool-groups";

// ---- page ----

export default function InterceptPage() {
  const canAdmin = useCan("admin"); // rules, scope and the judge are admin-only to change (server/rbac_policy.go)
  const [rules, setRules] = React.useState<InterceptRule[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [open, setOpen] = React.useState(false);
  const [editing, setEditing] = React.useState<InterceptRule | null>(null);
  const [form, setForm] = React.useState<RuleForm>(defaultForm());
  const [saving, setSaving] = React.useState(false);
  const [regexErr, setRegexErr] = React.useState("");
  const [regexWarn, setRegexWarn] = React.useState(false); // true = JS cannot parse it but it may be valid Go syntax

  // ---- tool scope dialog ----
  const [scopeOpen, setScopeOpen] = React.useState(false);
  const [allTools, setAllTools] = React.useState<Tool[]>([]);
  const [enabledTools, setEnabledTools] = React.useState<Set<string>>(new Set());
  const [scopeLoading, setScopeLoading] = React.useState(false);
  const [scopeSaving, setScopeSaving] = React.useState(false);
  const [scopeTools, setScopeTools] = React.useState<string[]>([]); // header info bar: tools currently under interception

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

  React.useEffect(() => {
    void load();
    void loadScope();
  }, [load, loadScope]);

  React.useEffect(() => {
    if (form.match_type !== "regex" || !form.pattern) {
      setRegexErr("");
      setRegexWarn(false);
      return;
    }
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

  function set(patch: Partial<RuleForm>) {
    setForm((f) => ({ ...f, ...patch }));
  }

  function openNew() {
    setEditing(null);
    setForm(defaultForm());
    setRegexErr("");
    setOpen(true);
  }

  function openEdit(rule: InterceptRule) {
    setEditing(rule);
    setForm({
      name: rule.name,
      enabled: rule.enabled,
      priority: rule.priority,
      match_target: rule.match_target,
      match_type: rule.match_type,
      pattern: rule.pattern,
      action: rule.action,
      message: rule.message,
      timeout_enabled: rule.timeout_enabled,
      timeout_seconds: rule.timeout_seconds,
      timeout_action: rule.timeout_action,
    });
    setRegexErr("");
    setOpen(true);
  }

  async function handleSave() {
    if (!form.name.trim()) {
      toast.error(t("Name cannot be empty"));
      return;
    }
    if (!form.pattern.trim()) {
      toast.error(t("Pattern cannot be empty"));
      return;
    }
    if (regexErr) {
      toast.error(t("Invalid regex syntax"));
      return;
    }
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
      void load();
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function handleDelete(id: number) {
    try {
      await api.deleteInterceptRule(id);
      toast.success(t("Rule deleted"));
      void load();
    } catch (e) {
      toast.error((e as Error).message);
    }
  }

  async function handleToggle(rule: InterceptRule) {
    try {
      await api.toggleInterceptRule(rule.id, !rule.enabled);
      void load();
    } catch (e) {
      toast.error((e as Error).message);
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
      toast.error(t("Load failed: {error}", { error: (e as Error).message }));
    } finally {
      setScopeLoading(false);
    }
  }

  function toggleTool(key: string, val: boolean) {
    setEnabledTools((prev) => {
      const next = new Set(prev);
      if (val) next.add(key);
      else next.delete(key);
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
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
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
            {t(
              "Tools are matched against interception rules before execution; unmatched commands can be judged by the model fallback",
            )}
          </p>
        </div>
      </div>

      {/* ---- interception scope info bar (shared by rule matching and the model fallback: tools outside the scope are handled by neither) ---- */}
      <div
        className={`flex items-center justify-between gap-3 rounded-lg border px-4 py-2.5 ${
          scopeTools.length === 0 ? "border-amber-400/60 bg-amber-50/50 dark:bg-amber-950/20" : "bg-muted/40"
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
              <Badge variant="secondary" className="shrink-0">
                {t("{count} tools", { count: scopeTools.length })}
              </Badge>
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

          <RulesTable
            rules={rules}
            loading={loading}
            openNew={openNew}
            openEdit={openEdit}
            handleDelete={handleDelete}
            handleToggle={handleToggle}
          />
        </TabsContent>

        {/* ---- tab: model config ---- */}
        <TabsContent value="judge" className="mt-4">
          <JudgeCard />
        </TabsContent>
      </Tabs>

      <RuleSheet
        open={open}
        setOpen={setOpen}
        editing={editing}
        form={form}
        set={set}
        regexErr={regexErr}
        regexWarn={regexWarn}
        saving={saving}
        handleSave={handleSave}
      />

      {/* ---- scope dialog ---- */}
      <ScopeDialog
        open={scopeOpen}
        onOpenChange={setScopeOpen}
        loading={scopeLoading}
        saving={scopeSaving}
        toolGroups={toolGroups}
        enabledTools={enabledTools}
        toggleTool={toggleTool}
        onSave={saveScope}
      />
    </div>
  );
}
