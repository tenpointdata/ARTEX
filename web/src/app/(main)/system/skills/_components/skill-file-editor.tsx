"use client";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";

export function SkillFileEditor({
  skill,
  path,
  content,
  loading,
  dirty,
  saving,
  onChange,
  onSave,
}: {
  skill: string;
  path: string;
  content: string;
  loading: boolean;
  dirty: boolean;
  saving: boolean;
  onChange: (value: string) => void;
  onSave: () => void;
}) {
  const canOperate = useCan("operate"); // PUT /api/skills file writes
  return (
    <div className="flex h-full flex-col gap-2">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{skill}</span>
        <span>/</span>
        <span className="font-mono">{path}</span>
        {canOperate && (
          <Button size="sm" className="ml-auto" onClick={onSave} disabled={!dirty || saving}>
            {saving ? t("Saving…") : t("Save")}
          </Button>
        )}
      </div>
      {loading ? (
        <p className="text-xs text-muted-foreground">{t("Loading…")}</p>
      ) : (
        <Textarea
          className="flex-1 resize-none font-mono text-xs"
          value={content}
          readOnly={!canOperate}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </div>
  );
}
