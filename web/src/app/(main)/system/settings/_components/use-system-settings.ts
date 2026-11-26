"use client";

import * as React from "react";

import { toast } from "sonner";

import { useCan } from "@/hooks/use-session";
import { t } from "@/i18n";
import { api } from "@/lib/api";
import type { Settings } from "@/lib/types";

// useSettingsAccess: every save on the settings page is PUT /api/settings, which is admin-only
// (server/rbac_policy.go). Other roles see the values read-only: controls disabled, Save buttons hidden.
export function useSettingsAccess(loaded: boolean): { canAdmin: boolean; editable: boolean } {
  const canAdmin = useCan("admin");
  return { canAdmin, editable: loaded && canAdmin };
}

export function useSystemSettings() {
  const [trafficCapture, setTrafficCapture] = React.useState(false);
  const [agentTrafficBinding, setAgentTrafficBinding] = React.useState(false);
  const [webSearch, setWebSearch] = React.useState(false);
  const [backend, setBackend] = React.useState("ddgs");
  const [braveKeySet, setBraveKeySet] = React.useState(false);
  const [braveKeyInput, setBraveKeyInput] = React.useState("");
  const [tavilyKeySet, setTavilyKeySet] = React.useState(false);
  const [tavilyKeyInput, setTavilyKeyInput] = React.useState("");
  const [savingTavilyKey, setSavingTavilyKey] = React.useState(false);
  const [keenableKeySet, setKeenableKeySet] = React.useState(false);
  const [keenableKeyInput, setKeenableKeyInput] = React.useState("");
  const [savingKeenableKey, setSavingKeenableKey] = React.useState(false);
  const [proxyInput, setProxyInput] = React.useState("");
  const [savingProxy, setSavingProxy] = React.useState(false);
  const [globalProxyInput, setGlobalProxyInput] = React.useState("");
  const [savingGlobalProxy, setSavingGlobalProxy] = React.useState(false);
  const [testing, setTesting] = React.useState(false);
  const [loaded, setLoaded] = React.useState(false);
  const [saving, setSaving] = React.useState(false);
  const [savingKey, setSavingKey] = React.useState(false);
  const [pyInterp, setPyInterp] = React.useState("");
  const [workers, setWorkers] = React.useState("3");
  const [savingWorkers, setSavingWorkers] = React.useState(false);
  // Operation constraint injection scope (both on by default).
  const [injectPlanner, setInjectPlanner] = React.useState(true);
  const [injectWorker, setInjectWorker] = React.useState(true);
  // Experimental: noa context compaction (off by default).
  const [noaCompaction, setNoaCompaction] = React.useState(false);

  const apply = React.useCallback((s: Settings) => {
    setTrafficCapture(!!s.traffic_capture);
    setAgentTrafficBinding(!!s.agent_traffic_binding);
    setWebSearch(!!s.web_search_enabled);
    setBackend(s.web_search_backend || "ddgs");
    setBraveKeySet(!!s.brave_key_set);
    setTavilyKeySet(!!s.tavily_key_set);
    setKeenableKeySet(!!s.keenable_key_set);
    setProxyInput(s.web_search_proxy ?? "");
    setGlobalProxyInput(s.global_proxy ?? "");
    setPyInterp(s.python_interpreter ?? "");
    setWorkers(String(s.workers ?? 3));
    setInjectPlanner(s.constraints_inject_planner !== false);
    setInjectWorker(s.constraints_inject_worker !== false);
    setNoaCompaction(!!s.noa_compaction);
  }, []);

  const saveWorkers = () => {
    const n = Number(workers);
    if (!Number.isInteger(n) || n <= 0) {
      toast.error(t("Concurrency must be an integer greater than 0"));
      return;
    }
    setSavingWorkers(true);
    api
      .setSettings({ workers: n })
      .then((s) => {
        apply(s);
        toast.success(t("Saved the concurrent worker agent count (applies to tasks started afterwards)"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingWorkers(false));
  };

  const savePython = () => {
    setSaving(true);
    api
      .setSettings({ python_interpreter: pyInterp.trim() })
      .then((s) => {
        apply(s);
        toast.success(t("Saved Python interpreter config"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSaving(false));
  };
  const detectPython = () => {
    setSaving(true);
    api
      .detectPython()
      .then((r) => setPyInterp(r.python_interpreter))
      .catch(() => undefined)
      .finally(() => setSaving(false));
  };

  React.useEffect(() => {
    api
      .settings()
      .then(apply)
      .catch(() => undefined)
      .finally(() => setLoaded(true));
  }, [apply]);

  const toggleTraffic = (v: boolean) => {
    setTrafficCapture(v); // optimistic
    setSaving(true);
    api
      .setSettings({ traffic_capture: v })
      .then(apply)
      .catch(() => setTrafficCapture(!v)) // revert on failure
      .finally(() => setSaving(false));
  };

  const toggleInjectPlanner = (v: boolean) => {
    setInjectPlanner(v); // optimistic
    api
      .setSettings({ constraints_inject_planner: v })
      .then(apply)
      .catch(() => setInjectPlanner(!v)); // revert on failure
  };

  const toggleAgentTrafficBinding = (v: boolean) => {
    setAgentTrafficBinding(v);
    setSaving(true);
    api
      .setSettings({ agent_traffic_binding: v })
      .then((s) => {
        apply(s);
        toast.success(v ? t("Enabled automatic agent traffic binding") : t("Disabled automatic agent traffic binding"));
      })
      .catch((e) => {
        setAgentTrafficBinding(!v);
        toast.error(t("Save failed: {error}", { error: (e as Error).message }));
      })
      .finally(() => setSaving(false));
  };

  const toggleInjectWorker = (v: boolean) => {
    setInjectWorker(v); // optimistic
    api
      .setSettings({ constraints_inject_worker: v })
      .then(apply)
      .catch(() => setInjectWorker(!v)); // revert on failure
  };

  const toggleNoaCompaction = (v: boolean) => {
    setNoaCompaction(v); // optimistic
    api
      .setSettings({ noa_compaction: v })
      .then((s) => {
        apply(s);
        toast.success(
          v
            ? t("Enabled noa context compaction (applies to runs started afterwards)")
            : t("Disabled noa context compaction (built-in compaction restored)"),
        );
      })
      .catch((e) => {
        setNoaCompaction(!v); // revert on failure
        toast.error(t("Save failed: {error}", { error: (e as Error).message }));
      });
  };

  // Persist a web-search patch (enable and/or backend). Optimistic with refetch.
  const saveWebSearch = (patch: Partial<Settings>) => {
    setSaving(true);
    api
      .setSettings(patch)
      .then((s) => {
        apply(s);
        toast.success(t("Saved web search config"));
      })
      .catch((e) => {
        toast.error(t("Save failed: {error}", { error: (e as Error).message }));
        api
          .settings()
          .then(apply)
          .catch(() => undefined);
      })
      .finally(() => setSaving(false));
  };

  const saveBraveKey = () => {
    setSavingKey(true);
    api
      .setSettings({ brave_search_api_key: braveKeyInput })
      .then((s) => {
        apply(s);
        setBraveKeyInput("");
        toast.success(t("Saved Brave API Key"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingKey(false));
  };

  const saveTavilyKey = () => {
    setSavingTavilyKey(true);
    api
      .setSettings({ tavily_search_api_key: tavilyKeyInput })
      .then((s) => {
        apply(s);
        setTavilyKeyInput("");
        toast.success(t("Saved Tavily API Key"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingTavilyKey(false));
  };

  const saveKeenableKey = () => {
    setSavingKeenableKey(true);
    api
      .setSettings({ keenable_search_api_key: keenableKeyInput })
      .then((s) => {
        apply(s);
        setKeenableKeyInput("");
        toast.success(t("Saved Keenable API Key"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingKeenableKey(false));
  };

  const saveProxy = () => {
    setSavingProxy(true);
    api
      .setSettings({ web_search_proxy: proxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(proxyInput.trim() ? t("Saved egress proxy") : t("Cleared egress proxy (direct connection)"));
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingProxy(false));
  };

  const saveGlobalProxy = () => {
    setSavingGlobalProxy(true);
    api
      .setSettings({ global_proxy: globalProxyInput.trim() })
      .then((s) => {
        apply(s);
        toast.success(
          globalProxyInput.trim() ? t("Saved global proxy") : t("Cleared global proxy (direct connection)"),
        );
      })
      .catch((e) => toast.error(t("Save failed: {error}", { error: (e as Error).message })))
      .finally(() => setSavingGlobalProxy(false));
  };

  // Run a real "test" search ("test") against the CURRENT form values (backend +
  // proxy + entered key), falling back to saved values server-side. Toasts result.
  const runTest = () => {
    setTesting(true);
    api
      .testWebSearch({
        web_search_backend: backend,
        web_search_proxy: proxyInput.trim(),
        brave_search_api_key: braveKeyInput,
        tavily_search_api_key: tavilyKeyInput,
        keenable_search_api_key: keenableKeyInput,
      })
      .then((r) => {
        if (r.ok)
          toast.success(
            t("Search test succeeded · {backend} returned {count} results", {
              backend: r.backend ?? "",
              count: r.count ?? 0,
            }),
          );
        else toast.error(t("Search test failed: {error}", { error: r.error || t("unknown error") }));
      })
      .catch((e) => toast.error(t("Search test failed: {error}", { error: (e as Error).message })))
      .finally(() => setTesting(false));
  };

  // brave-free selected but no key stored and none being entered → tool stays off.
  const braveNeedsKey = webSearch && backend === "brave-free" && !braveKeySet;

  return {
    trafficCapture,
    agentTrafficBinding,
    webSearch,
    setWebSearch,
    backend,
    setBackend,
    braveKeySet,
    braveKeyInput,
    setBraveKeyInput,
    tavilyKeySet,
    tavilyKeyInput,
    setTavilyKeyInput,
    savingTavilyKey,
    keenableKeySet,
    keenableKeyInput,
    setKeenableKeyInput,
    savingKeenableKey,
    proxyInput,
    setProxyInput,
    savingProxy,
    globalProxyInput,
    setGlobalProxyInput,
    savingGlobalProxy,
    testing,
    loaded,
    saving,
    savingKey,
    pyInterp,
    setPyInterp,
    workers,
    setWorkers,
    savingWorkers,
    injectPlanner,
    injectWorker,
    noaCompaction,
    braveNeedsKey,
    saveWorkers,
    savePython,
    detectPython,
    toggleTraffic,
    toggleInjectPlanner,
    toggleAgentTrafficBinding,
    toggleInjectWorker,
    toggleNoaCompaction,
    saveWebSearch,
    saveBraveKey,
    saveTavilyKey,
    saveKeenableKey,
    saveProxy,
    saveGlobalProxy,
    runTest,
  };
}
