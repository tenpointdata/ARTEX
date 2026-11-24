"use client";

import * as React from "react";

import { PlusIcon } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Table, TableBody, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCan, useSession } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import { errorText } from "@/lib/api-errors";
import { governanceApi } from "@/lib/governance-api";
import type { ConsoleUser, Role, UserPatch } from "@/lib/governance-types";
import type { Company } from "@/lib/types";

import { AddUserDialog } from "./_components/add-user-dialog";
import { CompaniesDialog } from "./_components/companies-dialog";
import { PasswordDialog } from "./_components/password-dialog";
import { UserRow } from "./_components/user-row";

// Admin-only console user management. The gate is outside UsersAdmin so a non-admin makes no API call.
export default function UsersPage() {
  const isAdmin = useCan("admin");
  return (
    <div className="space-y-4 p-6">
      <div>
        <h1 className="text-xl font-semibold">{t("Users")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {t("Console logins and their roles. The bootstrap ARTEX login is always an admin and is not listed here.")}
        </p>
      </div>
      {isAdmin ? (
        <UsersAdmin />
      ) : (
        <p className="rounded-md border p-4 text-sm text-muted-foreground">{t("Only admins can manage users")}</p>
      )}
    </div>
  );
}

function UsersAdmin() {
  const { username } = useSession();
  const [users, setUsers] = React.useState<ConsoleUser[] | null>(null);
  const [companies, setCompanies] = React.useState<Company[]>([]);
  const [bindings, setBindings] = React.useState<Record<number, number[]>>({});
  const [loadError, setLoadError] = React.useState("");
  const [adding, setAdding] = React.useState(false);
  const [pwUser, setPwUser] = React.useState<ConsoleUser | null>(null);
  const [coUser, setCoUser] = React.useState<ConsoleUser | null>(null);

  const loadBindings = React.useCallback(async (list: ConsoleUser[]) => {
    const clients = list.filter((u) => u.role === "client_read");
    const pairs = await Promise.all(
      clients.map(async (u) => [u.id, await governanceApi.userCompanies(u.id).catch(() => undefined)] as const),
    );
    setBindings((prev) => {
      const next = { ...prev };
      for (const [id, ids] of pairs) if (ids) next[id] = ids;
      return next;
    });
  }, []);

  const load = React.useCallback(async () => {
    setLoadError("");
    try {
      const [res, cos] = await Promise.all([governanceApi.users(), api.companies().catch(() => [] as Company[])]);
      setUsers(res.users);
      setCompanies(cos);
      await loadBindings(res.users);
    } catch (err) {
      setLoadError(errorText(err));
      setUsers((u) => u ?? []);
    }
  }, [loadBindings]);

  React.useEffect(() => {
    void load();
  }, [load]);

  const replace = React.useCallback((u: ConsoleUser) => {
    setUsers((list) => (list ?? []).map((x) => (x.id === u.id ? u : x)));
  }, []);

  async function patch(u: ConsoleUser, p: UserPatch) {
    try {
      const next = await governanceApi.updateUser(u.id, p);
      replace(next);
      if (p.role === "client_read") void loadBindings([next]);
    } catch (err) {
      toast.error(errorText(err));
    }
  }

  async function remove(u: ConsoleUser) {
    try {
      await governanceApi.deleteUser(u.id);
      setUsers((list) => (list ?? []).filter((x) => x.id !== u.id));
      toast.success(t("User {name} deleted", { name: u.username }));
    } catch (err) {
      toast.error(errorText(err));
    }
  }

  const companyName = React.useMemo(() => new Map(companies.map((c) => [c.id, c.name])), [companies]);
  const namesFor = (u: ConsoleUser): string[] | undefined =>
    bindings[u.id]?.map((id) => companyName.get(id) ?? t("Company #{id}", { id }));

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm text-muted-foreground">
          {t(
            "A client (read-only) account sees only the companies it is bound to; an unbound one is refused everywhere.",
          )}
        </p>
        <Button onClick={() => setAdding(true)}>
          <PlusIcon />
          {t("Add user")}
        </Button>
      </div>
      {loadError && (
        <p role="alert" className="text-sm text-destructive">
          {t("Failed to load users: {error}", { error: loadError })}
        </p>
      )}
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{t("Username")}</TableHead>
            <TableHead>{t("Role")}</TableHead>
            <TableHead>{t("Status")}</TableHead>
            <TableHead>{t("Companies")}</TableHead>
            <TableHead>{t("Created by")}</TableHead>
            <TableHead>{t("Created")}</TableHead>
            <TableHead>{t("Updated")}</TableHead>
            <TableHead className="text-right">{t("Actions")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {(users ?? []).map((u) => (
            <UserRow
              key={u.id}
              user={u}
              isSelf={u.username === username}
              companyNames={namesFor(u)}
              onRole={(x, role: Role) => void patch(x, { role })}
              onDisabled={(x, disabled) => void patch(x, { disabled })}
              onPassword={setPwUser}
              onCompanies={setCoUser}
              onDelete={(x) => void remove(x)}
            />
          ))}
        </TableBody>
      </Table>
      {users !== null && users.length === 0 && !loadError && (
        <p className="py-6 text-center text-sm text-muted-foreground">{t("No users yet.")}</p>
      )}
      <AddUserDialog open={adding} onOpenChange={setAdding} onCreated={() => void load()} />
      <PasswordDialog user={pwUser} onClose={() => setPwUser(null)} onSaved={replace} />
      <CompaniesDialog
        user={coUser}
        companies={companies}
        bound={coUser ? (bindings[coUser.id] ?? []) : []}
        onClose={() => setCoUser(null)}
        onSaved={(id, ids) => setBindings((b) => ({ ...b, [id]: ids }))}
      />
    </div>
  );
}
