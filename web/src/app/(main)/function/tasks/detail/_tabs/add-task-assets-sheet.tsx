import * as React from "react";

import { PlusIcon } from "lucide-react";
import { toast } from "sonner";

import { ScopeTextEditor } from "@/components/scope-text-editor";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Spinner } from "@/components/ui/spinner";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { parseCompanyScopeText } from "@/lib/company-scope";

export function AddTaskAssetsSheet({
  onAttached,
  onOpenChange,
  open,
  taskId,
}: {
  onAttached: () => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  taskId: string;
}) {
  const [scopeText, setScopeText] = React.useState("");
  const [saving, setSaving] = React.useState(false);
  const parsedScope = React.useMemo(() => parseCompanyScopeText(scopeText), [scopeText]);

  React.useEffect(() => {
    if (!open) return;
    setScopeText("");
  }, [open]);

  const attach = async () => {
    if (parsedScope.rules.length === 0 || parsedScope.errors.length > 0) return;
    setSaving(true);
    try {
      const result = await api.registerTaskAssetScopes(taskId, parsedScope.rules);
      const assetSummary = result.assets_linked + result.assets_existing;
      toast.success(
        t("Registered {scopes} scopes, linked {assets} domain/IP assets", {
          scopes: result.requested,
          assets: assetSummary,
        }),
      );
      onAttached();
      onOpenChange(false);
    } catch (reason) {
      toast.error(t("Failed to add: {error}", { error: String((reason as Error)?.message ?? reason) }));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-xl">
        <SheetHeader>
          <SheetTitle>{t("Add test assets")}</SheetTitle>
          <SheetDescription>
            {t(
              "Enter the test scope directly. Domains and IPs create or reuse global assets; CIDRs, ICP records, and keywords act as agent scope context.",
            )}
          </SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-4">
          <ScopeTextEditor
            id="task-asset-scope"
            value={scopeText}
            onValueChange={setScopeText}
            parsed={parsedScope}
            label={t("Test assets and scope")}
            description={t(
              "One entry per line; domains, IPs, CIDRs, ICP records, and keywords are recognized automatically.",
            )}
          />
        </div>
        <SheetFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            {t("Cancel")}
          </Button>
          <Button
            onClick={() => void attach()}
            disabled={saving || parsedScope.rules.length === 0 || parsedScope.errors.length > 0}
          >
            {saving ? <Spinner data-icon="inline-start" /> : <PlusIcon data-icon="inline-start" />}
            {parsedScope.rules.length > 0
              ? t("Register {count} entries", { count: parsedScope.rules.length })
              : t("Register entries")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
