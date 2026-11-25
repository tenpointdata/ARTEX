// Pure route-permission resolution for the console's route guard. No "@/"
// imports, so node --test can load this file directly (route-access.test.mjs).

import type { Permission } from "./governance-types";

// A route the console exposes and the permission it needs. Unset need means
// "read" (server/rbac_policy.go: a GET needs read by default).
export interface RouteRule {
  url: string;
  need?: Permission;
}

// A nav tree shaped like the sidebar's groups: items (a parent item may have
// no url of its own), optionally with sub-items.
export interface RouteRuleGroup {
  items: ReadonlyArray<{ url?: string; need?: Permission; subItems?: readonly RouteRule[] }>;
}

function normalize(path: string): string {
  let p = path;
  try {
    p = decodeURIComponent(path);
  } catch {
    // keep the raw path
  }
  // Static export enables trailingSlash; "/function/tasks/" is "/function/tasks".
  const trimmed = p.split(/[?#]/)[0].replace(/\/+$/, "");
  return trimmed === "" ? "/" : trimmed;
}

// flattenRoutes lists every item and sub-item of a nav tree as a route rule.
export function flattenRoutes(groups: readonly RouteRuleGroup[]): RouteRule[] {
  const out: RouteRule[] = [];
  for (const g of groups) {
    for (const it of g.items) {
      if (it.url !== undefined) out.push({ url: it.url, need: it.need });
      for (const sub of it.subItems ?? []) out.push({ url: sub.url, need: sub.need });
    }
  }
  return out;
}

// routeNeed is the permission a pathname needs: that of the longest rule whose
// url is the path or a parent of it (so "/function/tasks/detail" inherits
// "/function/tasks"). A path no rule covers needs "read".
export function routeNeed(rules: readonly RouteRule[], pathname: string): Permission {
  const path = normalize(pathname);
  let best: RouteRule | null = null;
  for (const r of rules) {
    const url = normalize(r.url);
    const covers = path === url || (url !== "/" && path.startsWith(`${url}/`));
    if (covers && (best === null || url.length > normalize(best.url).length)) best = r;
  }
  return best?.need ?? "read";
}
