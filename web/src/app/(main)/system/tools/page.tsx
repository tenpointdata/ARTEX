"use client";

import * as React from "react";

import { PlusIcon, SearchIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Agent, Tool } from "@/lib/types";

// Traffic tools are host tools gated by the global traffic capture switch: bindable, but
// only usable when capture is on. Keep in sync with traffic.SeedToolMetas.
import { CustomToolDialog } from "./_components/custom-tool-dialog";
import { ToolEditor } from "./_components/tool-editor";
import { ToolGridCard } from "./_components/tool-grid-card";

export default function ToolsPage() {
  const [tools, setTools] = React.useState<Tool[]>([]);
  const [agents, setAgents] = React.useState<Agent[]>([]);
  const [captureOn, setCaptureOn] = React.useState(false);
  const [selectedKey, setSelectedKey] = React.useState<string | null>(null);
  const [customEdit, setCustomEdit] = React.useState<Tool | "new" | null>(null);

  const reload = React.useCallback(() => {
    api
      .tools()
      .then(setTools)
      .catch(() => setTools([]));
  }, []);
  React.useEffect(() => {
    reload();
    api
      .agents()
      .then(setAgents)
      .catch(() => {});
    api
      .settings()
      .then((s) => setCaptureOn(!!s.traffic_capture))
      .catch(() => {});
  }, [reload]);

  const [query, setQuery] = React.useState("");

  const selected = tools.find((t) => t.key === selectedKey) ?? null;
  const matchTool = React.useCallback(
    (t: Tool) => {
      const q = query.trim().toLowerCase();
      if (!q) return true;
      return (
        t.key.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.agents.some((a) => a.toLowerCase().includes(q))
      );
    },
    [query],
  );
  const systemTools = tools.filter((t) => t.system && matchTool(t));
  const customTools = tools.filter((t) => !t.system && matchTool(t));
  const allSystemCount = tools.filter((t) => t.system).length;
  const allCustomCount = tools.filter((t) => !t.system).length;
  // clicking a built-in tool opens the binding drawer; a custom tool opens its full editor.
  const openTool = (t: Tool) => (t.system ? setSelectedKey(t.key) : setCustomEdit(t));

  return (
    <div className="flex flex-1 flex-col gap-4 md:gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-semibold text-xl tracking-tight">{t("Tools")}</h1>
          <p className="text-muted-foreground text-sm">
            {t("Descriptions/bindings for system tools, plus custom tools (command/script/http)")}
          </p>
        </div>
        <div className="relative w-64">
          <SearchIcon className="absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            className="h-8 pl-8 text-sm"
            placeholder={t("Search tool name, description, agent…")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>

      <Tabs defaultValue="system">
        <TabsList>
          <TabsTrigger value="system">{t("System tools")}</TabsTrigger>
          <TabsTrigger value="custom">{t("Custom tools")}</TabsTrigger>
        </TabsList>

        <TabsContent value="system">
          <Card>
            <CardHeader>
              <CardTitle>{t("System tools")}</CardTitle>
              <CardDescription>
                {query.trim()
                  ? t(
                      "{count} / {total} matching; click a card to edit the description, parameter defaults and agent bindings",
                      { count: systemTools.length, total: allSystemCount },
                    )
                  : t("{total} total; click a card to edit the description, parameter defaults and agent bindings", {
                      total: allSystemCount,
                    })}
              </CardDescription>
            </CardHeader>
            <CardContent>
              {systemTools.length === 0 ? (
                <p className="py-6 text-center text-muted-foreground text-sm">
                  {query.trim()
                    ? t("No matching system tools")
                    : t("(no system tools yet, waiting for the backend seed)")}
                </p>
              ) : (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {systemTools.map((t) => (
                    <ToolGridCard key={t.key} tool={t} onClick={() => openTool(t)} />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="custom">
          <Card>
            <CardHeader>
              <div className="flex items-start justify-between gap-3">
                <div>
                  <CardTitle>{t("Custom tools")}</CardTitle>
                  <CardDescription>
                    {query.trim()
                      ? t("shell/command/script/http, {count} / {total} matching; click a card to edit", {
                          count: customTools.length,
                          total: allCustomCount,
                        })
                      : t(
                          "shell (bash declaration) / command (command template) / script (Python) / http (API), {total} total; click a card to edit",
                          { total: allCustomCount },
                        )}
                  </CardDescription>
                </div>
                <Button size="sm" onClick={() => setCustomEdit("new")}>
                  <PlusIcon /> {t("Create custom tool")}
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {customTools.length === 0 ? (
                <p className="py-6 text-center text-muted-foreground text-sm">
                  {query.trim()
                    ? t("No matching custom tools")
                    : t("(no custom tools yet; click 'Create custom tool' in the top right)")}
                </p>
              ) : (
                <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
                  {customTools.map((t) => (
                    <ToolGridCard key={t.key} tool={t} onClick={() => openTool(t)} />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>

      <Sheet open={!!selected} onOpenChange={(o) => !o && setSelectedKey(null)}>
        <SheetContent side="right" className="gap-0 p-0 data-[side=right]:w-[30vw] data-[side=right]:sm:max-w-[30vw]">
          {selected && (
            <>
              <SheetHeader className="px-4">
                <SheetTitle className="font-mono">{selected.key}</SheetTitle>
                <SheetDescription>{t("Edit the description, parameter defaults and agent bindings")}</SheetDescription>
              </SheetHeader>
              <ToolEditor
                key={selected.key}
                tool={selected}
                agents={agents}
                captureOn={captureOn}
                onSaved={reload}
                onClose={() => setSelectedKey(null)}
              />
            </>
          )}
        </SheetContent>
      </Sheet>

      <CustomToolDialog
        edit={customEdit}
        agents={agents}
        onClose={() => setCustomEdit(null)}
        onSaved={() => {
          setCustomEdit(null);
          reload();
        }}
      />
    </div>
  );
}
