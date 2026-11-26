import * as React from "react";

import { toast } from "sonner";

import type { SideQuestions } from "@/hooks/use-side-questions";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Session } from "@/lib/types";

import { MAX_WORKER_MESSAGE_CHARS, newWorkerMessageRequestID, workerMessageCharCount } from "./helpers";

// useWorkerChat owns the paused-worker composer. The request id is kept across retries
// of the same text so the server can dedupe a re-sent message.
export function useWorkerChat({
  taskId,
  active,
  side,
  patchIntentState,
}: {
  taskId: string;
  active: Session;
  side: SideQuestions;
  patchIntentState: (intentId: string, state?: string) => void;
}) {
  const [workerMessage, setWorkerMessage] = React.useState("");
  const [workerMessageRequestId, setWorkerMessageRequestId] = React.useState("");
  const [workerMessageSending, setWorkerMessageSending] = React.useState(false);

  function changeWorkerMessage(value: string) {
    setWorkerMessage(value);
    setWorkerMessageRequestId("");
  }

  function sendWorkerChat() {
    const intentId = active.intent_id;
    const message = workerMessage.trim();
    if (side.handleCommand(message, () => setWorkerMessage(""))) return;
    if (!intentId || active.inherited || active.status !== "paused" || workerMessageSending || !message) return;
    if (workerMessageCharCount(message) > MAX_WORKER_MESSAGE_CHARS) {
      toast.error(t("Messages cannot exceed {count} characters", { count: MAX_WORKER_MESSAGE_CHARS }));
      return;
    }
    const requestId = workerMessageRequestId || newWorkerMessageRequestID();
    if (!workerMessageRequestId) setWorkerMessageRequestId(requestId);
    setWorkerMessageSending(true);
    api
      .sendWorkerMessage(taskId, intentId, message, requestId)
      .then((result) => {
        // The server records the user turn and streams the run over SSE, so there is
        // no optimistic insert: the message and the continuation arrive live.
        patchIntentState(intentId, result.state);
        setWorkerMessage("");
        setWorkerMessageRequestId("");
        toast.success(t("Message sent to Worker #{id}; execution resumed immediately", { id: intentId }));
      })
      .catch((error) => {
        toast.error(t("Send failed: {error}", { error: (error as Error).message || t("please retry later") }));
      })
      .finally(() => setWorkerMessageSending(false));
  }

  return {
    workerMessage,
    changeWorkerMessage,
    resetWorkerMessage: () => changeWorkerMessage(""),
    workerMessageSending,
    sendWorkerChat,
  };
}
