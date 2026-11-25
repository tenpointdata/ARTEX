"use client";

import * as React from "react";

import { PencilIcon, SaveIcon, Trash2Icon, XIcon } from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import { rich } from "@/lib/rich-text";
import type { Agent, AgentTrigger, Tool } from "@/lib/types";
import { cn } from "@/lib/utils";

// AgentTriggersTab manages a custom agent's P3 triggers: list + add + delete.
// Each trigger fires (schedule/finding found/goal met/task timeout/tool call, multi-select) → a new conversation runs
// in parallel with the base user message + auto context appended by the backend.
type RunMode = "serial" | "parallel";
type MergeMode = "by_task" | "all" | "none";

function runModeHint(runMode: RunMode, mergeMode: MergeMode): string {
  if (runMode === "parallel") {
    return t(
      "Parallel: each trigger immediately starts its own concurrent session, no merging; triggers beyond the max concurrency queue for a free slot.",
    );
  }
  if (mergeMode === "by_task") {
    return t(
      "Serial · merge by task: one run at a time per agent; queued events for the same task merge into one conversation.",
    );
  }
  if (mergeMode === "all") {
    return t(
      "Serial · merge all: one run at a time per agent; when dequeuing, all currently queued triggers merge into one conversation.",
    );
  }
  return t("Serial · no merge: one run at a time per agent; each trigger gets its own conversation.");
}

export function AgentTriggersTab({ agentKey, agent }: { agentKey: string; agent?: Agent }) {
  // Creating a trigger and the handling policy live under /api/agents (admin);
  // updating or deleting an existing trigger is /api/triggers/{id} (operate).
  const canAdmin = useCan("admin");
  const canOperate = useCan("operate");
  const [triggers, setTriggers] = React.useState<AgentTrigger[]>([]);
  const [tools, setTools] = React.useState<Tool[]>([]);
  // post-trigger handling policy (per agent); initial value comes from agent detail, saved on change.
  const [runMode, setRunMode] = React.useState<"serial" | "parallel">(agent?.trigger_run_mode ?? "serial");
  const [mergeMode, setMergeMode] = React.useState<"by_task" | "all" | "none">(agent?.trigger_merge_mode ?? "all");
  const [maxParallel, setMaxParallel] = React.useState(String(agent?.trigger_max_parallel ?? 5));
  React.useEffect(() => {
    setRunMode(agent?.trigger_run_mode ?? "serial");
    setMergeMode(agent?.trigger_merge_mode ?? "all");
    setMaxParallel(String(agent?.trigger_max_parallel ?? 5));
  }, [agent?.trigger_run_mode, agent?.trigger_merge_mode, agent?.trigger_max_parallel]);

  async function saveBehavior(patch: {
    trigger_run_mode?: "serial" | "parallel";
    trigger_merge_mode?: "by_task" | "all" | "none";
    trigger_max_parallel?: number;
  }) {
    try {
      await api.saveAgentConfig(agentKey, patch);
    } catch (e) {
      toast.error(t("Failed to save policy: {error}", { error: errorText(e) }));
    }
  }
  const [onInterval, setOnInterval] = React.useState(false);
  const [intervalSec, setIntervalSec] = React.useState("60");
  const [onFinding, setOnFinding] = React.useState(false);
  const [onGoalMet, setOnGoalMet] = React.useState(false);
  const [onTaskTimeout, setOnTaskTimeout] = React.useState(false);
  const [onToolCall, setOnToolCall] = React.useState(false);
  const [onTaskCreate, setOnTaskCreate] = React.useState(false);
  const [intervalMsg, setIntervalMsg] = React.useState("");
  const [findingMsg, setFindingMsg] = React.useState("");
  const [goalMsg, setGoalMsg] = React.useState("");
  const [taskTimeoutMsg, setTaskTimeoutMsg] = React.useState("");
  const [toolCallMsg, setToolCallMsg] = React.useState("");
  const [taskCreateMsg, setTaskCreateMsg] = React.useState("");
  const [toolNames, setToolNames] = React.useState<string[]>([]);
  const [saving, setSaving] = React.useState(false);
  // null = create mode; non-null = editing the trigger with that id.
  const [editingId, setEditingId] = React.useState<number | null>(null);

  const reload = React.useCallback(() => {
    api
      .agentTriggers(agentKey)
      .then(setTriggers)
      .catch(() => setTriggers([]));
  }, [agentKey]);
  React.useEffect(() => {
    reload();
  }, [reload]);
  React.useEffect(() => {
    api
      .tools()
      .then(setTools)
      .catch(() => setTools([]));
  }, []);

  function toggleTool(key: string) {
    setToolNames((prev) => (prev.includes(key) ? prev.filter((k) => k !== key) : [...prev, key]));
  }

  // resetForm clears the form and returns to "create" mode.
  function resetForm() {
    setEditingId(null);
    setOnInterval(false);
    setIntervalSec("60");
    setOnFinding(false);
    setOnGoalMet(false);
    setOnTaskTimeout(false);
    setOnToolCall(false);
    setOnTaskCreate(false);
    setIntervalMsg("");
    setFindingMsg("");
    setGoalMsg("");
    setTaskTimeoutMsg("");
    setToolCallMsg("");
    setTaskCreateMsg("");
    setToolNames([]);
  }

  // startEdit fills the form with an existing trigger and enters "edit" mode.
  function startEdit(trg: AgentTrigger) {
    setEditingId(trg.id);
    setOnInterval(trg.interval_sec > 0);
    setIntervalSec(trg.interval_sec > 0 ? String(trg.interval_sec) : "60");
    setOnFinding(trg.on_finding);
    setOnGoalMet(trg.on_goal_met);
    setOnTaskTimeout(trg.on_task_timeout);
    setOnToolCall(trg.on_tool_call);
    setOnTaskCreate(trg.on_task_create);
    setIntervalMsg(trg.interval_message);
    setFindingMsg(trg.finding_message);
    setGoalMsg(trg.goal_message);
    setTaskTimeoutMsg(trg.task_timeout_message);
    setToolCallMsg(trg.tool_call_message);
    setTaskCreateMsg(trg.task_create_message);
    setToolNames(trg.tool_names);
  }

  // submit creates or saves changes based on editingId; editing keeps the trigger's enabled state.
  async function submit() {
    const n = onInterval ? Math.max(1, Math.floor(Number(intervalSec) || 0)) : 0;
    if (n === 0 && !onFinding && !onGoalMet && !onTaskTimeout && !onToolCall && !onTaskCreate) {
      toast.error(t("Select at least one trigger condition"));
      return;
    }
    if (onToolCall && toolNames.length === 0) {
      toast.error(t("Tool-call triggers require at least one tool"));
      return;
    }
    const body = {
      interval_sec: n,
      on_finding: onFinding,
      on_goal_met: onGoalMet,
      on_task_timeout: onTaskTimeout,
      on_tool_call: onToolCall,
      on_task_create: onTaskCreate,
      interval_message: intervalMsg.trim(),
      finding_message: findingMsg.trim(),
      goal_message: goalMsg.trim(),
      task_timeout_message: taskTimeoutMsg.trim(),
      tool_call_message: toolCallMsg.trim(),
      task_create_message: taskCreateMsg.trim(),
      tool_names: onToolCall ? toolNames : [],
    };
    setSaving(true);
    try {
      if (editingId != null) {
        const cur = triggers.find((x) => x.id === editingId);
        await api.updateTrigger(editingId, { ...body, enabled: cur?.enabled ?? true });
        toast.success(t("Changes saved"));
      } else {
        await api.createTrigger(agentKey, { ...body, enabled: true });
        toast.success(t("Trigger added"));
      }
      resetForm();
      reload();
    } catch (e) {
      const error = errorText(e);
      toast.error(editingId != null ? t("Save failed: {error}", { error }) : t("Add failed: {error}", { error }));
    } finally {
      setSaving(false);
    }
  }
  async function toggleEnabled(trg: AgentTrigger) {
    try {
      await api.updateTrigger(trg.id, {
        enabled: !trg.enabled,
        interval_sec: trg.interval_sec,
        on_finding: trg.on_finding,
        on_goal_met: trg.on_goal_met,
        on_task_timeout: trg.on_task_timeout,
        on_tool_call: trg.on_tool_call,
        on_task_create: trg.on_task_create,
        interval_message: trg.interval_message,
        finding_message: trg.finding_message,
        goal_message: trg.goal_message,
        task_timeout_message: trg.task_timeout_message,
        tool_call_message: trg.tool_call_message,
        task_create_message: trg.task_create_message,
        tool_names: trg.tool_names,
      });
      reload();
    } catch (e) {
      toast.error(t("Save failed: {error}", { error: errorText(e) }));
    }
  }
  async function del(id: number) {
    try {
      await api.deleteTrigger(id);
      if (editingId === id) resetForm();
      reload();
    } catch (e) {
      toast.error(t("Delete failed: {error}", { error: errorText(e) }));
    }
  }

  function condLabel(trg: AgentTrigger): string {
    const parts: string[] = [];
    if (trg.interval_sec > 0) parts.push(t("every {n}s", { n: trg.interval_sec }));
    if (trg.on_finding) parts.push(t("finding found"));
    if (trg.on_goal_met) parts.push(t("goal met"));
    if (trg.on_task_timeout) parts.push(t("task timeout"));
    if (trg.on_tool_call) parts.push(t("tool call ({count})", { count: trg.tool_names.length }));
    if (trg.on_task_create) parts.push(t("task created"));
    return parts.join(" · ") || t("(no conditions)");
  }

  return (
    <div className="grid gap-4">
      <p className="text-muted-foreground text-xs">
        {rich(
          t(
            'Triggers make this custom Agent run automatically: each trigger <b>starts a new conversation</b> (visible on the Chat page). Multiple trigger conditions can be selected; the system automatically appends "why this fired + related task/finding/goal" to the base message you write.',
          ),
        )}
      </p>

      {/* post-trigger handling policy */}
      <fieldset disabled={!canAdmin} className="grid min-w-0 gap-3 rounded-md border p-3">
        <Label className="text-muted-foreground text-xs">
          {t("Post-trigger handling policy (how triggers are queued/merged to run)")}
        </Label>
        <div className="flex flex-wrap items-center gap-4">
          <div className="grid gap-1">
            <Label className="text-xs">{t("Run mode")}</Label>
            <Select
              value={runMode}
              onValueChange={(v) => {
                const rm = v as "serial" | "parallel";
                setRunMode(rm);
                void saveBehavior({ trigger_run_mode: rm });
              }}
            >
              <SelectTrigger size="sm" className="h-8 w-40">
                <SelectValue />
              </SelectTrigger>
              <SelectContent position="popper">
                <SelectItem value="serial">{t("Serial (queued, one at a time)")}</SelectItem>
                <SelectItem value="parallel">{t("Parallel (independent concurrent sessions)")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          <div className="grid gap-1">
            <Label className="text-xs">{t("Merge mode")}</Label>
            <Select
              value={mergeMode}
              disabled={runMode === "parallel"}
              onValueChange={(v) => {
                const mm = v as "by_task" | "all" | "none";
                setMergeMode(mm);
                void saveBehavior({ trigger_merge_mode: mm });
              }}
            >
              <SelectTrigger size="sm" className="h-8 w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent position="popper">
                <SelectItem value="by_task">{t("Merge by task")}</SelectItem>
                <SelectItem value="all">{t("Merge all into one")}</SelectItem>
                <SelectItem value="none">{t("No merge")}</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {runMode === "parallel" && (
            <div className="grid gap-1">
              <Label htmlFor="tr-maxpar" className="text-xs">
                {t("Max concurrency (0 = unlimited)")}
              </Label>
              <Input
                id="tr-maxpar"
                type="number"
                min={0}
                className="h-8 w-28"
                value={maxParallel}
                onChange={(e) => setMaxParallel(e.target.value)}
                onBlur={() => {
                  const n = Math.max(0, Math.floor(Number(maxParallel) || 0));
                  setMaxParallel(String(n));
                  void saveBehavior({ trigger_max_parallel: n });
                }}
              />
            </div>
          )}
        </div>
        <p className="text-muted-foreground text-xs">{runModeHint(runMode, mergeMode)}</p>
      </fieldset>

      {/* create / edit trigger */}
      {(canAdmin || editingId != null) && (
        <fieldset disabled={!canOperate} className="grid min-w-0 gap-3 rounded-md border p-3">
          <Label className="text-muted-foreground text-xs">
            {editingId != null
              ? t('Edit trigger #{id} (click "Save changes" when done)', { id: editingId })
              : t("New trigger (each condition can have its own user message)")}
          </Label>

          {/* schedule */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-1" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-1" checked={onInterval} onCheckedChange={(v) => setOnInterval(!!v)} />{" "}
              {t("Scheduled trigger")}
            </label>
            {onInterval && (
              <div className="grid gap-1.5">
                <div className="flex items-center gap-2">
                  <Label htmlFor="tr-interval" className="text-xs">
                    {t("Every")}
                  </Label>
                  <Input
                    id="tr-interval"
                    type="number"
                    min={1}
                    className="h-8 w-24"
                    value={intervalSec}
                    onChange={(e) => setIntervalSec(e.target.value)}
                  />
                  <span className="text-muted-foreground text-xs">{t("seconds")}</span>
                </div>
                <Textarea
                  className="text-xs"
                  rows={2}
                  value={intervalMsg}
                  placeholder={t("Message sent to the agent when the schedule fires, e.g. inspect all tasks")}
                  onChange={(e) => setIntervalMsg(e.target.value)}
                />
              </div>
            )}
          </div>

          {/* finding */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-2" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-2" checked={onFinding} onCheckedChange={(v) => setOnFinding(!!v)} />{" "}
              {t("Trigger on finding")}
            </label>
            {onFinding && (
              <Textarea
                className="text-xs"
                rows={2}
                value={findingMsg}
                placeholder={t(
                  "Message sent to the agent when a finding is found (the system attaches task and finding details)",
                )}
                onChange={(e) => setFindingMsg(e.target.value)}
              />
            )}
          </div>

          {/* goal met */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-3" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-3" checked={onGoalMet} onCheckedChange={(v) => setOnGoalMet(!!v)} />{" "}
              {t("Trigger on goal met")}
            </label>
            {onGoalMet && (
              <Textarea
                className="text-xs"
                rows={2}
                value={goalMsg}
                placeholder={t(
                  "Message sent to the agent when a goal is met (the system attaches the task and the met goal)",
                )}
                onChange={(e) => setGoalMsg(e.target.value)}
              />
            )}
          </div>

          {/* task timeout */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-4" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-4" checked={onTaskTimeout} onCheckedChange={(v) => setOnTaskTimeout(!!v)} />{" "}
              {t("Trigger on task timeout")}
            </label>
            {onTaskTimeout && (
              <Textarea
                className="text-xs"
                rows={2}
                value={taskTimeoutMsg}
                placeholder={t("Message sent to the agent on task timeout (the system attaches the task id and goals)")}
                onChange={(e) => setTaskTimeoutMsg(e.target.value)}
              />
            )}
          </div>

          {/* tool call */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-5" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-5" checked={onToolCall} onCheckedChange={(v) => setOnToolCall(!!v)} />{" "}
              {t("Trigger on tool call")}
            </label>
            {onToolCall && (
              <div className="grid gap-1.5">
                <div className="text-muted-foreground text-xs">
                  {rich(
                    t(
                      "Select the tools to watch (at least one); each time these tools finish a <b>call</b> during task execution, a trigger fires. {count} selected.",
                      { count: toolNames.length },
                    ),
                  )}
                </div>
                <div className="max-h-40 overflow-y-auto rounded-md border p-2">
                  {tools.length === 0 && (
                    <span className="text-muted-foreground text-xs">{t("(tool list is empty)")}</span>
                  )}
                  <div className="grid gap-1">
                    {tools.map((tool) => (
                      <label htmlFor={`tr-trig-${tool.key}`} key={tool.key} className="flex items-start gap-2 text-xs">
                        <Checkbox
                          id={`tr-trig-${tool.key}`}
                          className="mt-0.5"
                          checked={toolNames.includes(tool.key)}
                          onCheckedChange={() => toggleTool(tool.key)}
                        />
                        <span className="min-w-0">
                          <span className="font-medium">{tool.key}</span>
                          {tool.description && (
                            <span className="line-clamp-1 text-muted-foreground"> {tool.description}</span>
                          )}
                        </span>
                      </label>
                    ))}
                  </div>
                </div>
                <Textarea
                  className="text-xs"
                  rows={2}
                  value={toolCallMsg}
                  placeholder={t(
                    "Message sent to the agent when a tool is called (the system attaches task info, tool input and output)",
                  )}
                  onChange={(e) => setToolCallMsg(e.target.value)}
                />
              </div>
            )}
          </div>

          {/* task created */}
          <div className="grid gap-1.5">
            <label htmlFor="tr-trig-6" className="flex items-center gap-2 text-sm">
              <Checkbox id="tr-trig-6" checked={onTaskCreate} onCheckedChange={(v) => setOnTaskCreate(!!v)} />{" "}
              {t("Trigger on task created")}
            </label>
            {onTaskCreate && (
              <Textarea
                className="text-xs"
                rows={2}
                value={taskCreateMsg}
                placeholder={t(
                  "Message sent to the agent when a task is created (the system attaches the task id and goals)",
                )}
                onChange={(e) => setTaskCreateMsg(e.target.value)}
              />
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button size="sm" onClick={submit} disabled={saving}>
              <SaveIcon /> {editingId != null ? t("Save changes") : t("Add trigger")}
            </Button>
            {editingId != null && (
              <Button size="sm" variant="ghost" onClick={resetForm} disabled={saving}>
                <XIcon /> {t("Cancel edit")}
              </Button>
            )}
          </div>
        </fieldset>
      )}

      {/* existing triggers */}
      <div className="grid gap-2">
        <Label className="text-muted-foreground text-xs">{t("Existing triggers")}</Label>
        {triggers.length === 0 && <span className="text-muted-foreground text-xs">{t("(none)")}</span>}
        {triggers.map((trg) => (
          <div
            key={trg.id}
            className={cn(
              "flex items-start gap-2 rounded-md border p-2 text-sm",
              editingId === trg.id && "border-primary bg-primary/5",
            )}
          >
            <Switch
              checked={trg.enabled}
              disabled={!canOperate}
              onCheckedChange={() => toggleEnabled(trg)}
              className="mt-0.5"
            />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="font-medium">{condLabel(trg)}</span>
                {!trg.enabled && (
                  <Badge variant="outline" className="px-1 py-0 text-[9px] text-destructive">
                    {t("Disabled")}
                  </Badge>
                )}
              </div>
              <div className="grid gap-0.5 text-muted-foreground text-xs">
                {trg.interval_sec > 0 && trg.interval_message && (
                  <div className="line-clamp-1">{t("Schedule: {message}", { message: trg.interval_message })}</div>
                )}
                {trg.on_finding && trg.finding_message && (
                  <div className="line-clamp-1">{t("Finding: {message}", { message: trg.finding_message })}</div>
                )}
                {trg.on_goal_met && trg.goal_message && (
                  <div className="line-clamp-1">{t("Goal: {message}", { message: trg.goal_message })}</div>
                )}
                {trg.on_task_timeout && trg.task_timeout_message && (
                  <div className="line-clamp-1">{t("Timeout: {message}", { message: trg.task_timeout_message })}</div>
                )}
                {trg.on_task_create && trg.task_create_message && (
                  <div className="line-clamp-1">
                    {t("Task created: {message}", { message: trg.task_create_message })}
                  </div>
                )}
                {trg.on_tool_call && (
                  <>
                    <div className="line-clamp-1">
                      {t("Tools: {tools}", { tools: trg.tool_names.join(", ") || t("(none selected)") })}
                    </div>
                    {trg.tool_call_message && (
                      <div className="line-clamp-1">{t("Message: {message}", { message: trg.tool_call_message })}</div>
                    )}
                  </>
                )}
              </div>
            </div>
            {canOperate && (
              <>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:text-foreground"
                  onClick={() => startEdit(trg)}
                  title={t("Edit")}
                  aria-label={t("Edit")}
                >
                  <PencilIcon className="size-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => del(trg.id)}
                  title={t("Delete")}
                  aria-label={t("Delete")}
                >
                  <Trash2Icon className="size-3.5" />
                </Button>
              </>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
