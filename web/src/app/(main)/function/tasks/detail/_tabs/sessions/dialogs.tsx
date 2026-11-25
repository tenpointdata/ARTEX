import * as React from "react";

import { Loader2Icon, Trash2Icon } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import type { Session } from "@/lib/types";
import { cn } from "@/lib/utils";

import type { DeleteMode } from "./use-worker-control";

// boldMarkup renders a translated sentence whose **marked** spans are bold, so the
// emphasis can move with Turkish word order instead of splitting the sentence.
function boldMarkup(text: string): React.ReactNode {
  return text.split(/\*\*(.+?)\*\*/g).map((part, index) =>
    // biome-ignore lint/suspicious/noArrayIndexKey: static split of a fixed string
    index % 2 === 1 ? <strong key={index}>{part}</strong> : <React.Fragment key={index}>{part}</React.Fragment>,
  );
}

export function DeleteWorkerDialog({
  cancelIntent,
  cancelReason,
  deleteMode,
  controllingIntent,
  onClose,
  setCancelReason,
  setDeleteMode,
  onConfirm,
}: {
  cancelIntent: Session | null;
  cancelReason: string;
  deleteMode: DeleteMode;
  controllingIntent: string | null;
  onClose: () => void;
  setCancelReason: (value: string) => void;
  setDeleteMode: (mode: DeleteMode) => void;
  onConfirm: (session: Session) => void;
}) {
  return (
    <AlertDialog
      open={cancelIntent !== null}
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <AlertDialogContent className="max-w-[min(32rem,calc(100vw-2rem))]">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Delete Worker #{id}?", { id: cancelIntent?.intent_id ?? "" })}</AlertDialogTitle>
          <AlertDialogDescription className="break-words whitespace-normal">
            {deleteMode === "hard"
              ? boldMarkup(
                  t(
                    "**Hard delete** physically removes this intent and the downstream nodes **supported solely by it** (cascading to the leaves to avoid orphaned data); shared nodes, goals, and the task's root facts are kept. **This action is irreversible.** The planner is notified of the deletion and re-plans accordingly.",
                  ),
                )
              : boldMarkup(
                  t(
                    '**Soft delete** marks the intent as "deleted" and records the reason; the intent node, execution records, registered facts, and findings **are all kept**. The planner receives "intent deleted by user + reason" and re-plans accordingly.',
                  ),
                )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="grid gap-3 py-1">
          <div className="grid grid-cols-2 gap-2">
            <button
              type="button"
              onClick={() => setDeleteMode("soft")}
              className={cn(
                "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                deleteMode === "soft" ? "border-primary bg-primary/5" : "hover:bg-accent",
              )}
            >
              <div className="font-medium">{t("Soft delete")}</div>
              <div className="text-xs text-muted-foreground">{t("Data kept, traceable")}</div>
            </button>
            <button
              type="button"
              onClick={() => setDeleteMode("hard")}
              className={cn(
                "rounded-md border px-3 py-2 text-left text-sm transition-colors",
                deleteMode === "hard" ? "border-destructive bg-destructive/5" : "hover:bg-accent",
              )}
            >
              <div className="font-medium">{t("Hard delete")}</div>
              <div className="text-xs text-muted-foreground">{t("Cascade removal, irreversible")}</div>
            </button>
          </div>
          <div className="grid gap-2">
            <label htmlFor="cancel-reason" className="text-sm font-medium">
              {t("Deletion reason (required)")}
            </label>
            <Textarea
              id="cancel-reason"
              value={cancelReason}
              onChange={(e) => setCancelReason(e.target.value)}
              placeholder={t(
                "Explain why this intent is being deleted, e.g. wrong direction / goal obsolete / duplicate of another intent…",
              )}
              rows={3}
              autoFocus
            />
          </div>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>{t("Back")}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={!cancelIntent || controllingIntent !== null || !cancelReason.trim()}
            onClick={() => cancelIntent && onConfirm(cancelIntent)}
          >
            {controllingIntent ? <Loader2Icon className="animate-spin" /> : <Trash2Icon />}
            {deleteMode === "hard" ? t("Delete permanently") : t("Confirm delete")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function NewMainSessionDialog({
  open,
  creating,
  onClose,
  onConfirm,
}: {
  open: boolean;
  creating: boolean;
  onClose: () => void;
  onConfirm: () => void;
}) {
  return (
    <AlertDialog open={open} onOpenChange={(next) => !next && onClose()}>
      <AlertDialogContent className="max-w-[min(32rem,calc(100vw-2rem))]">
        <AlertDialogHeader>
          <AlertDialogTitle>{t("Start a new session?")}</AlertDialogTitle>
          <AlertDialogDescription className="break-words whitespace-normal">
            {t(
              "The current session will be archived (you can switch back anytime) and the main agent continues with a clean context. The task's graph, assets, and goals are unaffected.",
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={creating}>{t("Cancel")}</AlertDialogCancel>
          <AlertDialogAction disabled={creating} onClick={onConfirm}>
            {creating ? t("Starting…") : t("Start new session")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
