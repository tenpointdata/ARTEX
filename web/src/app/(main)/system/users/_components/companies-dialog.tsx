"use client";

import * as React from "react";

import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { t } from "@/i18n";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { ConsoleUser } from "@/lib/governance-types";
import type { Company } from "@/lib/types";

interface Props {
  user: ConsoleUser | null;
  companies: Company[];
  bound: number[];
  onClose: () => void;
  onSaved: (userId: number, companyIds: number[]) => void;
}

// CompaniesDialog edits the companies a client_read user is bound to (PUT company_ids). An empty
// selection withdraws every binding, which leaves the account refused everywhere.
export function CompaniesDialog({ user, companies, bound, onClose, onSaved }: Props) {
  const [selected, setSelected] = React.useState<Set<number>>(new Set());
  const [busy, setBusy] = React.useState(false);
  const [error, setError] = React.useState("");

  // bound is intentionally not a dependency: it only seeds the selection when the dialog opens for a user.
  // biome-ignore lint/correctness/useExhaustiveDependencies: see above
  React.useEffect(() => {
    setSelected(new Set(bound));
    setError("");
  }, [user]);

  function toggle(id: number, on: boolean) {
    setSelected((s) => {
      const next = new Set(s);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });
  }

  async function save() {
    if (!user) return;
    setBusy(true);
    setError("");
    try {
      const ids = await governanceApi.setUserCompanies(user.id, [...selected]);
      toast.success(t("Companies updated for {name}", { name: user.username }));
      onSaved(user.id, ids);
      onClose();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={user !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("Bound companies")}</DialogTitle>
          <DialogDescription>
            {t(
              "{name} sees only the reports and findings of the companies checked here. With none checked the account is refused everywhere.",
              {
                name: user?.username ?? "",
              },
            )}
          </DialogDescription>
        </DialogHeader>
        <div className="grid max-h-72 gap-2 overflow-y-auto">
          {companies.length === 0 && <p className="text-sm text-muted-foreground">{t("No companies yet.")}</p>}
          {companies.map((c) => (
            <Label key={c.id} className="flex items-center gap-2 font-normal">
              <Checkbox checked={selected.has(c.id)} onCheckedChange={(v) => toggle(c.id, v === true)} />
              {c.name}
            </Label>
          ))}
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("Cancel")}
          </Button>
          <Button onClick={save} disabled={busy}>
            {t("Save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
