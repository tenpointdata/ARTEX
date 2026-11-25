"use client";

import * as React from "react";

import { CpuIcon, KeyboardIcon, RadioTowerIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import { CHAT_SEND_MODE_OPTIONS, type ChatSendMode, setChatSendMode, useChatSendMode } from "@/lib/chat-send-mode";
import { rich } from "@/lib/rich-text";
import type { Settings } from "@/lib/types";

type SavedProps = { settings: Settings | null; onSaved: (s: Settings) => void };

// Python interpreter for custom script tools. Saving is admin (PUT /api/settings).
export function PythonInterpreterCard({ settings, onSaved }: SavedProps) {
  const canAdmin = useCan("admin");
  const [pyInterp, setPyInterp] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  React.useEffect(() => setPyInterp(settings?.python_interpreter ?? ""), [settings]);
  const loaded = settings !== null && canAdmin;

  const savePython = () => {
    setSaving(true);
    api
      .setSettings({ python_interpreter: pyInterp.trim() })
      .then((s) => {
        onSaved(s);
        toast.success(t("Saved Python interpreter config"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: errorText(e) })))
      .finally(() => setSaving(false));
  };
  const detectPython = () => {
    setSaving(true);
    api
      .detectPython()
      .then((r) => setPyInterp(r.python_interpreter))
      .catch(() => undefined)
      .finally(() => setSaving(false));
  };

  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <RadioTowerIcon className="size-4" />
          {t("Custom scripts · Python interpreter")}
        </CardTitle>
        <CardDescription>
          {rich(
            t(
              "Custom <b>script</b> tools run Python with it. Auto-detected at startup (python3 first); you can enter an absolute path to a venv / specific version here; leave empty to auto-detect at runtime.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center gap-2">
          <Input
            className="font-mono text-sm"
            placeholder={t("/usr/bin/python3 (empty = auto-detect)")}
            value={pyInterp}
            disabled={!loaded || saving}
            onChange={(e) => setPyInterp(e.target.value)}
          />
          {canAdmin && (
            <>
              <Button variant="outline" onClick={detectPython} disabled={!loaded || saving}>
                {t("Re-detect")}
              </Button>
              <Button onClick={savePython} disabled={!loaded || saving}>
                {t("Save")}
              </Button>
            </>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// Concurrent worker agents per task. Saving is admin (PUT /api/settings).
export function WorkersCard({ settings, onSaved }: SavedProps) {
  const canAdmin = useCan("admin");
  const [workers, setWorkers] = React.useState("3");
  const [savingWorkers, setSavingWorkers] = React.useState(false);
  React.useEffect(() => setWorkers(String(settings?.workers ?? 3)), [settings]);
  const loaded = settings !== null && canAdmin;

  const saveWorkers = () => {
    const n = Number(workers);
    if (!Number.isInteger(n) || n <= 0) {
      toast.error(t("Concurrency must be an integer greater than 0"));
      return;
    }
    setSavingWorkers(true);
    api
      .setSettings({ workers: n })
      .then((s) => {
        onSaved(s);
        toast.success(t("Saved the concurrent worker agent count (applies to tasks started afterwards)"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: errorText(e) })))
      .finally(() => setSavingWorkers(false));
  };

  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <CpuIcon className="size-4" />
          {t("Work concurrency · worker agent count")}
        </CardTitle>
        <CardDescription>
          {rich(
            t(
              "The number of worker agents running concurrently per task (default 3). Higher values mean more parallel probing and higher consumption. Changes <b>apply to tasks started afterwards</b>; running tasks are unaffected.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <div className="flex items-center gap-2">
          <Input
            type="number"
            min={1}
            className="w-32 font-mono text-sm"
            placeholder="3"
            value={workers}
            disabled={!loaded || savingWorkers}
            onChange={(e) => setWorkers(e.target.value)}
          />
          {canAdmin && (
            <Button onClick={saveWorkers} disabled={!loaded || savingWorkers}>
              {t("Save")}
            </Button>
          )}
        </div>
      </CardContent>
    </Card>
  );
}

// Browser-local chat send key; not an API setting, so every role can change it.
export function ChatSendModeCard() {
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
        <Label htmlFor="chat-send-mode" className="font-normal text-muted-foreground text-sm">
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
