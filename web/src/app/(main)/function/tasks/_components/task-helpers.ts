import { t } from "@/i18n";
import type { SortDirection } from "@/lib/sort-preference";
import type { ChatAttachment, DeleteTaskResult, Task, TaskStatus } from "@/lib/types";

// fmtBytes renders a human file size for the upload manifest.
export function fmtBytes(n: number): string {
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

// UPLOAD_MARKER labels the auto-appended block of uploaded-file paths inside the task
// description, so re-uploads append under the same block instead of adding a new header.
export const UPLOAD_MARKER = "[Uploaded Files (Absolute Paths)]";

// appendUploads folds newly-uploaded files' ABSOLUTE paths into the description as a
// Read/Bash-friendly manifest — the worker opens them by path. Keeps one marked block:
// first upload adds the header, later uploads append bullets under it.
export function appendUploads(desc: string, atts: ChatAttachment[]): string {
  const bullets = atts.map((a) => `- ${a.abs ?? a.path} (${fmtBytes(a.size)})`).join("\n");
  if (desc.includes(UPLOAD_MARKER)) {
    return `${desc.replace(/\s*$/, "")}\n${bullets}\n`;
  }
  const head = desc.trim() ? `${desc.replace(/\s*$/, "")}\n\n` : "";
  return `${head}${UPLOAD_MARKER} worker can open them by path with Read/Bash:\n${bullets}\n`;
}

// POLL_MS is the task-list refresh interval. Task state moves on the server (planner /
// worker), so the list has to be pulled; 10s is plenty for status / progress / token changes.
export const POLL_MS = 10_000;
export const MAX_SOURCE_TASKS = 8;

// fmtTokens renders a compact token count (1234 → 1.2k, 2_000_000 → 2M).
export function fmtTokens(n: number): string {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1) + "M";
  if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + "k";
  return String(n);
}

// fmtDuration renders a run duration in seconds as a compact human string.
export function fmtDuration(sec: number): string {
  if (sec <= 0) return "—";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  const s = sec % 60;
  if (d > 0) return t("{d}d {h}h", { d, h });
  if (h > 0) return t("{h}h {m}m", { h, m });
  if (m > 0) return t("{m}m {s}s", { m, s });
  return t("{s}s", { s });
}

// taskDuration is the task's run duration in seconds: created → now while running,
// created → completion for a finished task, else created → last activity. 0 when it
// never ran (no activity yet).
export function taskDuration(task: Task, nowSec: number): number {
  const start = task.created_unix ?? 0;
  if (!start) return 0;
  const end =
    task.status === "running"
      ? nowSec
      : task.completed_unix && task.completed_unix > 0
        ? task.completed_unix
        : (task.last_activity_unix ?? 0);
  return end > start ? end - start : 0;
}

// deleteDetails takes only the countable part of the result so callers can pass an
// aggregate accumulated over a bulk delete.
export type DeleteCounts = Omit<DeleteTaskResult, "deleted" | "cleanup_warning">;

export function deleteDetails(result: DeleteCounts): string[] {
  const details: string[] = [];
  if (result.assets_deleted > 0) details.push(t("Deleted {count} assets", { count: result.assets_deleted }));
  if (result.assets_detached > 0) details.push(t("Detached {count} shared assets", { count: result.assets_detached }));
  if (result.traffic_deleted > 0) details.push(t("Deleted {count} traffic records", { count: result.traffic_deleted }));
  if (result.files_deleted) details.push(t("Deleted task files"));
  if (result.findings_deleted > 0) details.push(t("Deleted {count} findings", { count: result.findings_deleted }));
  if (result.llm_records_deleted > 0)
    details.push(t("Deleted {count} LLM request/response records", { count: result.llm_records_deleted }));
  return details;
}

export function deleteSummary(result: DeleteTaskResult): string {
  const details = deleteDetails(result);
  return details.length > 0 ? t("Task deleted ({details})", { details: details.join(", ") }) : t("Task deleted");
}

// fmtDateTime renders a unix-seconds timestamp as a compact local date-time
// (MM-DD HH:mm), or "—" when unset.
export function fmtDateTime(unix?: number): string {
  if (!unix || unix <= 0) return "—";
  const d = new Date(unix * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export const STATUS_OPTIONS: { value: TaskStatus; label: string }[] = [
  { value: "created", label: t("Created") },
  { value: "queued", label: t("Queued") },
  { value: "running", label: t("Running") },
  { value: "paused", label: t("Paused") },
  { value: "done", label: t("Completed") },
  { value: "failed", label: t("Failed") },
  { value: "timeout", label: t("Timed out") },
];

// Select does not accept an empty-string value, so "no category" in the filter, the create form, and the bulk move
// all use this sentinel value, which is translated back to the backend's null on submit.
export const UNCATEGORIZED_VALUE = "uncategorized";

// Pausable = non-terminal and not paused, matching the backend's applyTaskControlWithCause gating
// (done/failed/timeout are terminal); only paused can be resumed. The inline buttons and the bulk controls share this check,
// so the two sides can never disagree about one being clickable and the other not.
export const PAUSABLE_STATUSES = new Set<TaskStatus>(["created", "queued", "running"]);
export const ARCHIVABLE_STATUSES = new Set<TaskStatus>(["paused", "done", "failed", "timeout"]);

export function taskControlAction(status: TaskStatus): "pause" | "resume" | null {
  if (status === "paused") return "resume";
  if (PAUSABLE_STATUSES.has(status)) return "pause";
  return null;
}

export type TaskSortField = "id" | "created" | "duration" | "status";

export const TASK_SORT_FIELDS: readonly TaskSortField[] = ["id", "created", "duration", "status"];
export const TASK_SORT_PREFERENCE_KEY = "artex_task_list_sort";
export const TASK_FILTER_PREFERENCE_KEY = "artex_task_list_filters";

export const TASK_STATUS_RANK = new Map(STATUS_OPTIONS.map((option, index) => [option.value, index]));

export function taskIsPinned(task: Task): boolean {
  return task.pinned ?? Boolean(task.pinned_at);
}

export function compareTaskIDs(left: string, right: string): number {
  return left.localeCompare(right, undefined, { numeric: true, sensitivity: "base" });
}

export function taskCreatedUnix(task: Task): number {
  if (task.created_unix) return task.created_unix;
  const parsed = Date.parse(task.created_at);
  return Number.isNaN(parsed) ? 0 : Math.floor(parsed / 1000);
}

export function compareTasks(
  left: Task,
  right: Task,
  field: TaskSortField,
  direction: SortDirection,
  nowSec: number,
): number {
  const leftPinned = taskIsPinned(left);
  const rightPinned = taskIsPinned(right);
  if (leftPinned !== rightPinned) return leftPinned ? -1 : 1;
  if (leftPinned && left.pinned_at !== right.pinned_at) {
    return (right.pinned_at ?? "").localeCompare(left.pinned_at ?? "");
  }

  let compared = 0;
  switch (field) {
    case "created":
      compared = taskCreatedUnix(left) - taskCreatedUnix(right);
      break;
    case "duration":
      compared = taskDuration(left, nowSec) - taskDuration(right, nowSec);
      break;
    case "status":
      compared = (TASK_STATUS_RANK.get(left.status) ?? 0) - (TASK_STATUS_RANK.get(right.status) ?? 0);
      break;
    default:
      compared = compareTaskIDs(left.id, right.id);
  }
  if (compared !== 0) return direction === "asc" ? compared : -compared;
  return -compareTaskIDs(left.id, right.id);
}
