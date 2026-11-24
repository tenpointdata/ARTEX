"use client";

import { KeyboardIcon } from "lucide-react";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { t } from "@/i18n";
import { CHAT_SEND_MODE_OPTIONS, type ChatSendMode, setChatSendMode, useChatSendMode } from "@/lib/chat-send-mode";

import { rich } from "./rich";

export function ChatSendKeyCard() {
  // Pure frontend preference: bypasses /api/settings, reads and writes localStorage directly.
  const sendMode = useChatSendMode();
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <KeyboardIcon className="size-4" />
          {t("Chat input send key binding")}
        </CardTitle>
        <CardDescription>
          {t(
            "Shared by the main-agent chat inputs on the chat page and task details; takes effect immediately on selection, no saving needed.",
          )}
          <br />
          {rich(
            t(
              "This preference <b>lives only in this browser</b>, does not sync with the account, and must be set again after switching browsers or clearing site data.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-4">
        <Label htmlFor="chat-send-mode" className="text-sm font-normal text-muted-foreground">
          {t("Send mode")}
        </Label>
        <Select value={sendMode} onValueChange={(v) => setChatSendMode(v as ChatSendMode)}>
          <SelectTrigger id="chat-send-mode" className="w-72">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {CHAT_SEND_MODE_OPTIONS.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </CardContent>
    </Card>
  );
}
