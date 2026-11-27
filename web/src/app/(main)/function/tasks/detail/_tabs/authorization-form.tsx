"use client";

import * as React from "react";

import { BanIcon, PencilIcon, PlusIcon } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { formatScopeText, fromLocalInput, parseScopeText, toLocalInput } from "@/lib/governance";
import { governanceApi } from "@/lib/governance-api";
import type { EngagementAuthorization } from "@/lib/governance-types";

// Admin-only editor of the authorization artifact. Saving never edits a version:
// it POSTs a new one, and the target gate enforces the newest.

interface Draft {
  reference: string;
  authorizedBy: string;
  scopeText: string;
  techniques: string[];
  notBefore: string; // datetime-local value
  notAfter: string;
  note: string;
}

const EMPTY_DRAFT: Draft = {
  reference: "",
  authorizedBy: "",
  scopeText: "",
  techniques: [],
  notBefore: "",
  notAfter: "",
  note: "",
};

function draftFrom(a: EngagementAuthorization | null): Draft {
  if (!a) return EMPTY_DRAFT;
  return {
    reference: a.reference,
    authorizedBy: a.authorized_by,
    scopeText: formatScopeText(a.scope),
    techniques: [...(a.techniques ?? [])],
    notBefore: toLocalInput(a.not_before),
    notAfter: toLocalInput(a.not_after),
    note: a.note,
  };
}

export function AuthorizationEditor({
  taskId,
  current,
  techniqueClasses,
  onChanged,
}: {
  taskId: string;
  current: EngagementAuthorization | null;
  techniqueClasses: readonly string[];
  onChanged: () => Promise<void>;
}) {
  const [open, setOpen] = React.useState(false);
  const [draft, setDraft] = React.useState<Draft>(EMPTY_DRAFT);
  const [saving, setSaving] = React.useState(false);
  const [revokeNote, setRevokeNote] = React.useState("");
  const [error, setError] = React.useState("");

  const parsed = React.useMemo(() => parseScopeText(draft.scopeText), [draft.scopeText]);
  const patch = (p: Partial<Draft>) => setDraft((d) => ({ ...d, ...p }));
  const canSave = !saving && draft.notBefore !== "" && draft.notAfter !== "";

  function startEdit() {
    setDraft(draftFrom(current));
    setError("");
    setOpen(true);
  }

  function toggleTechnique(cls: string, on: boolean) {
    setDraft((d) => ({
      ...d,
      techniques: on ? [...d.techniques.filter((x) => x !== cls), cls] : d.techniques.filter((x) => x !== cls),
    }));
  }

  async function save() {
    setSaving(true);
    setError("");
    try {
      await governanceApi.addAuthorization(taskId, {
        reference: draft.reference.trim(),
        authorized_by: draft.authorizedBy.trim(),
        scope: parsed,
        techniques: draft.techniques,
        not_before: fromLocalInput(draft.notBefore),
        not_after: fromLocalInput(draft.notAfter),
        note: draft.note.trim(),
      });
      setOpen(false);
      await onChanged();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setSaving(false);
    }
  }

  async function revoke() {
    setError("");
    try {
      await governanceApi.revokeAuthorization(taskId, revokeNote.trim());
      setRevokeNote("");
      setOpen(false);
      await onChanged();
    } catch (e) {
      setError(errorText(e));
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-muted-foreground text-sm">
        {t(
          "Versions are immutable: saving adds a new version and nothing is ever edited. The target gate enforces the newest version only.",
        )}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {!open ? (
          <Button variant="outline" size="sm" onClick={startEdit}>
            {current ? <PencilIcon /> : <PlusIcon />}
            {current ? t("Edit as new version") : t("Add authorization")}
          </Button>
        ) : null}
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button variant="destructive" size="sm" disabled={!current || current.revoked}>
              <BanIcon /> {t("Revoke")}
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("Revoke this authorization?")}</AlertDialogTitle>
              <AlertDialogDescription>
                {t(
                  "Adds a revoked version that authorizes nothing: the target gate refuses every tool that can reach a target until a new version is added.",
                )}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="auth-revoke-note">{t("Note (optional)")}</Label>
              <Textarea
                id="auth-revoke-note"
                rows={2}
                value={revokeNote}
                onChange={(e) => setRevokeNote(e.target.value)}
              />
            </div>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("Cancel")}</AlertDialogCancel>
              <AlertDialogAction variant="destructive" onClick={() => void revoke()}>
                {t("Revoke authorization")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>
      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : null}
      {open ? (
        <div className="grid gap-4 rounded-lg border p-4 sm:grid-cols-2">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auth-reference">{t("Reference")}</Label>
            <Input
              id="auth-reference"
              placeholder={t("e.g. SOW-2026-114")}
              value={draft.reference}
              onChange={(e) => patch({ reference: e.target.value })}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auth-by">{t("Authorized by")}</Label>
            <Input
              id="auth-by"
              placeholder={t("e.g. Jane Doe, CISO")}
              value={draft.authorizedBy}
              onChange={(e) => patch({ authorizedBy: e.target.value })}
            />
          </div>
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label htmlFor="auth-scope">{t("Scope entries")}</Label>
            <Textarea
              id="auth-scope"
              rows={5}
              className="font-mono text-sm"
              placeholder={t("One per line: kind:value (domain, host, ip, cidr) or a bare value")}
              value={draft.scopeText}
              onChange={(e) => patch({ scopeText: e.target.value })}
            />
            {parsed.length > 0 ? (
              <div className="flex flex-wrap items-center gap-1">
                <span className="text-muted-foreground text-xs">{t("Parsed as")}</span>
                {parsed.map((e) => (
                  <Badge key={`${e.kind}:${e.value}`} variant="outline">
                    {e.kind}: {e.value}
                  </Badge>
                ))}
              </div>
            ) : (
              <p className="text-muted-foreground text-xs">
                {t("Empty scope covers every target; the target gate still applies.")}
              </p>
            )}
          </div>
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label>{t("Technique classes")}</Label>
            <div className="flex flex-wrap gap-x-5 gap-y-2">
              {techniqueClasses.map((cls) => (
                <Label key={cls} className="flex items-center gap-2 font-normal">
                  <Checkbox
                    checked={draft.techniques.includes(cls)}
                    onCheckedChange={(v) => toggleTechnique(cls, v === true)}
                  />
                  {cls}
                </Label>
              ))}
            </div>
            <p className="text-muted-foreground text-xs">{t("None selected means every class is allowed.")}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auth-from">{t("Not before (local time)")}</Label>
            <Input
              id="auth-from"
              type="datetime-local"
              value={draft.notBefore}
              onChange={(e) => patch({ notBefore: e.target.value })}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="auth-to">{t("Not after (local time)")}</Label>
            <Input
              id="auth-to"
              type="datetime-local"
              value={draft.notAfter}
              onChange={(e) => patch({ notAfter: e.target.value })}
            />
          </div>
          <div className="flex flex-col gap-1.5 sm:col-span-2">
            <Label htmlFor="auth-note">{t("Note")}</Label>
            <Textarea id="auth-note" rows={2} value={draft.note} onChange={(e) => patch({ note: e.target.value })} />
          </div>
          <div className="flex gap-2 sm:col-span-2">
            <Button size="sm" disabled={!canSave} onClick={() => void save()}>
              {saving ? t("Saving…") : t("Save as new version")}
            </Button>
            <Button size="sm" variant="ghost" disabled={saving} onClick={() => setOpen(false)}>
              {t("Cancel")}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
