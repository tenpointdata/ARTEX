"use client";

import type * as React from "react";

import {
  ChevronRightIcon,
  FilePlusIcon,
  FileTextIcon,
  FolderIcon,
  FolderOpenIcon,
  FolderPlusIcon,
  Trash2Icon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { SkillItem } from "@/lib/types";
import { cn } from "@/lib/utils";

import { buildTree, fmtTime } from "./tree";
import type { Creating, PendingDelete, Selected, TreeNode } from "./types";

export function SkillTree({
  skills,
  expanded,
  selected,
  creating,
  newEntryName,
  inlineRef,
  cancelRef,
  setNewEntryName,
  commitCreate,
  cancelCreate,
  toggleExpanded,
  startCreate,
  setSelected,
  setPendingDelete,
}: {
  skills: SkillItem[];
  expanded: Set<string>;
  selected: Selected | null;
  creating: Creating;
  newEntryName: string;
  inlineRef: React.RefObject<HTMLInputElement | null>;
  cancelRef: React.MutableRefObject<boolean>;
  setNewEntryName: (v: string) => void;
  commitCreate: () => Promise<void>;
  cancelCreate: () => void;
  toggleExpanded: (key: string) => void;
  startCreate: (skill: string, inDir: string, kind: "file" | "dir") => void;
  setSelected: (s: Selected) => void;
  setPendingDelete: (p: PendingDelete) => void;
}) {
  const canOperate = useCan("operate"); // /api/skills writes are operate (server/rbac_policy.go)

  // ── Inline input JSX helper (NOT a React component — avoids remount on re-render) ──
  // Defined as a plain function returning JSX so React never sees a new component type.
  function inlineInputJSX(indent: number) {
    return (
      <div key="__inline_create__" className="flex items-center gap-1 py-0.5 pr-2" style={{ paddingLeft: indent }}>
        {creating?.kind === "dir" ? (
          <FolderIcon className="size-3.5 shrink-0 text-amber-500" />
        ) : (
          <FileTextIcon className="size-3.5 shrink-0 text-muted-foreground" />
        )}
        <Input
          ref={inlineRef}
          className="h-6 flex-1 px-1 py-0 font-mono text-xs"
          placeholder={creating?.kind === "dir" ? "folder-name" : "filename.py"}
          value={newEntryName}
          onChange={(e) => setNewEntryName(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              cancelRef.current = false;
              void commitCreate();
            }
            if (e.key === "Escape") cancelCreate();
          }}
          onBlur={() => {
            void commitCreate();
          }}
        />
      </div>
    );
  }

  function selectSkillRoot(name: string) {
    toggleExpanded(name);
    setSelected({ skill: name, path: null });
  }

  // Row keyboard activation; ignores keys that bubble up from the row's inline action buttons.
  const onActivateKey = (activate: () => void) => (e: React.KeyboardEvent) => {
    if (e.target !== e.currentTarget) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      activate();
    }
  };

  // ── Recursive tree renderer ───────────────────────────────────────────────
  function renderTree(nodes: TreeNode[], skill: string, depth: number): React.ReactNode {
    // depth+1: the skill root sits at 8px (px-2); its children indent one step
    // further in so the tree reads as nested under the skill folder.
    const baseIndent = 8 + (depth + 1) * 14;
    return nodes.map((node) => {
      if (node.type === "dir") {
        const key = `${skill}:${node.path}`;
        const open = expanded.has(key);
        return (
          <div key={node.path}>
            <div
              className="group relative flex cursor-pointer select-none items-center gap-1 rounded py-0.5 pr-1 text-sm hover:bg-muted"
              style={{ paddingLeft: baseIndent }}
              role="treeitem"
              aria-selected={false}
              aria-expanded={open}
              tabIndex={0}
              onClick={() => toggleExpanded(key)}
              onKeyDown={onActivateKey(() => toggleExpanded(key))}
            >
              <ChevronRightIcon className={cn("size-3.5 shrink-0 transition-transform", open && "rotate-90")} />
              {open ? (
                <FolderOpenIcon className="size-3.5 shrink-0 text-amber-500" />
              ) : (
                <FolderIcon className="size-3.5 shrink-0 text-amber-500" />
              )}
              <span className="min-w-0 flex-1 truncate" title={node.path}>
                {node.name}
              </span>
              {/* Absolute so a long name can never push the actions out of view */}
              {canOperate && (
                <span className="absolute inset-y-0 right-1 hidden items-center gap-0.5 rounded bg-muted pl-1 group-hover:flex">
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-5"
                    title={t("New file")}
                    onClick={(e) => {
                      e.stopPropagation();
                      startCreate(skill, node.path, "file");
                    }}
                  >
                    <FilePlusIcon className="size-3 text-muted-foreground" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-5"
                    title={t("New folder")}
                    onClick={(e) => {
                      e.stopPropagation();
                      startCreate(skill, node.path, "dir");
                    }}
                  >
                    <FolderPlusIcon className="size-3 text-muted-foreground" />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    className="size-5"
                    title={t("Delete folder")}
                    onClick={(e) => {
                      e.stopPropagation();
                      setPendingDelete({ kind: "dir", skill, path: node.path });
                    }}
                  >
                    <Trash2Icon className="size-3 text-destructive" />
                  </Button>
                </span>
              )}
            </div>
            {open && (
              <>
                {renderTree(node.children, skill, depth + 1)}
                {creating?.skill === skill && creating.inDir === node.path && inlineInputJSX(baseIndent + 14)}
              </>
            )}
          </div>
        );
      }

      // file node
      const isSelected = selected?.skill === skill && selected.path === node.path;
      return (
        <div
          key={node.path}
          className={cn(
            "group relative flex cursor-pointer select-none items-center gap-1 rounded py-0.5 pr-1 text-sm",
            isSelected ? "bg-accent text-accent-foreground" : "hover:bg-muted",
          )}
          style={{ paddingLeft: baseIndent + 16 }}
          role="treeitem"
          aria-selected={isSelected}
          tabIndex={0}
          onClick={() => setSelected({ skill, path: node.path })}
          onKeyDown={onActivateKey(() => setSelected({ skill, path: node.path }))}
        >
          <FileTextIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate font-mono text-xs" title={node.path}>
            {node.name}
          </span>
          {canOperate && (
            <span
              className={cn(
                "absolute inset-y-0 right-1 hidden items-center rounded pl-1 group-hover:flex",
                isSelected ? "bg-accent" : "bg-muted",
              )}
            >
              <Button
                size="icon"
                variant="ghost"
                className="size-5"
                title={t("Delete file")}
                onClick={(e) => {
                  e.stopPropagation();
                  setPendingDelete({ kind: "file", skill, path: node.path });
                }}
              >
                <Trash2Icon className="size-3 text-destructive" />
              </Button>
            </span>
          )}
        </div>
      );
    });
  }

  return (
    <ScrollArea className="flex-1 [&>[data-slot=scroll-area-viewport]>div]:!block">
      <div className="p-1" role="tree">
        {skills.map((s) => {
          const isOpen = expanded.has(s.name);
          const tree = buildTree(s.files);
          const isSkillSelected = selected?.skill === s.name && selected.path === null;
          return (
            <div key={s.name}>
              {/* skill root node */}
              <div
                className={cn(
                  "group relative flex cursor-pointer select-none items-center gap-1 rounded px-2 py-1 text-sm",
                  isSkillSelected ? "bg-accent text-accent-foreground" : "hover:bg-muted",
                )}
                role="treeitem"
                aria-selected={isSkillSelected}
                aria-expanded={isOpen}
                tabIndex={0}
                onClick={() => selectSkillRoot(s.name)}
                onKeyDown={onActivateKey(() => selectSkillRoot(s.name))}
              >
                <ChevronRightIcon className={cn("size-3.5 shrink-0 transition-transform", isOpen && "rotate-90")} />
                {isOpen ? (
                  <FolderOpenIcon className="size-3.5 shrink-0 text-blue-500" />
                ) : (
                  <FolderIcon className="size-3.5 shrink-0 text-blue-500" />
                )}
                <span className="min-w-0 flex-1 truncate font-semibold" title={s.name}>
                  {s.name}
                </span>
                {s.calls > 0 && (
                  <span
                    className="shrink-0 rounded bg-muted px-1 text-[10px] tabular-nums text-muted-foreground"
                    title={t("Called {count} times · last {time}", { count: s.calls, time: fmtTime(s.last_used) })}
                  >
                    {s.calls}
                  </span>
                )}
                {canOperate && (
                  <span
                    className={cn(
                      "absolute inset-y-0 right-1 hidden items-center gap-0.5 rounded pl-1 group-hover:flex",
                      isSkillSelected ? "bg-accent" : "bg-muted",
                    )}
                  >
                    <Button
                      size="icon"
                      variant="ghost"
                      className="size-5"
                      title={t("New file")}
                      onClick={(e) => {
                        e.stopPropagation();
                        startCreate(s.name, "", "file");
                      }}
                    >
                      <FilePlusIcon className="size-3 text-muted-foreground" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="size-5"
                      title={t("New folder")}
                      onClick={(e) => {
                        e.stopPropagation();
                        startCreate(s.name, "", "dir");
                      }}
                    >
                      <FolderPlusIcon className="size-3 text-muted-foreground" />
                    </Button>
                    <Button
                      size="icon"
                      variant="ghost"
                      className="size-5"
                      title={t("Delete skill")}
                      onClick={(e) => {
                        e.stopPropagation();
                        setPendingDelete({ kind: "skill", skill: s.name });
                      }}
                    >
                      <Trash2Icon className="size-3 text-destructive" />
                    </Button>
                  </span>
                )}
              </div>

              {/* expanded: recursive file tree */}
              {isOpen && (
                <>
                  {renderTree(tree, s.name, 0)}
                  {creating?.skill === s.name && creating.inDir === "" && inlineInputJSX(22)}
                </>
              )}
            </div>
          );
        })}

        {skills.length === 0 && (
          <p className="p-3 text-xs text-muted-foreground">{t('No skills yet — click "Create" to get started')}</p>
        )}
      </div>
    </ScrollArea>
  );
}
