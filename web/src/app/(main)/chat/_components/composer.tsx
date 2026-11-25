import * as React from "react";

import { ArrowUpIcon, ChevronDownIcon, Loader2Icon, PaperclipIcon, Square, XIcon, ZapIcon } from "lucide-react";

import { MentionTextarea } from "@/components/mention-textarea";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { t } from "@/i18n";
import { shouldSubmitOnKey, useChatSendMode } from "@/lib/chat-send-mode";
import { isBtwCommand } from "@/lib/side-questions";
import type { ChatAttachment, LLMProfile } from "@/lib/types";
import { cn } from "@/lib/utils";

import { fmtBytes } from "./chat-helpers";

// Composer is the shared bottom input (textarea grows to a cap, Enter sends,
// Shift+Enter newlines) — the same affordance across DraftChat and ChatView.
export function Composer({
  value,
  onChange,
  onSend,
  disabled,
  placeholder,
  leftSlot,
  running,
  onStop,
  stopDisabled,
  attachments,
  onPickFiles,
  onRemoveAttachment,
  uploading,
  allowBtw,
}: {
  value: string;
  onChange: (v: string) => void;
  onSend: () => void;
  disabled: boolean;
  placeholder: string;
  leftSlot?: React.ReactNode;
  running?: boolean;
  onStop?: () => void;
  stopDisabled?: boolean;
  // Upload method 1: the paperclip button + attachment chip preview only appear when onPickFiles is provided.
  attachments?: ChatAttachment[];
  onPickFiles?: (files: File[]) => void;
  onRemoveAttachment?: (path: string) => void;
  uploading?: boolean;
  allowBtw?: boolean;
}) {
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const atts = attachments ?? [];
  // The send key binding comes from system settings (localStorage); Enter sends by default.
  const sendMode = useChatSendMode();
  function onKeyDown(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (!shouldSubmitOnKey(e, sendMode)) return;
    e.preventDefault();
    onSend();
  }
  return (
    <div className="border-t p-3">
      {atts.length > 0 && (
        <div className="mb-2 flex flex-wrap gap-1.5">
          {atts.map((a) => (
            <div
              key={a.path}
              className="flex items-center gap-1.5 rounded-md border bg-muted/50 px-2 py-1 text-xs"
              title={a.path}
            >
              <PaperclipIcon className="size-3 shrink-0 text-primary" />
              <span className="max-w-[160px] truncate">{a.name}</span>
              <span className="text-muted-foreground">{fmtBytes(a.size)}</span>
              {onRemoveAttachment && (
                <button
                  type="button"
                  className="ml-0.5 text-muted-foreground hover:text-foreground"
                  onClick={() => onRemoveAttachment(a.path)}
                  title={t("Remove")}
                >
                  <XIcon className="size-3" />
                </button>
              )}
            </div>
          ))}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {leftSlot ? <div className="w-full sm:w-auto">{leftSlot}</div> : null}
        {onPickFiles && (
          <>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={(e) => {
                // A FileList is live-bound to the input element: snapshot it into an array first, then clear value,
                // otherwise the async onPickFiles (which may need to create a conversation first in draft mode) resumes with an empty list.
                const picked = Array.from(e.target.files ?? []);
                e.target.value = ""; // allow re-picking the same file
                if (picked.length > 0) onPickFiles(picked);
              }}
            />
            <Button
              size="icon"
              variant="ghost"
              onClick={() => fileInputRef.current?.click()}
              disabled={disabled || uploading}
              title={t("Upload files")}
            >
              {uploading ? <Loader2Icon className="size-4 animate-spin" /> : <PaperclipIcon className="size-4" />}
            </Button>
          </>
        )}
        <MentionTextarea
          className="max-h-40 min-h-10 min-w-0 flex-1 resize-none"
          rows={1}
          placeholder={placeholder}
          value={value}
          disabled={disabled && !(running && allowBtw)}
          onValueChange={onChange}
          onKeyDown={onKeyDown}
        />
        {running && allowBtw && isBtwCommand(value) && (
          <Button size="icon" onClick={onSend} aria-label={t("Send side question")} title={t("Send side question")}>
            <ArrowUpIcon />
          </Button>
        )}
        {running ? (
          // while a run is in flight the send button becomes a stop button —
          // aborts just this session (the trigger queue keeps going).
          <Button size="icon" variant="destructive" onClick={onStop} disabled={stopDisabled} title={t("Stop this run")}>
            <Square className="size-3.5 fill-current" />
          </Button>
        ) : (
          <Button
            size="icon"
            onClick={onSend}
            disabled={disabled || (!value.trim() && atts.length === 0)}
            title={t("Send message")}
            aria-label={t("Send message")}
          >
            <ArrowUpIcon />
          </Button>
        )}
      </div>
    </div>
  );
}

// LLMProfileRow shows the active LLM config below the composer and lets the user
// switch it via a Popover. `selected` is the profile id or null for default.
export function LLMProfileRow({
  profiles,
  selected,
  onChange,
  disabled,
  rightSlot,
}: {
  profiles: LLMProfile[];
  selected: number | null;
  onChange: (id: number | null) => void;
  disabled?: boolean;
  rightSlot?: React.ReactNode;
}) {
  const [open, setOpen] = React.useState(false);
  const activeDefault = profiles.find((p) => p.is_default);
  const current = selected != null ? profiles.find((p) => Number(p.id) === selected) : null;
  const label = current
    ? current.name
    : activeDefault
      ? t("Default ({name})", { name: activeDefault.name })
      : t("Default");

  return (
    <div className="flex min-w-0 shrink-0 items-center gap-1 px-1 pt-0.5 pb-1">
      <ZapIcon className="text-muted-foreground/50 size-3 shrink-0" />
      <span className="truncate text-muted-foreground/70 text-xs" title={label}>
        {label}
      </span>
      <Popover open={open} onOpenChange={disabled ? undefined : setOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            disabled={disabled}
            className="flex shrink-0 items-center gap-0.5 text-primary text-xs hover:underline disabled:pointer-events-none disabled:opacity-40"
          >
            {t("Change")}
            <ChevronDownIcon className="size-3" />
          </button>
        </PopoverTrigger>
        <PopoverContent align="start" className="w-64 p-1">
          <p className="text-muted-foreground px-2 py-1 text-[11px] font-medium">{t("Select an LLM profile")}</p>
          {/* default option */}
          <button
            type="button"
            onClick={() => {
              onChange(null);
              setOpen(false);
            }}
            className={cn(
              "flex w-full flex-col rounded px-2 py-1.5 text-left hover:bg-accent",
              selected == null && "bg-accent",
            )}
          >
            <span className="text-sm">
              {activeDefault ? t("Default ({name})", { name: activeDefault.name }) : t("Default")}
            </span>
            {activeDefault && (
              <span className="text-muted-foreground text-[11px]">
                {activeDefault.format} · {activeDefault.model}
              </span>
            )}
          </button>
          {profiles.map((p) => (
            <button
              key={p.id}
              type="button"
              onClick={() => {
                onChange(Number(p.id));
                setOpen(false);
              }}
              className={cn(
                "flex w-full flex-col rounded px-2 py-1.5 text-left hover:bg-accent",
                selected === Number(p.id) && "bg-accent",
              )}
            >
              <span className="text-sm">{p.name}</span>
              <span className="text-muted-foreground text-[11px]">
                {p.format} · {p.model}
              </span>
            </button>
          ))}
        </PopoverContent>
      </Popover>
      {rightSlot}
    </div>
  );
}
