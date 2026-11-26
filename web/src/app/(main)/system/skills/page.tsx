"use client";

import * as React from "react";

import { PlusIcon, UploadIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Agent, MCPServer, MissingSkill, SkillCall, SkillItem } from "@/lib/types";

import { CreateSkillSheet } from "./_components/create-skill-sheet";
import { DeleteConfirmDialog } from "./_components/delete-confirm-dialog";
import { MissingSkillsPopover } from "./_components/missing-skills-popover";
import { SkillDetail } from "./_components/skill-detail";
import { SkillFileEditor } from "./_components/skill-file-editor";
import { SkillTree } from "./_components/skill-tree";
import { SkillsOverview } from "./_components/skills-overview";
import type { Creating, PendingDelete, Selected } from "./_components/types";

// ── Page ──────────────────────────────────────────────────────────────────
export default function SkillsPage() {
  const canOperate = useCan("operate"); // skill create/upload/delete are /api/skills writes (operate)
  const [skills, setSkills] = React.useState<SkillItem[]>([]);
  const [agents, setAgents] = React.useState<Agent[]>([]);
  const [selected, setSelected] = React.useState<Selected | null>(null);
  const [visibility, setVisibility] = React.useState<Record<string, string[]>>({});
  const [expanded, setExpanded] = React.useState<Set<string>>(new Set());

  // file editor
  const [fileContent, setFileContent] = React.useState("");
  const [fileLoading, setFileLoading] = React.useState(false);
  const [dirty, setDirty] = React.useState(false);
  const [saving, setSaving] = React.useState(false);

  // inline create — using a ref to avoid stale closure in onBlur
  const [creating, setCreating] = React.useState<Creating>(null);
  const [newEntryName, setNewEntryName] = React.useState("");
  const inlineRef = React.useRef<HTMLInputElement>(null);
  // cancelRef prevents onBlur from committing when Escape was pressed
  const cancelRef = React.useRef(false);

  // new skill dialog
  const [newOpen, setNewOpen] = React.useState(false);
  const [mcpOptions, setMcpOptions] = React.useState<MCPServer[]>([]);
  const [uploading, setUploading] = React.useState(false);
  const uploadRef = React.useRef<HTMLInputElement>(null);

  // skill detail — MCP edit state (optimistic, rolls back on error)
  const [detailMcps, setDetailMcps] = React.useState<string[]>([]);

  // delete confirmation
  const [pendingDelete, setPendingDelete] = React.useState<PendingDelete>(null);
  const [deleting, setDeleting] = React.useState(false);

  // Call stats: counts/last-used come back with api.skills(); selecting a skill fetches its
  // Call stats: counts/last-used come back with api.skills(); selecting a skill fetches its
  // recent call details. missing = skills invoked by name that do not exist (wanted but absent).
  const [usageCalls, setUsageCalls] = React.useState<SkillCall[]>([]);
  const [usageLoading, setUsageLoading] = React.useState(false);
  const [missing, setMissing] = React.useState<MissingSkill[]>([]);

  // ── Data ──────────────────────────────────────────────────────────────────
  const load = React.useCallback(() => {
    api
      .agents()
      .then(setAgents)
      .catch(() => {
        // best effort: the agent list is optional here
      });
    api
      .mcpServers()
      .then(setMcpOptions)
      .catch(() => {
        // best effort: MCP options are optional here
      });
    api
      .missingSkills()
      .then(setMissing)
      .catch(() => {
        // best effort: the missing-skills hint is optional
      });
    api
      .skills()
      .then((ss) => {
        setSkills(ss);
        ss.forEach((s) => {
          void api
            .skillVisibility(s.name)
            .then((ids) => setVisibility((v) => ({ ...v, [s.name]: ids })))
            .catch(() => {
              // best effort: visibility stays unset for this skill
            });
        });
      })
      .catch(() => {
        // best effort: keep the current skill list
      });
  }, []);

  React.useEffect(() => {
    load();
  }, [load]);

  // ── Upload a .zip skill ───────────────────────────────────────────────────
  async function uploadZip(file: File, overwrite = false) {
    setUploading(true);
    try {
      const r = await api.uploadSkill(file, overwrite);
      toast.success(t("Skill installed: {name} ({count} files)", { name: r.name, count: r.files }));
      load();
    } catch (e) {
      const msg = (e as Error).message;
      // offer overwrite when the skill already exists
      if (!overwrite && msg.includes("already exists")) {
        if (window.confirm(`${msg}\n\n${t("Overwrite the skill with the same name?")}`)) {
          await uploadZip(file, true);
          return;
        }
      } else {
        toast.error(t("Upload failed: {error}", { error: msg }));
      }
    } finally {
      setUploading(false);
    }
  }
  function onUploadPick(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0];
    e.target.value = ""; // reset so picking the same file again re-fires
    if (f) void uploadZip(f);
  }

  React.useEffect(() => {
    if (creating) {
      // Small timeout so the element is actually in the DOM before focusing
      setTimeout(() => inlineRef.current?.focus(), 20);
    }
  }, [creating]);

  // Sync detail-panel MCP state when the selected skill changes.
  React.useEffect(() => {
    if (!selected || selected.path !== null) return;
    const sk = skills.find((s) => s.name === selected.skill);
    setDetailMcps(sk?.mcps ?? []);
  }, [selected, skills]);

  // Recent calls for the selected skill (detail panel only).
  React.useEffect(() => {
    if (!selected || selected.path !== null) {
      setUsageCalls([]);
      return;
    }
    const name = selected.skill;
    setUsageLoading(true);
    api
      .skillUsage(name, 20)
      .then((calls) => setUsageCalls(calls))
      .catch(() => setUsageCalls([]))
      .finally(() => setUsageLoading(false));
  }, [selected]);

  React.useEffect(() => {
    if (!selected || selected.path === null) {
      setFileContent("");
      setDirty(false);
      return;
    }
    setFileLoading(true);
    api
      .readSkillFile(selected.skill, selected.path)
      .then((c) => {
        setFileContent(c);
        setDirty(false);
      })
      .catch(() => toast.error(t("Failed to read the file")))
      .finally(() => setFileLoading(false));
  }, [selected]);

  // ── Expand helpers ────────────────────────────────────────────────────────
  function toggleExpanded(key: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  }
  function ensureExpanded(skill: string, dirPath: string) {
    setExpanded((prev) => {
      const next = new Set(prev);
      next.add(skill);
      if (dirPath) {
        let cur = "";
        for (const part of dirPath.split("/")) {
          cur = cur ? `${cur}/${part}` : part;
          next.add(`${skill}:${cur}`);
        }
      }
      return next;
    });
  }

  // ── Inline create ─────────────────────────────────────────────────────────
  function startCreate(skill: string, inDir: string, kind: "file" | "dir") {
    cancelRef.current = false;
    ensureExpanded(skill, inDir);
    setCreating({ skill, inDir, kind });
    setNewEntryName("");
  }

  // Use a ref snapshot so commitCreate reads the latest creating value
  // without depending on potentially stale closure state.
  const creatingRef = React.useRef<Creating>(null);
  React.useEffect(() => {
    creatingRef.current = creating;
  }, [creating]);
  const newEntryRef = React.useRef("");
  React.useEffect(() => {
    newEntryRef.current = newEntryName;
  }, [newEntryName]);

  async function commitCreate() {
    if (cancelRef.current) {
      cancelRef.current = false;
      return;
    }
    const c = creatingRef.current;
    const name = newEntryRef.current.trim();
    setCreating(null);
    setNewEntryName("");
    if (!c || !name) return;

    const fullPath = c.inDir ? `${c.inDir}/${name}` : name;
    try {
      if (c.kind === "dir") {
        await api.createSkillDir(c.skill, fullPath);
        toast.success(t("Folder created: {path}", { path: fullPath }));
        ensureExpanded(c.skill, fullPath);
      } else {
        await api.writeSkillFile(c.skill, fullPath, "");
        toast.success(t("File created: {path}", { path: fullPath }));
        setSelected({ skill: c.skill, path: fullPath });
        ensureExpanded(c.skill, c.inDir);
      }
      load();
    } catch (e) {
      toast.error(t("Create failed: {error}", { error: (e as Error).message }));
    }
  }

  function cancelCreate() {
    cancelRef.current = true;
    setCreating(null);
    setNewEntryName("");
  }

  async function deletePath(skill: string, path: string) {
    try {
      await api.deleteSkillPath(skill, path);
      toast.success(t("Deleted: {path}", { path }));
      if (selected?.skill === skill && selected.path === path) setSelected(null);
      load();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
    }
  }

  async function deleteSkill(name: string) {
    try {
      await api.deleteSkill(name);
      toast.success(t("Skill deleted: {name}", { name }));
      if (selected?.skill === name) setSelected(null);
      load();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: (e as Error).message }));
    }
  }

  async function runPendingDelete() {
    const p = pendingDelete;
    if (!p) return;
    setDeleting(true);
    try {
      if (p.kind === "skill") await deleteSkill(p.skill);
      else await deletePath(p.skill, p.path);
    } finally {
      setDeleting(false);
      setPendingDelete(null);
    }
  }

  async function saveFile() {
    if (!selected || selected.path === null) return;
    setSaving(true);
    try {
      await api.writeSkillFile(selected.skill, selected.path, fileContent);
      toast.success(t("Saved"));
      setDirty(false);
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: (e as Error).message }));
    } finally {
      setSaving(false);
    }
  }

  async function toggleSkillMcp(skillName: string, mcpName: string, mcpOn: boolean) {
    const next = mcpOn ? [...detailMcps, mcpName] : detailMcps.filter((n) => n !== mcpName);
    setDetailMcps(next);
    try {
      await api.updateSkillMeta(skillName, { mcps: next });
      toast.success(mcpOn ? t('Linked "{name}"', { name: mcpName }) : t('Unlinked "{name}"', { name: mcpName }));
      load();
    } catch (e) {
      // roll back on error
      setDetailMcps(detailMcps);
      toast.error(t("Operation failed: {error}", { error: (e as Error).message }));
    }
  }

  async function toggleVisibility(skillName: string, agentId: string, agentName: string) {
    const on = (visibility[skillName] ?? []).includes(agentId);
    try {
      await api.toggleSkillVisibility(agentId, skillName, !on);
      toast.success(
        on
          ? t('Revoked visibility to "{name}"', { name: agentName })
          : t('Granted visibility to "{name}"', { name: agentName }),
      );
      const ids = await api.skillVisibility(skillName);
      setVisibility((v) => ({ ...v, [skillName]: ids }));
    } catch (e) {
      toast.error(t("Operation failed: {error}", { error: (e as Error).message }));
    }
  }

  const selectedSkill = selected ? (skills.find((s) => s.name === selected.skill) ?? null) : null;

  return (
    <div data-content-padding="false" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex items-center gap-3 border-b px-4 py-2.5 lg:px-6">
        <div className="flex flex-col gap-0.5">
          <h1 className="text-sm font-semibold leading-tight">{t("Skill")}</h1>
          <p className="text-muted-foreground text-xs">
            {t("Skill library · agentskills.io spec · visibility granted per agent")}
          </p>
        </div>
        {/* Gap list: skills agents invoke by name but missing from the library — direct evidence of what to add. */}
        {missing.length > 0 && <MissingSkillsPopover missing={missing} />}
      </div>
      <div className="flex flex-1 overflow-hidden">
        {/* ── left file tree ── */}
        <div className="flex w-64 shrink-0 flex-col border-r">
          {canOperate && (
            <div className="flex flex-col gap-2 border-b p-2">
              <Button size="sm" variant="outline" className="w-full" onClick={() => setNewOpen(true)}>
                <PlusIcon className="size-3.5" />
                {t("Create skill")}
              </Button>
              <input
                ref={uploadRef}
                type="file"
                accept=".zip,application/zip"
                className="hidden"
                onChange={onUploadPick}
              />
              <Button
                size="sm"
                variant="outline"
                className="w-full"
                disabled={uploading}
                onClick={() => uploadRef.current?.click()}
                title={t("Upload a .zip archive containing SKILL.md")}
              >
                <UploadIcon className="size-3.5" />
                {uploading ? t("Uploading…") : t("Upload archive")}
              </Button>
            </div>
          )}
          <SkillTree
            skills={skills}
            expanded={expanded}
            selected={selected}
            creating={creating}
            newEntryName={newEntryName}
            inlineRef={inlineRef}
            cancelRef={cancelRef}
            setNewEntryName={setNewEntryName}
            commitCreate={commitCreate}
            cancelCreate={cancelCreate}
            toggleExpanded={toggleExpanded}
            startCreate={startCreate}
            setSelected={setSelected}
            setPendingDelete={setPendingDelete}
          />
        </div>

        {/* ── right panel ── */}
        <div className="flex flex-1 flex-col overflow-auto p-4">
          {!selected && (
            <SkillsOverview
              skills={skills}
              missing={missing}
              onSelect={(name) => setSelected({ skill: name, path: null })}
            />
          )}

          {selected && selected.path === null && selectedSkill && (
            <SkillDetail
              selectedSkill={selectedSkill}
              agents={agents}
              mcpOptions={mcpOptions}
              detailMcps={detailMcps}
              visibility={visibility}
              usageCalls={usageCalls}
              usageLoading={usageLoading}
              onToggleMcp={toggleSkillMcp}
              onToggleVisibility={toggleVisibility}
            />
          )}

          {selected && selected.path !== null && (
            <SkillFileEditor
              skill={selected.skill}
              path={selected.path}
              content={fileContent}
              loading={fileLoading}
              dirty={dirty}
              saving={saving}
              onChange={(v) => {
                setFileContent(v);
                setDirty(true);
              }}
              onSave={saveFile}
            />
          )}
        </div>
      </div>

      <DeleteConfirmDialog
        pendingDelete={pendingDelete}
        deleting={deleting}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          void runPendingDelete();
        }}
      />

      <CreateSkillSheet
        open={newOpen}
        onOpenChange={setNewOpen}
        agents={agents}
        mcpOptions={mcpOptions}
        onCreated={load}
      />
    </div>
  );
}
