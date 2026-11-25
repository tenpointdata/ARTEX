import * as React from "react";

import { toast } from "sonner";

import type { SideQuestions } from "@/hooks/use-side-questions";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { ChatAttachment } from "@/lib/types";

// useMainComposer owns the main-agent chat composer: text, file attachments, send and
// stop. `sending` is owned by the caller because the session list's liveness needs it.
export function useMainComposer({
  taskId,
  seg,
  isMain,
  externalBusy,
  side,
  sending,
  setSending,
  setMainChatRunning,
  chatStatusRequestRef,
}: {
  taskId: string;
  seg: number;
  isMain: boolean;
  // The main turn is running per the server / recent activity (before counting `sending`).
  externalBusy: boolean;
  side: SideQuestions;
  sending: boolean;
  setSending: (value: boolean) => void;
  setMainChatRunning: (value: boolean) => void;
  chatStatusRequestRef: React.MutableRefObject<number>;
}) {
  const [input, setInput] = React.useState("");
  const [stopping, setStopping] = React.useState(false);
  // Method 1, file upload: chosen attachments (already landed in the task work dir uploads/) are sent with the next message.
  const [attachments, setAttachments] = React.useState<ChatAttachment[]>([]);
  const [uploading, setUploading] = React.useState(false);
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  // A main turn is serialized per task (chat lock), so "busy" is task-wide.
  const mainBusy = (isMain && sending) || (isMain && externalBusy);

  async function pickFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    setUploading(true);
    try {
      const r = await api.chatUpload("task", taskId, Array.from(files));
      setAttachments((prev) => [...prev, ...r.attachments]);
    } catch (e) {
      toast.error(t("Upload failed: {error}", { error: (e as Error).message }));
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = "";
    }
  }

  function stop() {
    if (stopping) return;
    setStopping(true);
    void api.stopChat(taskId).finally(() => setStopping(false));
  }

  function send() {
    const text = input.trim();
    const atts = attachments;
    if (side.handleCommand(text, () => setInput(""))) return;
    if ((!text && atts.length === 0) || sending || mainBusy) return;
    // No optimistic echo: the backend persists+broadcasts the human turn before it
    // returns, so it streams back over SSE (worker="mainagent") with its real DB
    // seq — the transcript renders it from server data like every other step. Clear
    // the composer eagerly for responsiveness; restore it if the send fails.
    setInput("");
    setAttachments([]);
    setSending(true);
    chatStatusRequestRef.current++;
    api
      .chat(text, taskId, atts.length > 0 ? atts : undefined, seg)
      .then(({ mode }) => {
        chatStatusRequestRef.current++;
        setMainChatRunning(mode === "llm");
      })
      .catch((e) => {
        setInput(text); // restore so the user doesn't lose their text / attachments
        setAttachments(atts);
        toast.error(t("Send failed: {error}", { error: (e as Error).message || t("please retry later") }));
      })
      .finally(() => setSending(false));
  }

  return {
    input,
    setInput,
    attachments,
    setAttachments,
    uploading,
    stopping,
    fileInputRef,
    pickFiles,
    stop,
    send,
    mainBusy,
  };
}
