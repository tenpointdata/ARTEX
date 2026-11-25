"use client";

import { CpuIcon, FlaskConicalIcon, RadioTowerIcon, ShieldAlertIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { t } from "@/i18n";

import { rich } from "./rich";

export function ConstraintInjectionCard({
  injectPlanner,
  injectWorker,
  loaded,
  toggleInjectPlanner,
  toggleInjectWorker,
}: {
  injectPlanner: boolean;
  injectWorker: boolean;
  loaded: boolean;
  toggleInjectPlanner: (v: boolean) => void;
  toggleInjectWorker: (v: boolean) => void;
}) {
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldAlertIcon className="size-4" />
          {t("Operation constraint injection")}
        </CardTitle>
        <CardDescription>
          {rich(
            t(
              'When enabled, each task\'s <b>operation constraints</b> (the allow/deny entries maintained under "Operation constraints" in the task overview) are spliced into the corresponding agent\'s system prompt to bound the exploration scope (e.g. "only test the current port", "no brute forcing").',
            ),
          )}
          <br />
          {rich(
            t(
              "You can control injection into the <b>planner</b> and the <b>worker</b> separately; both are on by default. Switching takes effect immediately (read on the next turn) without rebuilding agents. When off, the agent no longer sees the constraints.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="inject-planner" className="text-sm font-normal text-muted-foreground">
            {injectPlanner ? t("Inject into planner · On") : t("Inject into planner · Off")}
          </Label>
          <Switch
            id="inject-planner"
            checked={injectPlanner}
            disabled={!loaded}
            onCheckedChange={toggleInjectPlanner}
          />
        </div>
        <div className="flex items-center justify-between gap-4">
          <Label htmlFor="inject-worker" className="text-sm font-normal text-muted-foreground">
            {injectWorker ? t("Inject into worker · On") : t("Inject into worker · Off")}
          </Label>
          <Switch id="inject-worker" checked={injectWorker} disabled={!loaded} onCheckedChange={toggleInjectWorker} />
        </div>
      </CardContent>
    </Card>
  );
}

export function ExperimentalCard({
  noaCompaction,
  loaded,
  toggleNoaCompaction,
}: {
  noaCompaction: boolean;
  loaded: boolean;
  toggleNoaCompaction: (v: boolean) => void;
}) {
  return (
    <Card className="mb-4 break-inside-avoid md:mb-6">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <FlaskConicalIcon className="size-4" />
          {t("Experimental features")}
        </CardTitle>
        <CardDescription>
          {t(
            "Mechanisms still being validated, off by default. They may change agent behavior or affect stability; enable only after understanding the impact.",
          )}
          <br />
          {rich(
            t(
              "<b>noa context compaction</b>: the model actively compacts long conversation history (norma v0.4.0). When enabled, the four agent types integrated into the platform (<b>planner / worker / main agent / chat</b>) switch to noa-managed context in place of the built-in compaction, and the pre-compaction originals are archived under the task working directory for later review. Switching takes effect immediately (for runs started afterwards) without rebuilding agents; when disabled, the built-in compaction is restored immediately.",
            ),
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex items-center justify-between gap-4">
        <Label htmlFor="noa-compaction" className="text-sm font-normal text-muted-foreground">
          {noaCompaction ? t("noa context compaction · On") : t("noa context compaction · Off")}
        </Label>
        <Switch id="noa-compaction" checked={noaCompaction} disabled={!loaded} onCheckedChange={toggleNoaCompaction} />
      </CardContent>
    </Card>
  );
}

export function PythonInterpreterCard({
  pyInterp,
  setPyInterp,
  loaded,
  saving,
  detectPython,
  savePython,
}: {
  pyInterp: string;
  setPyInterp: (v: string) => void;
  loaded: boolean;
  saving: boolean;
  detectPython: () => void;
  savePython: () => void;
}) {
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
          <Button variant="outline" onClick={detectPython} disabled={!loaded || saving}>
            {t("Re-detect")}
          </Button>
          <Button onClick={savePython} disabled={!loaded || saving}>
            {t("Save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

export function WorkersCard({
  workers,
  setWorkers,
  loaded,
  savingWorkers,
  saveWorkers,
}: {
  workers: string;
  setWorkers: (v: string) => void;
  loaded: boolean;
  savingWorkers: boolean;
  saveWorkers: () => void;
}) {
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
          <Button onClick={saveWorkers} disabled={!loaded || savingWorkers}>
            {t("Save")}
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
