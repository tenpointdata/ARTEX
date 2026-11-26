import * as React from "react";

// A text input whose trimmed value reaches the server only after the user pauses,
// so typing does not refetch on every keystroke. `clear` resets both at once.
export function useDebouncedInput(delay = 300) {
  const [value, setValue] = React.useState("");
  const [debounced, setDebounced] = React.useState("");
  React.useEffect(() => {
    const timer = setTimeout(() => setDebounced(value.trim()), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  const clear = React.useCallback(() => {
    setValue("");
    setDebounced("");
  }, []);
  return { value, setValue, debounced, clear };
}

// The server-side filter set shared by the list query and the HAR export.
export interface TrafficFilterQuery {
  host: string;
  method: string;
  q: string;
  body: string;
  path: string;
  status: string;
  rewritten: string;
  respMin: string;
  respMax: string;
}

// Filter state for the exchange list: text inputs are debounced, selects apply
// immediately. `query` is memoized so effects can depend on it directly.
export function useTrafficFilters() {
  const host = useDebouncedInput();
  const search = useDebouncedInput();
  const body = useDebouncedInput();
  const path = useDebouncedInput();
  const respMin = useDebouncedInput();
  const respMax = useDebouncedInput();
  const [method, setMethod] = React.useState("all");
  const [status, setStatus] = React.useState("all");
  const [rewritten, setRewritten] = React.useState("all");

  const query = React.useMemo<TrafficFilterQuery>(
    () => ({
      host: host.debounced,
      method,
      q: search.debounced,
      body: body.debounced,
      path: path.debounced,
      status,
      rewritten,
      respMin: respMin.debounced,
      respMax: respMax.debounced,
    }),
    [
      host.debounced,
      method,
      search.debounced,
      body.debounced,
      path.debounced,
      status,
      rewritten,
      respMin.debounced,
      respMax.debounced,
    ],
  );

  const hasAdvancedFilter =
    Boolean(body.debounced || path.debounced || respMin.debounced || respMax.debounced) ||
    status !== "all" ||
    rewritten !== "all";
  const { clear: clearBody } = body;
  const { clear: clearPath } = path;
  const { clear: clearRespMin } = respMin;
  const { clear: clearRespMax } = respMax;
  const resetAdvanced = React.useCallback(() => {
    clearBody();
    clearPath();
    clearRespMin();
    clearRespMax();
    setStatus("all");
    setRewritten("all");
  }, [clearBody, clearPath, clearRespMin, clearRespMax]);

  return {
    host,
    search,
    body,
    path,
    respMin,
    respMax,
    method,
    setMethod,
    status,
    setStatus,
    rewritten,
    setRewritten,
    query,
    hasAdvancedFilter,
    resetAdvanced,
  };
}

export type TrafficFilters = ReturnType<typeof useTrafficFilters>;
