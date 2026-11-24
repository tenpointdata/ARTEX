import { t } from "@/i18n";
import type { FieldDef } from "@/lib/config-form";
import { INCLUDE_EVIDENCE } from "@/lib/integrations";

// Per-kind config fields for ticket trackers (see ticketing/*.go for what each adapter reads).
// Layout and copy live here; which keys are credentials comes from the backend's secret_keys,
// so a key renders masked only when the adapter says it is one.

export const TRACKER_KIND_LABEL: Record<string, string> = {
  github: "GitHub Issues",
  jira: "Jira",
  defectdojo: "DefectDojo",
};

export const TRACKER_FIELDS: Record<string, FieldDef[]> = {
  github: [
    { key: "repo", label: t("Repository"), kind: "text", placeholder: "owner/name" },
    {
      key: "token",
      label: t("Access token"),
      kind: "password",
      help: t("A token that can create and edit issues in the repository"),
    },
    {
      key: "base_url",
      label: t("API base URL"),
      kind: "text",
      placeholder: "https://api.github.com",
      help: t("Only for GitHub Enterprise; leave empty for github.com"),
    },
    {
      key: "labels",
      label: t("Labels"),
      kind: "list",
      placeholder: "security, artex",
      help: t("Added to every issue"),
    },
  ],
  jira: [
    { key: "base_url", label: t("Jira URL"), kind: "text", placeholder: "https://your-org.atlassian.net" },
    { key: "project_key", label: t("Project key"), kind: "text", placeholder: "SEC" },
    {
      key: "email",
      label: t("Account email"),
      kind: "text",
      help: t("Jira Cloud only; leave empty for a Data Center personal access token"),
    },
    { key: "token", label: t("API token"), kind: "password" },
    { key: "issue_type", label: t("Issue type"), kind: "text", placeholder: "Bug" },
    {
      key: "done_transition",
      label: t("Done transition"),
      kind: "text",
      help: t("Transition name used to close an issue"),
    },
    {
      key: "reopen_transition",
      label: t("Reopen transition"),
      kind: "text",
      help: t("Transition name used to reopen an issue"),
    },
  ],
  defectdojo: [
    { key: "base_url", label: t("DefectDojo URL"), kind: "text", placeholder: "https://defectdojo.example.com" },
    { key: "api_key", label: t("API key"), kind: "password" },
    {
      key: "test_id",
      label: t("Test ID"),
      kind: "number",
      help: t("The DefectDojo test that findings are attached to"),
    },
    { key: "found_by", label: t("Found-by test type IDs"), kind: "list", placeholder: "1, 2" },
  ],
};

export const EVIDENCE_FIELD: FieldDef = {
  key: INCLUDE_EVIDENCE,
  label: t("Include evidence in tickets"),
  kind: "switch",
  help: t("Off by default: captured traffic can carry session material; credentials are redacted"),
};

// trackerFields is the full field list a tracker form submits: the adapter's own fields plus the shared option.
export function trackerFields(kind: string): FieldDef[] {
  return [...(TRACKER_FIELDS[kind] ?? []), EVIDENCE_FIELD];
}

export function trackerKindLabel(kind: string): string {
  return TRACKER_KIND_LABEL[kind] ?? kind;
}
