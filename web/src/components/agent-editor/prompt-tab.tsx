"use client";

import * as React from "react";

import { EyeIcon, GitCompareIcon, RotateCcwIcon, SaveIcon } from "lucide-react";
import { toast } from "sonner";

import { Markdown } from "@/components/markdown";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { intlLocale, t } from "@/i18n";
import type { AgentDetail, PromptVar, PromptVersion } from "@/lib/types";

import { DiffView } from "./diff-view";

export type PromptTabProps = {
  showLLM: boolean;
  showConfig: boolean;
  showWebSearch: boolean;
  showInteractiveShell: boolean;
  webSearchGlobalOn: boolean;
  llmProfileId: string;
  setLlmProfileId: (v: string) => void;
  llmProfiles: NonNullable<AgentDetail["llm_profiles"]>;
  maxTurns: string;
  setMaxTurns: (v: string) => void;
  runSecs: string;
  setRunSecs: (v: string) => void;
  webSearch: boolean;
  setWebSearch: (v: boolean) => void;
  interactiveShell: boolean;
  setInteractiveShell: (v: boolean) => void;
  saveConfig: () => void;
  variables: PromptVar[];
  versions: PromptVersion[];
  prompt: string;
  setPrompt: React.Dispatch<React.SetStateAction<string>>;
  preview: string;
  doPreview: () => void;
  savePrompt: () => void;
  resetPrompt: () => void;
};

// Config & Prompt tab: LLM binding, run budget, feature switches, prompt editor, version history.
export function PromptTab(p: PromptTabProps) {
  const {
    showLLM,
    showConfig,
    showWebSearch,
    showInteractiveShell,
    webSearchGlobalOn,
    llmProfileId,
    setLlmProfileId,
    llmProfiles,
    maxTurns,
    setMaxTurns,
    runSecs,
    setRunSecs,
    webSearch,
    setWebSearch,
    interactiveShell,
    setInteractiveShell,
    saveConfig,
    variables,
    versions,
    prompt,
    setPrompt,
    preview,
    doPreview,
    savePrompt,
    resetPrompt,
  } = p;
  const [viewVer, setViewVer] = React.useState<PromptVersion | null>(null);
  const [diffVer, setDiffVer] = React.useState<PromptVersion | null>(null);
  return (
    <div className="grid gap-4">
      {(showLLM || showConfig || showWebSearch || showInteractiveShell) && (
        <div className="grid gap-3 rounded-md border p-3">
          {showLLM && (
            <div className="grid gap-1.5">
              <Label htmlFor="llm-profile" className="text-xs">
                {t("Default model (LLM profile)")}
              </Label>
              <div className="flex flex-wrap items-center gap-3">
                <Select
                  value={llmProfileId || "__follow__"}
                  onValueChange={(v) => setLlmProfileId(v === "__follow__" ? "" : v)}
                >
                  <SelectTrigger id="llm-profile" className="h-8 w-72">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="__follow__">{t("Follow task / globally active profile")}</SelectItem>
                    {llmProfiles.map((p) => (
                      <SelectItem key={p.id} value={String(p.id)}>
                        {p.is_default
                          ? t("{name} ({model}) · default", { name: p.name, model: p.model })
                          : `${p.name} (${p.model})`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <span className="text-muted-foreground max-w-md text-xs">
                  {t(
                    'Bind a fixed LLM profile to this Agent (click "Save config" to apply). Priority: Agent binding > task/session override > global active.',
                  )}
                </span>
              </div>
            </div>
          )}
          <div className="flex flex-wrap items-end gap-3">
            {showConfig && (
              <>
                <div className="grid gap-1.5">
                  <Label htmlFor="max-turns" className="text-xs">
                    {t("Max loop turns (0 = unlimited)")}
                  </Label>
                  <Input
                    id="max-turns"
                    type="number"
                    min={0}
                    className="h-8 w-32"
                    value={maxTurns}
                    onChange={(e) => setMaxTurns(e.target.value)}
                  />
                </div>
                <div className="grid gap-1.5">
                  <Label htmlFor="run-seconds" className="text-xs">
                    {t("Run duration (seconds, 0 = unlimited)")}
                  </Label>
                  <Input
                    id="run-seconds"
                    type="number"
                    min={0}
                    className="h-8 w-32"
                    value={runSecs}
                    onChange={(e) => setRunSecs(e.target.value)}
                  />
                </div>
              </>
            )}
            <Button size="sm" variant="outline" onClick={saveConfig}>
              <SaveIcon /> {t("Save config")}
            </Button>
          </div>
          {showWebSearch && (
            <div className="flex items-center gap-3 border-t pt-3">
              <Switch
                id="web-search"
                checked={webSearch}
                disabled={!webSearchGlobalOn}
                onCheckedChange={setWebSearch}
              />
              <div className="grid gap-0.5">
                <Label htmlFor="web-search" className="text-sm">
                  {t("Web search")}
                </Label>
                <span className="text-muted-foreground text-xs">
                  {webSearchGlobalOn
                    ? t("Once enabled for this Agent (click Save above to apply), web_search can search the internet")
                    : t("Enable web search in System settings and configure a backend before enabling it here")}
                </span>
              </div>
            </div>
          )}
          {showInteractiveShell && (
            <div className="flex items-center gap-3 border-t pt-3">
              <Switch id="interactive-shell" checked={interactiveShell} onCheckedChange={setInteractiveShell} />
              <div className="grid gap-0.5">
                <Label htmlFor="interactive-shell" className="text-sm">
                  {t("Interactive shell")}
                </Label>
                <span className="text-muted-foreground text-xs">
                  {t(
                    "Once enabled for this Agent (click Save above to apply), persistent PTY session tools (shell_open/send/read/close/list) can drive interactive programs such as msfconsole/ssh/REPL",
                  )}
                </span>
              </div>
            </div>
          )}
        </div>
      )}

      <div className="grid gap-2">
        <Label className="text-muted-foreground text-xs">
          {t("Variables (click to insert a placeholder; replaced with runtime data when rendered)")}
        </Label>
        <div className="flex flex-wrap gap-2">
          {variables.map((v) => (
            <Tooltip key={v.name}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  onClick={() => setPrompt((p) => `${p}{{.${v.name}}}`)}
                  className="hover:bg-muted inline-flex items-center gap-1 rounded-md border bg-muted/40 px-2 py-1 font-mono text-xs"
                >
                  {`{{.${v.name}}}`}
                  <Badge variant="secondary" className="px-1 py-0 text-[10px]">
                    {t(v.source)}
                  </Badge>
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-xs">
                <p className="font-medium">{v.description}</p>
                <p className="text-muted-foreground mt-1">{t("Example: {example}", { example: v.example })}</p>
              </TooltipContent>
            </Tooltip>
          ))}
          {variables.length === 0 && <span className="text-muted-foreground text-xs">{t("(no variables)")}</span>}
        </div>
      </div>

      <Textarea
        className="font-mono text-xs"
        rows={16}
        value={prompt}
        placeholder={t("Leave empty to use the built-in default prompt")}
        onChange={(e) => setPrompt(e.target.value)}
      />

      <div className="flex flex-wrap gap-2">
        <Dialog>
          <DialogTrigger asChild>
            <Button variant="outline" size="sm" onClick={doPreview}>
              <EyeIcon /> {t("Render preview")}
            </Button>
          </DialogTrigger>
          <DialogContent className="sm:max-w-2xl">
            <DialogHeader>
              <DialogTitle>{t("Render preview")}</DialogTitle>
              <DialogDescription>
                {t("All {var} have been replaced by the backend with sample values.", { var: "{{.Var}}" })}
              </DialogDescription>
            </DialogHeader>
            <div className="max-h-[60vh] overflow-auto rounded-md border bg-muted/30 p-3">
              <Markdown text={preview} />
            </div>
          </DialogContent>
        </Dialog>
        <Button size="sm" onClick={savePrompt}>
          <SaveIcon /> {t("Save as new version")}
        </Button>
        <Button variant="outline" size="sm" onClick={resetPrompt}>
          <RotateCcwIcon /> {t("Restore default")}
        </Button>
      </div>

      <Separator />
      <div className="grid gap-2">
        <Label className="text-muted-foreground text-xs">{t("Version history")}</Label>
        <ul className="grid gap-1">
          {versions.map((ver, i) => (
            <li key={ver.version} className="flex items-center gap-2 rounded-md px-1 py-0.5 text-xs hover:bg-muted/50">
              <span className="font-mono shrink-0">v{ver.version}</span>
              {i === 0 && (
                <Badge variant="secondary" className="px-1.5 py-0 shrink-0">
                  {t("Current")}
                </Badge>
              )}
              <span className="text-muted-foreground truncate flex-1">{ver.note}</span>
              {ver.ts && (
                <span className="text-muted-foreground/60 shrink-0 tabular-nums">
                  {new Date(ver.ts).toLocaleDateString(intlLocale, {
                    month: "2-digit",
                    day: "2-digit",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </span>
              )}
              <Button
                variant="ghost"
                size="icon-sm"
                className="size-6 shrink-0"
                onClick={() => setViewVer(ver)}
                aria-label={t("View version")}
                title={t("View version")}
              >
                <EyeIcon className="size-3" />
              </Button>
              {i > 0 && versions[0] && (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="size-6 shrink-0"
                  onClick={() => setDiffVer(ver)}
                  aria-label={t("Compare with current")}
                  title={t("Compare with current")}
                >
                  <GitCompareIcon className="size-3" />
                </Button>
              )}
            </li>
          ))}
          {versions.length === 0 && (
            <li className="text-muted-foreground text-xs">{t("(no saved versions; using the built-in default)")}</li>
          )}
        </ul>
      </div>

      {/* view-version dialog */}
      <Dialog
        open={!!viewVer}
        onOpenChange={(o) => {
          if (!o) setViewVer(null);
        }}
      >
        <DialogContent className="sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>
              v{viewVer?.version}
              {viewVer?.version === versions[0]?.version && (
                <Badge variant="secondary" className="ml-2 px-1.5 py-0 align-middle">
                  {t("Current")}
                </Badge>
              )}
            </DialogTitle>
            <DialogDescription>
              {viewVer?.note || t("(no note)")}
              {viewVer?.ts && (
                <span className="ml-2 text-muted-foreground/60">{new Date(viewVer.ts).toLocaleString(intlLocale)}</span>
              )}
            </DialogDescription>
          </DialogHeader>
          <pre className="bg-muted max-h-[55vh] overflow-auto whitespace-pre-wrap rounded-md p-3 font-mono text-xs">
            {viewVer?.template_text || t("(empty)")}
          </pre>
          <div className="flex gap-2 justify-end">
            {viewVer && viewVer.version !== versions[0]?.version && (
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  if (viewVer) {
                    setDiffVer(viewVer);
                    setViewVer(null);
                  }
                }}
              >
                <GitCompareIcon className="mr-1 size-3.5" /> {t("Compare with current")}
              </Button>
            )}
            <Button
              size="sm"
              onClick={() => {
                if (viewVer) {
                  setPrompt(viewVer.template_text);
                  setViewVer(null);
                  toast.success(
                    t('Loaded v{version} into the editor; click "Save as new version" to confirm', {
                      version: viewVer.version,
                    }),
                  );
                }
              }}
            >
              {t("Load into editor")}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* version-diff dialog */}
      <Dialog
        open={!!diffVer}
        onOpenChange={(o) => {
          if (!o) setDiffVer(null);
        }}
      >
        <DialogContent className="sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              {t("Version diff: v{from} → v{to} (current)", {
                from: diffVer?.version ?? "",
                to: versions[0]?.version ?? "",
              })}
            </DialogTitle>
            <DialogDescription>
              <span className="inline-flex items-center gap-3 text-xs">
                <span className="rounded bg-red-500/15 px-1.5 py-0.5 text-red-600 dark:text-red-400">
                  {t("- removed")}
                </span>
                <span className="rounded bg-green-500/15 px-1.5 py-0.5 text-green-600 dark:text-green-400">
                  {t("+ added")}
                </span>
              </span>
            </DialogDescription>
          </DialogHeader>
          <DiffView oldText={diffVer?.template_text ?? ""} newText={versions[0]?.template_text ?? ""} />
        </DialogContent>
      </Dialog>
    </div>
  );
}
