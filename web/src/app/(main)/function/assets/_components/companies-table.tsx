"use client";

import { Trash2Icon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import type { Company } from "@/lib/types";

import { AppendScopeDialog, CompanyAvatar, EditScopeDialog } from "./company-dialogs";

export function CompaniesTable({
  companies,
  onSaved,
  onDelete,
}: {
  companies: Company[];
  onSaved: () => void;
  onDelete: (c: Company) => void;
}) {
  return (
    <Card className="flex min-h-0 flex-1 flex-col overflow-hidden py-0">
      <div className="min-h-0 flex-1 overflow-auto">
        <Table>
          <TableHeader className="sticky top-0 z-10 bg-card">
            <TableRow>
              <TableHead>{t("Company")}</TableHead>
              <TableHead className="w-24 text-right">{t("Assets")}</TableHead>
              <TableHead>{t("Asset scope")}</TableHead>
              <TableHead className="w-36 text-right">{t("Actions")}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {companies.map((c) => (
              <TableRow key={c.id}>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <CompanyAvatar name={c.name} logo={c.logo} />
                    <span className="font-medium">{c.name}</span>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums text-sm">{c.asset_count}</TableCell>
                <TableCell>
                  {c.scope?.length ? (
                    <div className="flex flex-wrap gap-1">
                      {c.scope.map((s, i) => (
                        <Badge key={i} variant="secondary" className="font-mono text-[11px]">
                          {s.raw}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-xs text-muted-foreground">{t("No scope set")}</span>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-1.5">
                    <EditScopeDialog company={c} onSaved={onSaved} />
                    <AppendScopeDialog company={c} onSaved={onSaved} />
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-7 text-muted-foreground hover:text-destructive"
                      onClick={() => onDelete(c)}
                      aria-label={t("Delete company {name}", { name: c.name })}
                    >
                      <Trash2Icon className="size-3.5" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {companies.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="py-10 text-center text-sm text-muted-foreground">
                  {t(
                    'No companies yet. Click "Add company" in the top-right corner and fill in the asset scope; the system will automatically claim matching assets.',
                  )}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </Card>
  );
}
