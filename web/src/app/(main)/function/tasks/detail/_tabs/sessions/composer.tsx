import { ArrowUpIcon, Loader2Icon, PaperclipIcon, SquareIcon, XIcon } from "lucide-react";

import { MentionTextarea } from "@/components/mention-textarea";
import { InputGroup, InputGroupAddon, InputGroupButton } from "@/components/ui/input-group";
import { Spinner } from "@/components/ui/spinner";
import { t } from "@/i18n";
import { type ChatSendMode, shouldSubmitOnKey } from "@/lib/chat-send-mode";
import { isBtwCommand } from "@/lib/side-questions";
import type { Session } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtBytes, MAX_WORKER_MESSAGE_CHARS, workerMessageCharCount } from "./helpers";
import type { useMainComposer } from "./use-main-composer";
import type { useWorkerChat } from "./use-worker-chat";

// MainComposer is the main-agent message box (attachments, send, stop, /btw).
export function MainComposer({
  composer,
  sending,
  sendMode,
}: {
  composer: ReturnType<typeof useMainComposer>;
  sending: boolean;
  sendMode: ChatSendMode;
}) {
  const {
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
  } = composer;
  return (
    <div className="border-t p-3">
      {attachments.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {attachments.map((a) => (
            <div
              key={a.path}
              className="flex items-center gap-1.5 rounded-md border bg-muted/50 px-2 py-1 text-xs"
              title={a.path}
            >
              <PaperclipIcon className="size-3 shrink-0 text-primary" />
              <span className="max-w-[160px] truncate">{a.name}</span>
              <span className="text-muted-foreground">{fmtBytes(a.size)}</span>
              <button
                type="button"
                className="ml-0.5 text-muted-foreground hover:text-foreground"
                onClick={() => setAttachments((p) => p.filter((x) => x.path !== a.path))}
                title={t("Remove")}
              >
                <XIcon className="size-3" />
              </button>
            </div>
          ))}
        </div>
      )}
      <input
        ref={fileInputRef}
        type="file"
        multiple
        className="hidden"
        onChange={(e) => void pickFiles(e.target.files)}
      />
      <InputGroup className="min-h-9 has-disabled:opacity-100">
        <MentionTextarea
          inputGroup
          rows={1}
          aria-label={t("Message the main agent")}
          placeholder={
            mainBusy
              ? t("The main agent is running; type /btw to ask a side question…")
              : t("Message the main agent; @ to reference findings, assets, and more…")
          }
          value={input}
          disabled={sending}
          onValueChange={setInput}
          onKeyDown={(e) => {
            if (!shouldSubmitOnKey(e, sendMode)) return;
            e.preventDefault();
            send();
          }}
          className="max-h-36 min-h-9 overflow-y-auto"
        />
        <InputGroupAddon align="block-end">
          <InputGroupButton
            size="icon-xs"
            variant="ghost"
            onClick={() => fileInputRef.current?.click()}
            disabled={mainBusy || uploading}
            title={t("Upload file")}
            aria-label={t("Upload file")}
          >
            {uploading ? <Loader2Icon className="animate-spin" /> : <PaperclipIcon />}
          </InputGroupButton>
          {mainBusy && isBtwCommand(input) && (
            <InputGroupButton size="icon-xs" onClick={send} aria-label={t("Send side question")}>
              <ArrowUpIcon />
            </InputGroupButton>
          )}
          {mainBusy ? (
            <InputGroupButton
              className="ml-auto"
              size="icon-xs"
              variant="destructive"
              onClick={stop}
              disabled={stopping}
              title={t("Stop current execution")}
              aria-label={t("Stop current execution")}
            >
              {stopping ? <Loader2Icon className="animate-spin" /> : <SquareIcon />}
            </InputGroupButton>
          ) : (
            <InputGroupButton
              className="ml-auto"
              size="icon-xs"
              variant="default"
              onClick={send}
              disabled={(!input.trim() && attachments.length === 0) || sending}
              title={t("Send message")}
              aria-label={t("Send message")}
            >
              {sending ? <Loader2Icon className="animate-spin" /> : <ArrowUpIcon />}
            </InputGroupButton>
          )}
        </InputGroupAddon>
      </InputGroup>
    </div>
  );
}

// WorkerComposer messages a running/paused worker; a running worker can be paused,
// a paused one continued directly or with a message.
export function WorkerComposer({
  active,
  chat,
  sendMode,
  controllingIntent,
  onPause,
  onResume,
}: {
  active: Session;
  chat: ReturnType<typeof useWorkerChat>;
  sendMode: ChatSendMode;
  controllingIntent: string | null;
  onPause: () => void;
  onResume: () => void;
}) {
  const { workerMessage, changeWorkerMessage, workerMessageSending, sendWorkerChat } = chat;
  return (
    <div className="border-t p-3">
      <InputGroup className="min-h-9 has-disabled:opacity-100">
        <MentionTextarea
          inputGroup
          rows={1}
          aria-label={t("Message Worker #{id}", { id: active.intent_id ?? "" })}
          placeholder={t("Message Worker #{id}; @ to reference records and adjust execution direction…", {
            id: active.intent_id ?? "",
          })}
          value={workerMessage}
          onValueChange={changeWorkerMessage}
          onKeyDown={(event) => {
            if (!shouldSubmitOnKey(event, sendMode)) return;
            event.preventDefault();
            sendWorkerChat();
          }}
          disabled={workerMessageSending}
          aria-invalid={workerMessageCharCount(workerMessage) > MAX_WORKER_MESSAGE_CHARS}
          className="max-h-36 min-h-9 overflow-y-auto"
        />
        <InputGroupAddon align="block-end">
          <span
            className={cn(
              "px-1 text-[10px] tabular-nums text-muted-foreground",
              workerMessageCharCount(workerMessage) > MAX_WORKER_MESSAGE_CHARS && "text-destructive",
            )}
          >
            {workerMessageCharCount(workerMessage)}/{MAX_WORKER_MESSAGE_CHARS}
          </span>
          {active.status === "running" && isBtwCommand(workerMessage) && (
            <InputGroupButton size="icon-xs" onClick={sendWorkerChat} aria-label={t("Send side question")}>
              <ArrowUpIcon />
            </InputGroupButton>
          )}
          {active.status === "running" ? (
            <InputGroupButton
              className="ml-auto"
              size="icon-xs"
              variant="destructive"
              onClick={onPause}
              disabled={controllingIntent === active.intent_id}
              title={t("Pause current worker")}
              aria-label={t("Pause current worker")}
            >
              {controllingIntent === active.intent_id ? <Loader2Icon className="animate-spin" /> : <SquareIcon />}
            </InputGroupButton>
          ) : (
            <>
              <InputGroupButton
                className="ml-auto"
                size="xs"
                variant="ghost"
                onClick={onResume}
                disabled={controllingIntent === active.intent_id || workerMessageSending}
                title={t("Continue execution directly without sending a message")}
                aria-label={t("Continue execution directly")}
              >
                {controllingIntent === active.intent_id ? (
                  <Loader2Icon className="animate-spin" />
                ) : (
                  t("Continue directly")
                )}
              </InputGroupButton>
              <InputGroupButton
                size="icon-xs"
                variant="default"
                onClick={sendWorkerChat}
                disabled={
                  workerMessageSending ||
                  !workerMessage.trim() ||
                  workerMessageCharCount(workerMessage) > MAX_WORKER_MESSAGE_CHARS
                }
                title={t("Send message")}
                aria-label={t("Send message")}
              >
                {workerMessageSending ? <Spinner /> : <ArrowUpIcon />}
              </InputGroupButton>
            </>
          )}
        </InputGroupAddon>
      </InputGroup>
    </div>
  );
}
