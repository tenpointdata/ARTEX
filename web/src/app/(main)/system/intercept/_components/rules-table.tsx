import { PencilIcon, PlusIcon, ShieldAlertIcon, Trash2Icon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import type { InterceptRule } from "@/lib/types";

import { ActionBadge, argvLabel } from "./shared";

export function RulesTable({
  rules,
  loading,
  openNew,
  openEdit,
  handleDelete,
  handleToggle,
}: {
  rules: InterceptRule[];
  loading: boolean;
  openNew: () => void;
  openEdit: (rule: InterceptRule) => void;
  handleDelete: (id: number) => void;
  handleToggle: (rule: InterceptRule) => void;
}) {
  const canAdmin = useCan("admin");
  return (
    <Card>
      <CardContent className="p-0">
        {loading ? (
          <p className="p-6 text-sm text-muted-foreground">{t("Loading…")}</p>
        ) : rules.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-2 py-16 text-center">
            <ShieldAlertIcon className="h-8 w-8 text-muted-foreground/40" />
            <p className="text-sm text-muted-foreground">{t("No rules yet")}</p>
            {canAdmin && (
              <Button size="sm" variant="outline" onClick={openNew}>
                <PlusIcon className="h-4 w-4" />
                {t("Create the first rule")}
              </Button>
            )}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent">
                <TableHead className="w-[72px]">{t("Priority")}</TableHead>
                <TableHead>{t("Name")}</TableHead>
                <TableHead className="w-[90px]">{t("Target")}</TableHead>
                <TableHead className="w-[80px]">{t("Type")}</TableHead>
                <TableHead>{t("Pattern")}</TableHead>
                <TableHead className="w-[72px]">{t("Policy")}</TableHead>
                <TableHead className="w-[64px] text-center">{t("Enabled")}</TableHead>
                <TableHead className="w-[80px]" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {rules.map((rule) => (
                <TableRow key={rule.id} className={!rule.enabled ? "opacity-40" : ""}>
                  <TableCell>
                    <span className="font-mono text-xs tabular-nums">{rule.priority}</span>
                  </TableCell>
                  <TableCell className="font-medium text-sm">{rule.name}</TableCell>
                  <TableCell>
                    <span className="text-xs text-muted-foreground">
                      {rule.match_target === "tool_name" ? t("Tool name") : t("Input content")}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span className="text-xs text-muted-foreground">
                      {rule.match_type === "regex"
                        ? t("Regex")
                        : rule.match_type === "argv"
                          ? t("Argv prefix")
                          : t("String")}
                    </span>
                  </TableCell>
                  <TableCell className="max-w-[220px]">
                    <code className="block truncate rounded bg-muted px-1.5 py-0.5 text-xs font-mono">
                      {rule.match_type === "argv" ? argvLabel(rule.pattern) : rule.pattern}
                    </code>
                  </TableCell>
                  <TableCell>
                    <ActionBadge action={rule.action} />
                  </TableCell>
                  <TableCell className="text-center">
                    <Switch checked={rule.enabled} disabled={!canAdmin} onCheckedChange={() => handleToggle(rule)} />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center justify-end gap-0.5">
                      <Button size="icon" variant="ghost" className="h-7 w-7" onClick={() => openEdit(rule)}>
                        <PencilIcon className="h-3.5 w-3.5" />
                      </Button>
                      {canAdmin && (
                        <Button
                          size="icon"
                          variant="ghost"
                          className="h-7 w-7 text-destructive hover:text-destructive"
                          onClick={() => handleDelete(rule.id)}
                        >
                          <Trash2Icon className="h-3.5 w-3.5" />
                        </Button>
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  );
}
