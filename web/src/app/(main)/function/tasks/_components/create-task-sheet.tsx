"use client";

import * as React from "react";

import { ChevronRightIcon, Loader2Icon, PaperclipIcon, PlusIcon, TrashIcon } from "lucide-react";
import { toast } from "sonner";

import { AssetInterceptRulesEditor } from "@/components/asset-intercept-rules-editor";
import { TaskLLMProfileChain } from "@/components/task-llm-profile-chain";
import { TaskTemplateControls } from "@/components/task-template-controls";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import { Spinner } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type {
  AssetInterceptRuleInput,
  Company,
  LLMProfile,
  Task,
  TaskCategory,
  TaskCredentialInput,
} from "@/lib/types";

import { MethodologyPicker } from "../methodology-picker";
import { appendUploads, MAX_SOURCE_TASKS } from "./task-helpers";
import { CategoryPicker, CompanyPicker, SourceTaskPicker } from "./task-pickers";

export function CreateTaskSheet({
  tasks,
  categories,
  onCreated,
  onCategoriesChanged,
}: {
  tasks: Task[];
  categories: TaskCategory[];
  onCreated: () => void;
  onCategoriesChanged: () => void;
}) {
  const [open, setOpen] = React.useState(false);
  const [name, setName] = React.useState("");
  const [categoryID, setCategoryID] = React.useState<number | undefined>(undefined);
  const [description, setDescription] = React.useState("");
  const [goal, setGoal] = React.useState("");
  const [selectedTemplateID, setSelectedTemplateID] = React.useState<number | null>(null);
  const [profiles, setProfiles] = React.useState<LLMProfile[]>([]);
  const [companies, setCompanies] = React.useState<Company[]>([]);
  const [sourceTaskIDs, setSourceTaskIDs] = React.useState<string[]>([]);
  const [companyIDs, setCompanyIDs] = React.useState<number[]>([]);
  const [llmProfileIDs, setLLMProfileIDs] = React.useState<string[]>([]);
  const [llmRotate, setLLMRotate] = React.useState(false);
  const [creating, setCreating] = React.useState(false);
  const [timeoutMin, setTimeoutMin] = React.useState(""); // task-level timeout (minutes); empty/0 = no limit
  const [heartbeatMin, setHeartbeatMin] = React.useState("10"); // planner heartbeat (minutes); default 10, minimum 10 (matching the backend)
  const [seedFirstIntent, setSeedFirstIntent] = React.useState(false); // dispatch a seed intent at creation so the worker starts without waiting for the first planner round; off by default, standard plan-then-execute
  const [coverageEnabled, setCoverageEnabled] = React.useState(true); // asset coverage feature; on by default. Off = no coverage computed/displayed, no scope accumulation, scope-related tools hidden (company links unaffected)
  const [methodologies, setMethodologies] = React.useState<string[]>([]); // chosen methodology ids; empty = automatic
  const [interceptRules, setInterceptRules] = React.useState<AssetInterceptRuleInput[]>([]); // task-level asset intercept rules (this task only, never entering the global table)
  const [credentials, setCredentials] = React.useState<TaskCredentialInput[]>([]); // test accounts declared at creation (#3); an empty password = the agent registers that account and records it
  // Method 1, file upload: files are staged under drafts/<draftId>/uploads/ before the task is created, and their absolute paths are appended to the description.
  const [uploading, setUploading] = React.useState(false);
  const [uploadCount, setUploadCount] = React.useState(0);
  const draftIdRef = React.useRef<string>("");
  const fileInputRef = React.useRef<HTMLInputElement>(null);
  const sheetContentRef = React.useRef<HTMLDivElement>(null);

  // load LLM profiles once for the create-task profile picker.
  React.useEffect(() => {
    api
      .llmProfiles()
      .then(setProfiles)
      .catch(() => setProfiles([]));
    api
      .companies()
      .then(setCompanies)
      .catch(() => setCompanies([]));
  }, []);

  // pickFiles uploads the chosen files into this draft's staging dir and appends their
  // absolute paths to the description; the task's agents open them by path via Read/Bash.
  async function pickFiles(files: FileList | null) {
    if (!files || files.length === 0) return;
    // crypto.randomUUID is only available in secure contexts (https/localhost); degrade when accessed via IP+http.
    if (!draftIdRef.current) {
      draftIdRef.current =
        globalThis.crypto?.randomUUID?.() ?? `d${Date.now().toString(36)}${Math.random().toString(36).slice(2, 10)}`;
    }
    setUploading(true);
    try {
      const r = await api.chatUpload("staging", draftIdRef.current, Array.from(files));
      setDescription((prev) => appendUploads(prev, r.attachments));
      setUploadCount((n) => n + r.attachments.length);
    } catch (e) {
      toast.error(t("Upload failed: {error}", { error: (e as Error).message }));
    } finally {
      setUploading(false);
      if (fileInputRef.current) fileInputRef.current.value = ""; // allow re-picking the same file
    }
  }

  async function createTask() {
    if (!description.trim() || !goal.trim()) {
      toast.error(t("Please fill in the description and goal"));
      return;
    }
    if (sourceTaskIDs.length > MAX_SOURCE_TASKS) {
      toast.error(t("Link up to {count} source tasks", { count: MAX_SOURCE_TASKS }));
      return;
    }
    setCreating(true);
    try {
      const timeoutSec = Math.max(0, Math.floor(Number(timeoutMin) || 0)) * 60;
      const heartbeatSec = Math.max(10, Math.floor(Number(heartbeatMin) || 10)) * 60; // minimum 10min, matching the backend's normalization
      await api.createTask({
        name: name.trim(),
        categoryId: categoryID,
        description: description.trim(),
        goal: goal.trim(),
        llmProfileIds: llmProfileIDs.map(Number),
        llmRotate,
        sourceTaskIds: sourceTaskIDs,
        companyIds: companyIDs,
        timeoutSeconds: timeoutSec,
        seedFirstIntent,
        planHeartbeatSeconds: heartbeatSec,
        coverageEnabled,
        methodologies: coverageEnabled ? methodologies : [],
        interceptRules: interceptRules
          .map((r) => ({ ...r, pattern: r.pattern.trim() }))
          .filter((r) => r.pattern !== ""),
        credentials: credentials
          .map((c) => ({
            domain: c.domain.trim(),
            username: c.username.trim(),
            password: c.password,
            note: c.note?.trim() ?? "",
          }))
          .filter((c) => c.domain !== "" && c.username !== ""),
      });
      toast.success(t("Task created"));
      setName("");
      setCategoryID(undefined);
      setDescription("");
      setGoal("");
      setSelectedTemplateID(null);
      setSourceTaskIDs([]);
      setCompanyIDs([]);
      setLLMProfileIDs([]);
      setLLMRotate(false);
      setTimeoutMin("");
      setHeartbeatMin("10");
      setSeedFirstIntent(false);
      setCoverageEnabled(true);
      setMethodologies([]);
      setInterceptRules([]);
      setCredentials([]);
      setUploadCount(0);
      draftIdRef.current = "";
      setOpen(false);
      onCreated();
    } catch (e) {
      toast.error(t("Create failed: {error}", { error: (e as Error).message }));
    } finally {
      setCreating(false);
    }
  }

  return (
    <Sheet open={open} onOpenChange={setOpen}>
      <SheetTrigger asChild>
        <Button size="sm">
          <PlusIcon /> {t("New task")}
        </Button>
      </SheetTrigger>
      {/* A 45vw-wide right drawer: full-height and scrollable, so long forms are no longer bound by dialog height. Narrow screens fall back to full width.
            The content is a flex column: header/footer fixed, the middle field area flex-1 scrolling independently. */}
      <SheetContent
        ref={sheetContentRef}
        side="right"
        className="w-full! max-w-none! gap-0 p-0 sm:w-[45vw]! sm:max-w-[45vw]!"
      >
        <SheetHeader className="border-b p-6">
          <SheetTitle>{t("New task")}</SheetTitle>
          <SheetDescription>
            {t("Fill in the test target and goal; advanced parameters can be expanded as needed.")}
          </SheetDescription>
        </SheetHeader>

        <div className="flex-1 overflow-y-auto p-6">
          <div className="grid gap-5">
            <TaskTemplateControls
              description={description}
              goal={goal}
              categoryID={categoryID}
              interceptRules={interceptRules}
              selectedTemplateID={selectedTemplateID}
              onSelectedTemplateIDChange={setSelectedTemplateID}
              onApply={(template) => {
                setDescription(template.description);
                setGoal(template.goal);
                setCategoryID(template.category_id ?? undefined);
                setInterceptRules(template.intercept_rules ?? []);
                setUploadCount(0);
              }}
              portalContainer={sheetContentRef}
            />
            <div className="grid gap-2">
              <Label htmlFor="name">{t("Name (optional)")}</Label>
              <Input
                id="name"
                placeholder={t("A recognizable name for the task, e.g. Acme website penetration test")}
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </div>
            <Field>
              <FieldLabel htmlFor="task-category">{t("Task category")}</FieldLabel>
              <CategoryPicker
                categories={categories}
                value={categoryID}
                onValueChange={setCategoryID}
                onCategoryCreated={onCategoriesChanged}
                portalContainer={sheetContentRef}
              />
              <FieldDescription>
                {t(
                  "Optional, single category; used for task list filtering and archiving, never affecting agent execution.",
                )}
              </FieldDescription>
            </Field>
            <div className="grid gap-2">
              <Label htmlFor="description">{t("Description")}</Label>
              <Textarea
                id="description"
                className="min-h-32"
                placeholder={t("Test target and background, e.g. test the example.com site")}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
              {/* Upload files (multi-select): staged under drafts/, with absolute paths appended to the description above so the worker can open them with Read/Bash. */}
              <div className="flex flex-wrap items-center gap-2">
                <input
                  ref={fileInputRef}
                  type="file"
                  multiple
                  className="hidden"
                  onChange={(e) => void pickFiles(e.target.files)}
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => fileInputRef.current?.click()}
                  disabled={uploading}
                >
                  {uploading ? <Loader2Icon className="animate-spin" /> : <PaperclipIcon />}
                  {t("Upload files")}
                </Button>
                <span className="text-muted-foreground text-xs">
                  {uploadCount > 0
                    ? t(
                        "{count} files uploaded; their absolute paths were appended to the end of the description (editable)",
                        {
                          count: uploadCount,
                        },
                      )
                    : t(
                        "Multi-select; after upload, absolute file paths are appended to the description for the worker to open with Read/Bash",
                      )}
                </span>
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="goal">{t("Goal")}</Label>
              <Textarea
                id="goal"
                className="min-h-32"
                placeholder={t("What to achieve, e.g. gain admin panel access or server privileges")}
                value={goal}
                onChange={(e) => setGoal(e.target.value)}
              />
            </div>
            <Field>
              <FieldLabel htmlFor="source-tasks">{t("Linked tasks")}</FieldLabel>
              <SourceTaskPicker
                tasks={tasks}
                value={sourceTaskIDs}
                onValueChange={setSourceTaskIDs}
                portalContainer={sheetContentRef}
              />
              <FieldDescription>
                {t(
                  "Link up to {count} tasks. Live, read-only inheritance of the selected tasks' persistent blackboards, asset scope, and related traffic; the new task writes to its own blackboard.",
                  { count: MAX_SOURCE_TASKS },
                )}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="task-companies">{t("Linked company asset scope")}</FieldLabel>
              <CompanyPicker
                companies={companies}
                value={companyIDs}
                onValueChange={setCompanyIDs}
                portalContainer={sheetContentRef}
              />
              <FieldDescription>
                {t(
                  'On task creation, the selected companies\' current assets are added to "Test assets", and their domains, IPs, CIDRs, ICPs, and company keywords are provided to the agent as scope context; no intents are generated automatically and execution goals are not forced to change.',
                )}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="task-intercept-rules">
                {t("Task-level asset intercept / allow rules (optional)")}
              </FieldLabel>
              <AssetInterceptRulesEditor value={interceptRules} onChange={setInterceptRules} />
              <FieldDescription>
                {t(
                  'Applies to this task only and is not written to global rules. Evaluation order: "Block" rules (including global ones) match first, and a hit forbids testing; with no hit and "Allow" rules configured for this task, an asset must match an allow rule to be permitted, otherwise testing is likewise forbidden; with no allow rules configured, no whitelist is active.',
                )}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="llm-profiles">{t("LLM config chain")}</FieldLabel>
              <TaskLLMProfileChain
                profiles={profiles}
                value={llmProfileIDs}
                rotate={llmRotate}
                onRotateChange={setLLMRotate}
                onValueChange={setLLMProfileIDs}
                inputId="llm-profiles"
                portalContainer={sheetContentRef}
              />
              <FieldDescription>
                {t(
                  "Reorder profiles to choose worker rotation order or failover priority. Saving the chain resets exhausted profiles.",
                )}
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="task-credentials">{t("Test accounts (optional)")}</FieldLabel>
              <div className="grid gap-2">
                {credentials.map((c, i) => (
                  <div key={i} className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
                    <Input
                      placeholder={t("domain (e.g. target.example.com)")}
                      value={c.domain}
                      onChange={(e) =>
                        setCredentials((prev) => prev.map((x, j) => (j === i ? { ...x, domain: e.target.value } : x)))
                      }
                    />
                    <Input
                      placeholder={t("email or username")}
                      value={c.username}
                      onChange={(e) =>
                        setCredentials((prev) => prev.map((x, j) => (j === i ? { ...x, username: e.target.value } : x)))
                      }
                    />
                    <Input
                      type="password"
                      autoComplete="new-password"
                      placeholder={t("password (empty = agent generates)")}
                      value={c.password}
                      onChange={(e) =>
                        setCredentials((prev) => prev.map((x, j) => (j === i ? { ...x, password: e.target.value } : x)))
                      }
                    />
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={t("Remove account")}
                      onClick={() => setCredentials((prev) => prev.filter((_, j) => j !== i))}
                    >
                      <TrashIcon />
                    </Button>
                    <Input
                      className="sm:col-span-4"
                      placeholder={t("note (optional, e.g. program alias account)")}
                      value={c.note ?? ""}
                      onChange={(e) =>
                        setCredentials((prev) => prev.map((x, j) => (j === i ? { ...x, note: e.target.value } : x)))
                      }
                    />
                  </div>
                ))}
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="w-fit"
                  onClick={() =>
                    setCredentials((prev) => [...prev, { domain: "", username: "", password: "", note: "" }])
                  }
                >
                  <PlusIcon className="size-4" /> {t("Add account")}
                </Button>
              </div>
              <FieldDescription>
                {t(
                  "Test accounts reused by the agent for authenticated testing. Leave the password empty to have the agent register that account itself and record the password it chose. Use dedicated test accounts only — anything declared here is sent to the configured LLM provider during the task.",
                )}
              </FieldDescription>
            </Field>

            {/* Advanced parameters are collapsed by default: timeout/heartbeat/first intent; they take space only when expanded, keeping the common path clean. */}
            <Collapsible>
              <CollapsibleTrigger className="group flex w-full items-center gap-2 border-t pt-4 text-sm font-medium">
                <ChevronRightIcon className="text-muted-foreground size-4 transition-transform group-data-[state=open]:rotate-90" />
                {t("Advanced settings")}
                <span className="text-muted-foreground ml-auto text-xs font-normal">
                  {t("Timeout · Heartbeat · First intent")}
                </span>
              </CollapsibleTrigger>
              <CollapsibleContent className="grid gap-5 pt-5">
                <div className="grid gap-2">
                  <Label htmlFor="timeout-min">{t("Task timeout (minutes, optional)")}</Label>
                  <Input
                    id="timeout-min"
                    type="number"
                    min={0}
                    className="w-40"
                    placeholder={t("Empty = no limit")}
                    value={timeoutMin}
                    onChange={(e) => setTimeoutMin(e.target.value)}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t(
                      "When the time is up, a graceful wind-down is triggered (each agent writes back + the planner makes the final verdict) and the task enters the timeout terminal state.",
                    )}
                  </p>
                </div>
                <div className="grid gap-2">
                  <Label htmlFor="heartbeat-min">{t("planner heartbeat (minutes)")}</Label>
                  <Input
                    id="heartbeat-min"
                    type="number"
                    min={10}
                    className="w-40"
                    placeholder={t("Default 10")}
                    value={heartbeatMin}
                    onChange={(e) => setHeartbeatMin(e.target.value)}
                  />
                  <p className="text-muted-foreground text-xs">
                    {t(
                      "When this much time has passed since the last planning round ended or the task started, with no triggers in between, a planning round fires automatically (deadlock backstop + wakes supervision of running workers). Minimum 10 minutes.",
                    )}
                  </p>
                </div>
                <div className="grid gap-2">
                  <label htmlFor="seed-first-intent" className="flex items-center gap-2 text-sm">
                    <Checkbox
                      id="seed-first-intent"
                      checked={seedFirstIntent}
                      onCheckedChange={(v) => setSeedFirstIntent(!!v)}
                    />
                    {t("Dispatch the first intent directly (description + goal)")}
                  </label>
                  <p className="text-muted-foreground text-xs">
                    {t(
                      'When enabled, the "description + goal" is dispatched as one intent right at creation; the worker starts without waiting for the first planning round and the planner takes over afterwards to judge/follow up. Recommended for settings like CTF where one run usually solves it directly; disabled follows the standard plan-then-execute flow.',
                    )}
                  </p>
                </div>
                <div className="grid gap-2">
                  <label htmlFor="coverage-enabled" className="flex items-center gap-2 text-sm">
                    <Checkbox
                      id="coverage-enabled"
                      checked={coverageEnabled}
                      onCheckedChange={(v) => setCoverageEnabled(!!v)}
                    />
                    {t("Asset coverage feature")}
                  </label>
                  <p className="text-muted-foreground text-xs">
                    {t(
                      'On by default: computes and displays test coverage, shows testing progress on the situational graph, and automatically accumulates the test scope. When off, coverage is no longer computed or displayed, the situational graph shows assets without progress, and agents no longer receive scope-related tools. Disabling does not affect "Linked company asset scope".',
                    )}
                  </p>
                </div>
                {coverageEnabled && <MethodologyPicker value={methodologies} onChange={setMethodologies} />}
              </CollapsibleContent>
            </Collapsible>
          </div>
        </div>

        <SheetFooter className="flex-row justify-end gap-2 border-t p-4">
          <SheetClose asChild>
            <Button variant="outline">{t("Cancel")}</Button>
          </SheetClose>
          <Button onClick={createTask} disabled={creating || uploading}>
            {creating && <Spinner data-icon="inline-start" />}
            {creating ? t("Creating") : t("Create")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
