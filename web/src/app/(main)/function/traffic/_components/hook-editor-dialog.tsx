"use client";

import { AlertTriangleIcon, Code2Icon, Loader2Icon, PlayIcon } from "lucide-react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { DEFAULT_REQUEST_HOOK_SOURCE, DEFAULT_RESPONSE_HOOK_SOURCE } from "@/lib/tamper-hooks";
import type { TamperHookPhase } from "@/lib/types";

import type { EditorState } from "./hook-editor-state";

interface HookEditorDialogProps {
  editor: EditorState | null;
  onEditorChange: (editor: EditorState | null) => void;
  formError: string | null;
  saving: boolean;
  onSave: () => void;
  onDryRunDraft: () => void;
  onClose: () => void;
}

// Create / Edit dialog for a tampering hook.
export function HookEditorDialog({
  editor,
  onEditorChange: setEditor,
  formError,
  saving,
  onSave,
  onDryRunDraft: openDryRunForDraft,
  onClose,
}: HookEditorDialogProps) {
  return (
    <Dialog open={editor !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{editor?.mode === "edit" ? t("Edit tampering hook") : t("New tampering hook")}</DialogTitle>
          <DialogDescription>
            {editor?.mode === "create"
              ? t("New hooks are created disabled. Save and dry-run before enabling.")
              : t("Saving creates a new version in history. Source and matchers are sealed at rest.")}
          </DialogDescription>
        </DialogHeader>

        {editor && (
          <div className="space-y-4 py-2">
            {formError && (
              <Alert variant="destructive">
                <AlertTriangleIcon className="size-4" />
                <AlertDescription className="font-mono text-xs whitespace-pre-wrap">{formError}</AlertDescription>
              </Alert>
            )}

            <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div className="space-y-1.5 sm:col-span-1">
                <Label htmlFor="hook-name">{t("Name")}</Label>
                <Input
                  id="hook-name"
                  value={editor.name}
                  placeholder="sign-api-requests"
                  onChange={(e) => setEditor({ ...editor, name: e.target.value })}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="hook-phase">{t("Phase")}</Label>
                <Select
                  value={editor.phase}
                  onValueChange={(val) => {
                    const nextPhase = val as TamperHookPhase;
                    const shouldSwapTemplate =
                      editor.mode === "create" &&
                      (editor.source.trim() === DEFAULT_REQUEST_HOOK_SOURCE.trim() ||
                        editor.source.trim() === DEFAULT_RESPONSE_HOOK_SOURCE.trim());
                    setEditor({
                      ...editor,
                      phase: nextPhase,
                      source: shouldSwapTemplate
                        ? nextPhase === "response"
                          ? DEFAULT_RESPONSE_HOOK_SOURCE
                          : DEFAULT_REQUEST_HOOK_SOURCE
                        : editor.source,
                    });
                  }}
                >
                  <SelectTrigger id="hook-phase">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="request">request</SelectItem>
                    <SelectItem value="response">response</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="hook-priority">{t("Priority")}</Label>
                <Input
                  id="hook-priority"
                  type="number"
                  value={editor.priority}
                  onChange={(e) => setEditor({ ...editor, priority: e.target.value })}
                />
              </div>
            </div>

            <div className="space-y-1.5">
              <Label htmlFor="hook-note">{t("Note")}</Label>
              <Input
                id="hook-note"
                value={editor.note}
                placeholder={t("Optional operator note")}
                onChange={(e) => setEditor({ ...editor, note: e.target.value })}
              />
            </div>

            {/* Matchers */}
            <div className="space-y-3 rounded-md border p-3">
              <div className="text-xs font-medium text-muted-foreground">
                {t("Matchers (prefilter — empty matches all)")}
              </div>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1">
                  <Label htmlFor="m-methods" className="text-xs">
                    {t("Methods (comma-separated)")}
                  </Label>
                  <Input
                    id="m-methods"
                    className="h-8 font-mono text-xs"
                    value={editor.methods}
                    placeholder="POST, PUT"
                    onChange={(e) => setEditor({ ...editor, methods: e.target.value })}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="m-host" className="text-xs">
                    {t("Host (exact or *.suffix)")}
                  </Label>
                  <Input
                    id="m-host"
                    className="h-8 font-mono text-xs"
                    value={editor.host}
                    placeholder="*.example.test"
                    onChange={(e) => setEditor({ ...editor, host: e.target.value })}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="m-url" className="text-xs">
                    {t("URL regex")}
                  </Label>
                  <Input
                    id="m-url"
                    className="h-8 font-mono text-xs"
                    value={editor.urlRegex}
                    placeholder="^/api/v1/"
                    onChange={(e) => setEditor({ ...editor, urlRegex: e.target.value })}
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="m-body" className="text-xs">
                    {t("Request body regex")}
                  </Label>
                  <Input
                    id="m-body"
                    className="h-8 font-mono text-xs"
                    value={editor.bodyRegex}
                    placeholder="token"
                    onChange={(e) => setEditor({ ...editor, bodyRegex: e.target.value })}
                  />
                </div>
              </div>
              <div className="space-y-1">
                <Label htmlFor="m-headers" className="text-xs">
                  {t("Request headers (one per line: Name or Name: value_regex)")}
                </Label>
                <Textarea
                  id="m-headers"
                  rows={2}
                  className="font-mono text-xs"
                  value={editor.headers}
                  placeholder={"Content-Type: application/json\nX-Api-Key"}
                  onChange={(e) => setEditor({ ...editor, headers: e.target.value })}
                />
              </div>

              {editor.phase === "response" && (
                <div className="grid grid-cols-1 gap-3 border-t pt-3 sm:grid-cols-2">
                  <div className="space-y-1">
                    <Label htmlFor="m-status" className="text-xs">
                      {t("Status codes (comma-separated, e.g. 200, 4xx)")}
                    </Label>
                    <Input
                      id="m-status"
                      className="h-8 font-mono text-xs"
                      value={editor.status}
                      placeholder="200, 2xx"
                      onChange={(e) => setEditor({ ...editor, status: e.target.value })}
                    />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="m-resp-body" className="text-xs">
                      {t("Response body regex")}
                    </Label>
                    <Input
                      id="m-resp-body"
                      className="h-8 font-mono text-xs"
                      value={editor.respBodyRegex}
                      onChange={(e) => setEditor({ ...editor, respBodyRegex: e.target.value })}
                    />
                  </div>
                  <div className="space-y-1 sm:col-span-2">
                    <Label htmlFor="m-resp-headers" className="text-xs">
                      {t("Response headers (one per line: Name or Name: value_regex)")}
                    </Label>
                    <Textarea
                      id="m-resp-headers"
                      rows={2}
                      className="font-mono text-xs"
                      value={editor.respHeaders}
                      placeholder="Content-Type: application/json"
                      onChange={(e) => setEditor({ ...editor, respHeaders: e.target.value })}
                    />
                  </div>
                </div>
              )}
            </div>

            {/* Source editor */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <Label htmlFor="hook-source" className="flex items-center gap-1.5">
                  <Code2Icon className="size-3.5" />
                  {t("Go-subset source")}
                </Label>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="h-6 px-2 text-xs"
                  onClick={() =>
                    setEditor({
                      ...editor,
                      source: editor.phase === "response" ? DEFAULT_RESPONSE_HOOK_SOURCE : DEFAULT_REQUEST_HOOK_SOURCE,
                    })
                  }
                >
                  {t("Reset template")}
                </Button>
              </div>
              <Textarea
                id="hook-source"
                rows={12}
                spellCheck={false}
                className="font-mono text-xs leading-relaxed"
                value={editor.source}
                onChange={(e) => setEditor({ ...editor, source: e.target.value })}
              />
              <p className="text-xs text-muted-foreground">
                {editor.phase === "response"
                  ? t("Entrypoint: package hook · func OnResponse(req *Request, resp *Response) error")
                  : t("Entrypoint: package hook · func OnRequest(req *Request) error")}
              </p>
            </div>
          </div>
        )}

        <DialogFooter className="gap-2 sm:justify-between">
          <Button type="button" variant="outline" size="sm" onClick={openDryRunForDraft} disabled={!editor}>
            <PlayIcon className="size-3.5" />
            {t("Dry-run draft")}
          </Button>
          <div className="flex items-center gap-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={saving}>
              {t("Cancel")}
            </Button>
            <Button type="button" onClick={() => onSave()} disabled={saving}>
              {saving && <Loader2Icon className="size-3.5 animate-spin" />}
              {editor?.mode === "edit" ? t("Save changes") : t("Create hook")}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
