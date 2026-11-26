"use client";

import * as React from "react";

import { PlusIcon, ShieldCheckIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@/components/ui/native-select";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { TaskScopeRow } from "@/lib/types";

// Display value of one test-scope row: domain / CIDR / company.
function scopeValue(row: TaskScopeRow): string {
  if (row.value) return row.value;
  if (row.domain) return row.domain;
  if (row.net) return row.net;
  if (row.company_id) return row.company_name?.trim() ? row.company_name : t("Company #{id}", { id: row.company_id });
  return "—";
}

const SCOPE_KIND_LABELS: Record<TaskScopeRow["kind"], string> = {
  company: t("Company"),
  root_domain: t("Root domain"),
  subdomain: t("Subdomain"),
  ip: "IP",
  cidr: "CIDR",
  icp: "ICP",
  keyword: t("Keyword"),
};

const SCOPE_SOURCE_LABELS: Record<TaskScopeRow["source"], string> = {
  auto: t("Auto"),
  agent: t("Agent"),
  manual: t("Manual"),
};

export function ScopeCard({ taskId }: { taskId: string }) {
  const canOperate = useCan("operate");
  // Test scope list + add-form state.
  const [scope, setScope] = React.useState<TaskScopeRow[]>([]);
  const [scopeKind, setScopeKind] = React.useState<TaskScopeRow["kind"]>("root_domain");
  const [scopeValueInput, setScopeValueInput] = React.useState("");
  const [scopeBusy, setScopeBusy] = React.useState(false);
  const [scopeErr, setScopeErr] = React.useState("");
  const loadScope = React.useCallback(async () => {
    try {
      const resp = await api.taskScope(taskId);
      setScope(resp.scope);
    } catch {
      // Ignore: without an asset store the endpoint 503s and the scope card stays empty
    }
  }, [taskId]);

  const addScope = async () => {
    const value = scopeValueInput.trim();
    if (!value) return;
    setScopeBusy(true);
    setScopeErr("");
    try {
      await api.addTaskScope(taskId, scopeKind, value);
      setScopeValueInput("");
      await loadScope();
    } catch (e) {
      setScopeErr(e instanceof Error ? e.message : t("Failed to add"));
    } finally {
      setScopeBusy(false);
    }
  };

  const removeScope = async (row: TaskScopeRow) => {
    setScope((prev) => prev.filter((s) => s.id !== row.id));
    try {
      await api.deleteTaskScope(taskId, row.id);
    } catch {
      await loadScope(); // delete failed: re-fetch to restore
    }
  };

  React.useEffect(() => {
    void loadScope();
  }, [loadScope]);

  return (
    <>
      {/* Test scope: coverage denominator (coverage scope, not enforced); can be edited manually. */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheckIcon className="size-4 text-emerald-500" /> {t("Test scope")}
            <span className="text-muted-foreground text-xs font-normal">
              {t("(coverage scope, not enforced, {count} in total)", { count: scope.length })}
            </span>
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {/* Add form */}
          {canOperate && (
            <div className="flex flex-wrap items-center gap-2">
              <NativeSelect
                size="sm"
                value={scopeKind}
                onChange={(e) => setScopeKind(e.target.value as TaskScopeRow["kind"])}
              >
                <NativeSelectOption value="root_domain">{t("Root domain")}</NativeSelectOption>
                <NativeSelectOption value="subdomain">{t("Subdomain")}</NativeSelectOption>
                <NativeSelectOption value="ip">IP</NativeSelectOption>
                <NativeSelectOption value="cidr">CIDR</NativeSelectOption>
                <NativeSelectOption value="icp">ICP</NativeSelectOption>
                <NativeSelectOption value="keyword">{t("Keyword")}</NativeSelectOption>
                <NativeSelectOption value="company">{t("Company")}</NativeSelectOption>
              </NativeSelect>
              <Input
                className="h-7 w-56 text-sm"
                placeholder={
                  scopeKind === "company"
                    ? t("Company name or id")
                    : scopeKind === "ip" || scopeKind === "cidr"
                      ? t("e.g. 10.0.0.1 or 10.0.0.0/24")
                      : scopeKind === "icp"
                        ? t("e.g. Beijing ICP No.12345678-1")
                        : scopeKind === "keyword"
                          ? t("e.g. a company-name keyword")
                          : t("e.g. example.com")
                }
                value={scopeValueInput}
                onChange={(e) => setScopeValueInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") void addScope();
                }}
                disabled={scopeBusy}
              />
              <Button
                size="sm"
                variant="outline"
                disabled={scopeBusy || !scopeValueInput.trim()}
                onClick={() => void addScope()}
              >
                <PlusIcon className="size-3.5" /> {t("Add")}
              </Button>
              {scopeErr && <span className="text-xs text-red-500">{scopeErr}</span>}
            </div>
          )}
          {/* Scope list */}
          {scope.length > 0 ? (
            <div className="flex flex-col gap-1.5">
              {scope.map((row) => (
                <div key={row.id} className="flex items-center gap-2 rounded-md border px-2.5 py-1.5 text-sm">
                  <span className="bg-muted text-muted-foreground shrink-0 rounded px-1.5 py-0.5 text-xs">
                    {SCOPE_KIND_LABELS[row.kind]}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{scopeValue(row)}</span>
                  <span className="text-muted-foreground shrink-0 text-xs">{SCOPE_SOURCE_LABELS[row.source]}</span>
                  {row.task_id.toString() === taskId ? (
                    canOperate && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-6 shrink-0 px-1.5"
                        onClick={() => void removeScope(row)}
                      >
                        <Trash2Icon className="size-3.5 text-red-500" />
                      </Button>
                    )
                  ) : (
                    <span className="text-muted-foreground shrink-0 text-xs">{t("Inherited")}</span>
                  )}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("No test scope yet; once added it serves as the asset coverage denominator.")}
            </p>
          )}
        </CardContent>
      </Card>
    </>
  );
}
