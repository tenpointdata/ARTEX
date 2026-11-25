"use client";

import * as React from "react";

import { usePathname, useRouter } from "next/navigation";

import { useSession } from "@/hooks/use-session";
import { can, landingPath } from "@/lib/governance";
import { flattenRoutes, routeNeed } from "@/lib/route-access";
import { sidebarItems } from "@/navigation/sidebar/sidebar-items";

const ROUTES = flattenRoutes(sidebarItems);

// RouteGuard mounts a page only when the session holds the permission its
// route needs (the sidebar's `need`, inherited by detail pages); otherwise it
// sends the session to its landing page without mounting the page, so none of
// the page's requests fire. The layout renders it only after the session has
// loaded. The API enforces the same policy regardless (server/rbac_policy.go).
export function RouteGuard({ children }: { children: React.ReactNode }) {
  const session = useSession();
  const pathname = usePathname();
  const router = useRouter();
  const allowed = can(session, routeNeed(ROUTES, pathname));
  const home = landingPath(session);

  React.useEffect(() => {
    if (!allowed) router.replace(home);
  }, [allowed, home, router]);

  return allowed ? <>{children}</> : null;
}
