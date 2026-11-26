// Route modules are tried in order; the first one that returns something other than NO_ROUTE wins.
export const NO_ROUTE: unique symbol = Symbol("no-route");

export interface RouteCtx {
  m: string;
  path: string;
  seg: string[];
  q: URLSearchParams;
  b: Record<string, unknown>;
}
