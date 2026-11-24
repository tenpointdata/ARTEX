// Role permissions as the server reports them on GET /api/auth/session
// (server/rbac_policy.go). They are sets, not a ladder. The server enforces
// them on every route; the UI only uses them to hide what a role cannot open.

export const PERMISSIONS = ["client_read", "read", "review", "operate", "admin"] as const;

export type Permission = (typeof PERMISSIONS)[number];

// A UI entry that needs a permission. Entries without `requires` are open to all.
export interface PermissionGated {
  requires?: Permission;
}

// parsePermissions keeps the known permission strings of an untrusted payload.
export function parsePermissions(raw: unknown): Permission[] {
  if (!Array.isArray(raw)) return [];
  return PERMISSIONS.filter((p) => raw.includes(p));
}

// allows is false for gated entries while permissions are still unknown (null),
// so a gated entry never flashes in for a role that cannot use it.
export function allows(held: readonly Permission[] | null, entry: PermissionGated): boolean {
  if (!entry.requires) return true;
  return held?.includes(entry.requires) === true;
}

interface GatedGroup<I extends PermissionGated & { subItems?: readonly PermissionGated[] }> {
  items: readonly I[];
}

// filterGroups drops the entries (and sub-entries) held does not allow, then
// the parents left with no sub-entries and the groups left empty.
export function filterGroups<
  I extends PermissionGated & { subItems?: readonly PermissionGated[] },
  G extends GatedGroup<I>,
>(groups: readonly G[], held: readonly Permission[] | null): G[] {
  const out: G[] = [];
  for (const group of groups) {
    const items: I[] = [];
    for (const item of group.items) {
      if (!allows(held, item)) continue;
      if (!item.subItems) {
        items.push(item);
        continue;
      }
      const subItems = item.subItems.filter((sub) => allows(held, sub));
      if (subItems.length > 0) items.push({ ...item, subItems });
    }
    if (items.length > 0) out.push({ ...group, items });
  }
  return out;
}
