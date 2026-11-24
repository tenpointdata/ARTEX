"use client";

import * as React from "react";

import { KeyRoundIcon, Trash2Icon, UsersIcon } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { TableCell, TableRow } from "@/components/ui/table";
import { t } from "@/i18n";
import { type ConsoleUser, ROLES, type Role } from "@/lib/governance-types";

import { roleLabel } from "./role-label";

export interface UserRowProps {
  user: ConsoleUser;
  isSelf: boolean;
  // Names of the bound companies; undefined while not loaded. Only meaningful for client_read.
  companyNames: string[] | undefined;
  onRole: (u: ConsoleUser, role: Role) => void;
  onDisabled: (u: ConsoleUser, disabled: boolean) => void;
  onPassword: (u: ConsoleUser) => void;
  onCompanies: (u: ConsoleUser) => void;
  onDelete: (u: ConsoleUser) => void;
}

function when(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

// UserRow is one console user. The signed-in user's own row keeps no demote, disable or delete
// control: the server refuses them (403), so offering them would only produce an error.
export function UserRow({
  user,
  isSelf,
  companyNames,
  onRole,
  onDisabled,
  onPassword,
  onCompanies,
  onDelete,
}: UserRowProps) {
  const [confirmDelete, setConfirmDelete] = React.useState(false);
  const unbound = user.role === "client_read" && companyNames !== undefined && companyNames.length === 0;

  return (
    <TableRow>
      <TableCell className="font-medium">
        {user.username}
        {isSelf && (
          <Badge variant="secondary" className="ml-2">
            {t("you")}
          </Badge>
        )}
      </TableCell>
      <TableCell>
        {isSelf ? (
          roleLabel(user.role)
        ) : (
          <Select value={user.role} onValueChange={(v) => onRole(user, v as Role)}>
            <SelectTrigger size="sm" className="w-40" aria-label={t("Role")}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ROLES.map((r) => (
                <SelectItem key={r} value={r}>
                  {roleLabel(r)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </TableCell>
      <TableCell>
        <div className="flex items-center gap-2">
          <Switch
            checked={!user.disabled}
            disabled={isSelf}
            onCheckedChange={(on) => onDisabled(user, !on)}
            aria-label={t("Enabled")}
          />
          <span className="text-muted-foreground">{user.disabled ? t("Disabled") : t("Enabled")}</span>
        </div>
      </TableCell>
      <TableCell>
        {user.role === "client_read" ? (
          <div className="flex flex-wrap items-center gap-1">
            {companyNames === undefined && <span className="text-muted-foreground">…</span>}
            {companyNames?.map((n) => (
              <Badge key={n} variant="outline">
                {n}
              </Badge>
            ))}
            {unbound && (
              <Badge variant="destructive" title={t("An unbound client account is refused everywhere.")}>
                {t("Unbound")}
              </Badge>
            )}
            <Button variant="ghost" size="icon-sm" onClick={() => onCompanies(user)} title={t("Edit companies")}>
              <UsersIcon />
              <span className="sr-only">{t("Edit companies")}</span>
            </Button>
          </div>
        ) : (
          <span className="text-muted-foreground">{t("All")}</span>
        )}
      </TableCell>
      <TableCell className="text-muted-foreground">{user.created_by || "-"}</TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground">{when(user.created_at)}</TableCell>
      <TableCell className="whitespace-nowrap text-muted-foreground">{when(user.updated_at)}</TableCell>
      <TableCell>
        <div className="flex justify-end gap-1">
          <Button variant="ghost" size="sm" onClick={() => onPassword(user)}>
            <KeyRoundIcon />
            {t("Set password")}
          </Button>
          {!isSelf && (
            <Button variant="ghost" size="icon-sm" onClick={() => setConfirmDelete(true)} title={t("Delete")}>
              <Trash2Icon className="text-destructive" />
              <span className="sr-only">{t("Delete")}</span>
            </Button>
          )}
        </div>
        <AlertDialog open={confirmDelete} onOpenChange={setConfirmDelete}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t("Delete user {name}?", { name: user.username })}</AlertDialogTitle>
              <AlertDialogDescription>
                {t("The login is removed and its sessions stop working. This cannot be undone.")}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t("Cancel")}</AlertDialogCancel>
              <AlertDialogAction variant="destructive" onClick={() => onDelete(user)}>
                {t("Delete")}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </TableCell>
    </TableRow>
  );
}
