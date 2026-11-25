import { ListFilterIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Separator } from "@/components/ui/separator";
import { Switch } from "@/components/ui/switch";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { Tool } from "@/lib/types";

export function ScopeDialog({
  open,
  onOpenChange,
  loading,
  saving,
  toolGroups,
  enabledTools,
  toggleTool,
  onSave,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  loading: boolean;
  saving: boolean;
  toolGroups: { label: string; tools: Tool[] }[];
  enabledTools: Set<string>;
  toggleTool: (key: string, val: boolean) => void;
  onSave: () => void;
}) {
  const canAdmin = useCan("admin");
  const scopeOpen = open;
  const setScopeOpen = onOpenChange;
  const scopeLoading = loading;
  const scopeSaving = saving;
  const saveScope = onSave;
  return (
    <Dialog open={scopeOpen} onOpenChange={setScopeOpen}>
      <DialogContent
        className="sm:max-w-lg flex flex-col overflow-hidden p-0 gap-0"
        style={{ maxHeight: "min(80vh, 560px)" }}
      >
        <DialogHeader className="shrink-0 border-b px-6 py-4">
          <DialogTitle className="flex items-center gap-2">
            <ListFilterIcon className="h-4 w-4" />
            {t("Interception scope")}
          </DialogTitle>
          <DialogDescription className="text-xs">
            {t("Only tools with interception enabled go through rule matching; all other tools pass through directly")}
          </DialogDescription>
        </DialogHeader>

        <div className="flex-1 min-h-0 overflow-y-auto px-6 py-4 space-y-5">
          {scopeLoading ? (
            <p className="text-sm text-muted-foreground py-4">{t("Loading…")}</p>
          ) : (
            toolGroups.map((group, gi) => (
              <div key={group.label}>
                {gi > 0 && <Separator className="mb-5" />}
                <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider mb-2">
                  {group.label}
                </p>
                <div className="space-y-0.5">
                  {group.tools.map((tool) => (
                    <div key={tool.key} className="flex items-center gap-3 rounded-md px-2 py-1.5 hover:bg-muted/50">
                      <Switch
                        id={`scope-${tool.key}`}
                        checked={enabledTools.has(tool.key)}
                        disabled={!canAdmin}
                        onCheckedChange={(v) => toggleTool(tool.key, v)}
                      />
                      <label htmlFor={`scope-${tool.key}`} className="flex-1 min-w-0 cursor-pointer">
                        <div className="flex items-center gap-1.5">
                          <span className="font-mono text-sm">{tool.key}</span>
                          {tool.kind && tool.kind !== "builtin" && (
                            <Badge variant="outline" className="px-1 py-0 text-[10px]">
                              {tool.kind}
                            </Badge>
                          )}
                        </div>
                        {tool.description && (
                          <p className="text-[11px] text-muted-foreground line-clamp-1">{tool.description}</p>
                        )}
                      </label>
                    </div>
                  ))}
                </div>
              </div>
            ))
          )}
        </div>

        <div className="shrink-0 border-t px-6 py-3 flex justify-end gap-2">
          <Button variant="outline" size="sm" onClick={() => setScopeOpen(false)}>
            {t("Cancel")}
          </Button>
          {canAdmin && (
            <Button size="sm" onClick={saveScope} disabled={scopeSaving || scopeLoading}>
              {scopeSaving ? t("Saving…") : t("Save")}
            </Button>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
