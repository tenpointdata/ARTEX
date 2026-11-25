// Mock routes: map (method, path) to the static data in lib/mock/data.
// Unmatched routes always return safe defaults ([] / {} / {ok:true}) so no page ever crashes.
// Only entered via the http() short-circuit in api.ts when NEXT_PUBLIC_MOCK=1.

import { advanceMockRetests } from "./handler/evidence";
import { NO_ROUTE, type RouteCtx } from "./handler/route-types";
import { routeAuthArchives } from "./handler/routes-auth-archives";
import { routeExploration } from "./handler/routes-exploration";
import { routeIntercept } from "./handler/routes-intercept";
import { routeScopeAssets } from "./handler/routes-scope-assets";
import { routeSystem } from "./handler/routes-system";
import { routeTasks } from "./handler/routes-tasks";
import { delay } from "./handler/state";
import { parseBody } from "./handler/task-helpers";

export async function mockHandle<T>(method: string, rawPath: string, body?: BodyInit | null): Promise<T> {
  advanceMockRetests();
  await delay();
  const [path, qs] = rawPath.split("?");
  const q = new URLSearchParams(qs ?? "");
  const seg = path.split("/").filter(Boolean); // ["exploration","activity"]
  const m = method.toUpperCase();
  const b = parseBody(body);
  return route({ m, path, seg, q, b }) as T;
}

const ROUTE_MODULES = [routeAuthArchives, routeTasks, routeScopeAssets, routeExploration, routeSystem, routeIntercept];

function route(ctx: RouteCtx): unknown {
  for (const handle of ROUTE_MODULES) {
    const result = handle(ctx);
    if (result !== NO_ROUTE) return result;
  }
  const { m, path } = ctx;

  // ── Write fallback: succeed without persisting ──
  if (["POST", "PUT", "PATCH", "DELETE"].includes(m)) return { ok: true };

  // ── Read fallback: [] for collections, {} for the rest ──
  return /(\/(tasks|profiles|conversations|rules|history|projects|tokens|agents|servers|skills|tools|findings|intents)s?$)|s$/.test(
    path,
  )
    ? []
    : {};
}
