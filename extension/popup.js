const $ = (id) => document.getElementById(id);

async function refreshState() {
  const st = await chrome.runtime.sendMessage({ type: "getState" });
  const lines = [];
  lines.push(st.capturing
    ? `● Recording tab ${st.tabId}`
    : "○ Not recording");
  lines.push(`ingested: ${st.ingested} · queued: ${st.queued} · failed: ${st.failed} · skipped (SSO): ${st.skipped || 0}`);
  if (st.lastError) lines.push(st.lastError);
  $("status").textContent = lines.join("\n");
  $("error").textContent = st.capturing ? "" : (st.lastError || "");
}

$("save").addEventListener("click", async () => {
  await chrome.storage.local.set({
    serverUrl: $("serverUrl").value.trim(),
    token: $("token").value.trim(),
    skipHosts: $("skipHosts").value,
  });
  $("save").textContent = "Saved ✓";
  setTimeout(() => ($("save").textContent = "Save settings"), 1200);
});

// The active tab is resolved up front so the click handler can call
// chrome.permissions.request() immediately: Chrome only shows the prompt while the
// click's user activation is live. activeTab (granted when the popup opens) lets us
// read this tab's URL without the broad "tabs" permission.
const activeTab = chrome.tabs.query({ active: true, currentWindow: true }).then(([tab]) => tab);

$("start").addEventListener("click", async () => {
  const tab = await activeTab;
  if (!tab || !ArtexHosts.originPattern(tab.url)) {
    $("error").textContent = "Only http(s) pages can be captured.";
    return;
  }
  if (ArtexHosts.isSkippedUrl(tab.url, ArtexHosts.parseHostList($("skipHosts").value))) {
    $("error").textContent = "This host is on the sign-in provider skip list and is never captured.";
    return;
  }
  // The only host access the extension asks for is the ARTEX server's own origin
  // (uploads need it; chrome.debugger does not need the captured site's). Chrome
  // shows its prompt once and remembers the answer.
  const serverOrigin = ArtexHosts.originPattern($("serverUrl").value.trim());
  if (!serverOrigin) {
    $("error").textContent = "Enter the ARTEX URL (http:// or https://) and save it first.";
    return;
  }
  if (!(await chrome.permissions.request({ origins: [serverOrigin] }))) {
    $("error").textContent = "Host access to the ARTEX server was not granted, so uploads cannot work.";
    return;
  }
  const out = await chrome.runtime.sendMessage({ type: "start", tabId: tab.id });
  if (out && out.ok === false) $("error").textContent = out.error || "failed to start";
  refreshState();
});

$("stop").addEventListener("click", async () => {
  await chrome.runtime.sendMessage({ type: "stop" });
  refreshState();
});

$("flush").addEventListener("click", async () => {
  await chrome.runtime.sendMessage({ type: "flushNow" });
  refreshState();
});

(async () => {
  const { serverUrl = "", token = "", skipHosts = "" } = await chrome.storage.local.get(["serverUrl", "token", "skipHosts"]);
  $("serverUrl").value = serverUrl;
  $("token").value = token;
  $("skipHosts").value = skipHosts;
  $("defaults").textContent = "Always skipped: " + ArtexHosts.DEFAULT_SKIP_HOSTS.join(", ");
  refreshState();
  setInterval(refreshState, 1500);
})();
