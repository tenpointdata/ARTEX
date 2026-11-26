"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Sheet, SheetContent, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Agent, MCPServer } from "@/lib/types";

export function CreateSkillSheet({
  open,
  onOpenChange,
  agents,
  mcpOptions,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  agents: Agent[];
  mcpOptions: MCPServer[];
  onCreated: () => void;
}) {
  const canAdmin = useCan("admin"); // initial visibility is /api/visibility (admin)
  const [newName, setNewName] = React.useState("");
  const [newDesc, setNewDesc] = React.useState("");
  const [newLicense, setNewLicense] = React.useState("");
  const [newCompat, setNewCompat] = React.useState("");
  const [newInst, setNewInst] = React.useState("");
  const [newMcps, setNewMcps] = React.useState<string[]>([]);
  const [newVisibility, setNewVisibility] = React.useState<string[]>([]);
  const [creatingSkill, setCreatingSkill] = React.useState(false);
  const setNewOpen = onOpenChange;

  async function createNewSkill() {
    if (!newName.trim()) {
      toast.error(t("Please fill in the name"));
      return;
    }
    if (!newDesc.trim()) {
      toast.error(t("description is required"));
      return;
    }
    setCreatingSkill(true);
    try {
      const name = newName.trim();
      await api.createSkill({
        name,
        description: newDesc.trim(),
        license: newLicense.trim() || undefined,
        compatibility: newCompat.trim() || undefined,
        mcps: newMcps.length ? newMcps : undefined,
        instructions: newInst.trim() || undefined,
      });
      // apply initial visibility (fire-and-forget per agent; best-effort)
      await Promise.all(newVisibility.map((id) => api.toggleSkillVisibility(id, name, true)));
      toast.success(t("Skill created"));
      setNewOpen(false);
      setNewName("");
      setNewDesc("");
      setNewLicense("");
      setNewCompat("");
      setNewInst("");
      setNewMcps([]);
      setNewVisibility([]);
      onCreated();
    } catch (e) {
      toast.error(t("Create failed: {error}", { error: (e as Error).message }));
    } finally {
      setCreatingSkill(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={setNewOpen}>
      <SheetContent side="right" className="flex w-full flex-col gap-0 data-[side=right]:sm:max-w-xl">
        <SheetHeader className="px-4 pt-4">
          <SheetTitle>{t("Create skill")}</SheetTitle>
        </SheetHeader>
        <Tabs defaultValue="basic" className="flex min-h-0 flex-1 flex-col">
          <TabsList className="mx-4 mt-3 shrink-0 justify-start">
            <TabsTrigger value="basic">{t("Basics")}</TabsTrigger>
            <TabsTrigger value="mcp">
              {t("Linked MCP")}
              {newMcps.length > 0 && (
                <span className="ml-1.5 rounded-full bg-primary px-1.5 py-0.5 text-[10px] leading-none text-primary-foreground">
                  {newMcps.length}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger value="visibility">
              {t("Visibility")}
              {newVisibility.length > 0 && (
                <span className="ml-1.5 rounded-full bg-primary px-1.5 py-0.5 text-[10px] leading-none text-primary-foreground">
                  {newVisibility.length}
                </span>
              )}
            </TabsTrigger>
          </TabsList>

          {/* basics */}
          <TabsContent
            value="basic"
            className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pb-4 pt-4 data-[state=inactive]:hidden"
          >
            <div className="grid gap-1.5">
              <Label htmlFor="sk-name">
                {t("Name")} <span className="text-destructive">*</span>
              </Label>
              <Input
                id="sk-name"
                placeholder="sqli-deepdive"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
              />
              <p className="text-muted-foreground text-xs">
                {t("Lowercase letters / digits / hyphens, 1–64 characters")}
              </p>
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="sk-desc">
                {t("Description")} <span className="text-destructive">*</span>
              </Label>
              <Textarea
                id="sk-desc"
                rows={2}
                className="resize-none"
                placeholder={t("What this skill does and when to use it.")}
                value={newDesc}
                onChange={(e) => setNewDesc(e.target.value)}
              />
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="grid gap-1.5">
                <Label className="text-muted-foreground text-xs">license</Label>
                <Input
                  placeholder={t("MIT / Proprietary")}
                  value={newLicense}
                  onChange={(e) => setNewLicense(e.target.value)}
                />
              </div>
              <div className="grid gap-1.5">
                <Label className="text-muted-foreground text-xs">compatibility</Label>
                <Input
                  placeholder={t("Requires sqlmap, python3")}
                  value={newCompat}
                  onChange={(e) => setNewCompat(e.target.value)}
                />
              </div>
            </div>
            <div className="flex min-h-0 flex-1 flex-col gap-1.5">
              <Label htmlFor="sk-inst">
                {t("Body")}{" "}
                <span className="text-muted-foreground text-xs font-normal">
                  {t("(skeleton generated automatically when empty)")}
                </span>
              </Label>
              <Textarea
                id="sk-inst"
                className="min-h-40 flex-1 resize-none font-mono text-sm leading-relaxed"
                placeholder={t(
                  "## How to run\n\n1. Probe for errors first\n2. Distinguish blind injection types\n\nScripts go in the scripts/ directory.",
                )}
                value={newInst}
                onChange={(e) => setNewInst(e.target.value)}
              />
            </div>
          </TabsContent>

          {/* linked MCP */}
          <TabsContent value="mcp" className="overflow-y-auto px-4 pb-4 pt-4 data-[state=inactive]:hidden">
            <p className="mb-3 text-xs text-muted-foreground">
              {t("The selected MCP's tools are disclosed and unlocked only when the skill loads.")}
            </p>
            {mcpOptions.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t('No MCP servers yet; add one on the "MCP" page.')}</p>
            ) : (
              <div className="flex flex-col gap-2">
                {mcpOptions.map((m) => (
                  <label
                    key={m.id}
                    htmlFor={`new-skill-mcp-${m.id}`}
                    className="flex cursor-pointer items-center gap-2 text-sm"
                  >
                    <Checkbox
                      id={`new-skill-mcp-${m.id}`}
                      checked={newMcps.includes(m.name)}
                      onCheckedChange={(on) =>
                        setNewMcps((cur) => (on ? [...cur, m.name] : cur.filter((n) => n !== m.name)))
                      }
                    />
                    {m.name}
                  </label>
                ))}
              </div>
            )}
          </TabsContent>

          {/* visibility */}
          <TabsContent value="visibility" className="overflow-y-auto px-4 pb-4 pt-4 data-[state=inactive]:hidden">
            <p className="mb-3 text-xs text-muted-foreground">
              {t("The selected agents will see this skill once it is created.")}
            </p>
            {agents.length === 0 ? (
              <p className="text-xs text-muted-foreground">{t("(no agents)")}</p>
            ) : (
              <div className="flex flex-col gap-2">
                {agents.map((a) => (
                  <label
                    key={a.key}
                    htmlFor={`new-skill-agent-${a.id}`}
                    className="flex cursor-pointer items-center gap-2 text-sm"
                  >
                    <Checkbox
                      id={`new-skill-agent-${a.id}`}
                      checked={newVisibility.includes(a.id)}
                      disabled={!canAdmin}
                      onCheckedChange={(on) =>
                        setNewVisibility((cur) => (on ? [...cur, a.id] : cur.filter((id) => id !== a.id)))
                      }
                    />
                    {a.name}
                  </label>
                ))}
              </div>
            )}
          </TabsContent>
        </Tabs>

        <SheetFooter className="flex-row justify-end gap-2 border-t px-4 py-3">
          <Button variant="outline" onClick={() => setNewOpen(false)}>
            {t("Cancel")}
          </Button>
          <Button onClick={createNewSkill} disabled={creatingSkill}>
            {creatingSkill ? t("Creating…") : t("Create")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
