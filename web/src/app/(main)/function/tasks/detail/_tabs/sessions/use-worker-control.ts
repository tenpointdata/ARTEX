import * as React from "react";

import { toast } from "sonner";

import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Session } from "@/lib/types";

export type DeleteMode = "soft" | "hard";

// useWorkerControl owns pause/resume/delete of worker intents plus the delete-dialog
// state (target, reason, soft|hard mode).
export function useWorkerControl(taskId: string, patchIntentState: (intentId: string, state?: string) => void) {
  const [controllingIntent, setControllingIntent] = React.useState<string | null>(null);
  const [cancelIntent, setCancelIntent] = React.useState<Session | null>(null);
  const [cancelReason, setCancelReason] = React.useState("");
  // Delete mode: soft = soft delete (default, sets deleted + records the reason, keeps data) | hard = hard delete (cascade-removes exclusive descendants).
  const [deleteMode, setDeleteMode] = React.useState<DeleteMode>("soft");

  const controlWorker = React.useCallback(
    async (session: Session, action: "pause" | "resume" | "cancel", reason?: string, mode?: DeleteMode) => {
      if (!session.intent_id || session.inherited || controllingIntent) return;
      if (action === "cancel" && !reason?.trim()) {
        toast.error(t("Please provide a deletion reason"));
        return;
      }
      setControllingIntent(session.intent_id);
      try {
        const res = await api.controlIntent(taskId, session.intent_id, action, reason, mode);
        if (action === "pause") {
          patchIntentState(session.intent_id, "paused");
          toast.success(t("Worker #{id} paused", { id: session.intent_id }));
        } else if (action === "resume") {
          patchIntentState(session.intent_id, "open");
          toast.success(t("Worker #{id} resumed, waiting to be re-claimed", { id: session.intent_id }));
        } else if (mode === "hard") {
          // Hard delete: the intent and its exclusive downstream are physically removed; drop the row from the list.
          patchIntentState(session.intent_id);
          const d = res.deleted;
          toast.success(
            d
              ? t(
                  "Worker #{id} and its exclusive downstream were permanently deleted (including {intents} intents / {facts} facts / {findings} findings)",
                  { id: session.intent_id, intents: d.intents, facts: d.facts, findings: d.findings },
                )
              : t("Worker #{id} and its exclusive downstream were permanently deleted", { id: session.intent_id }),
          );
          setCancelReason("");
        } else {
          // Soft delete: the intent is set to deleted with the reason recorded; nodes and output are kept.
          patchIntentState(session.intent_id, "deleted");
          toast.success(
            t("Worker #{id} deleted (reason recorded; the planner will re-plan accordingly)", {
              id: session.intent_id,
            }),
          );
          setCancelReason("");
        }
      } catch (error) {
        toast.error(t("Worker operation failed: {error}", { error: (error as Error).message }));
      } finally {
        setControllingIntent(null);
        setCancelIntent(null);
      }
    },
    [controllingIntent, patchIntentState, taskId],
  );

  return {
    controllingIntent,
    cancelIntent,
    setCancelIntent,
    cancelReason,
    setCancelReason,
    deleteMode,
    setDeleteMode,
    controlWorker,
  };
}
