// In-page helpers for role-probe-e2e.js. inventory() runs inside the page (it is
// passed to page.evaluate, so it must not close over anything in this module).

// An open overlay: a dialog, sheet, menu or listbox (Radix renders them in portals).
const OVERLAY = "[role=dialog]:visible, [role=alertdialog]:visible, [role=menu]:visible, [role=listbox]:visible";

// inventory lists the interactive controls the user can reach now. When an overlay is
// open only its controls count (a modal overlay blocks the rest). The sidebar's links
// are navigation, not controls, and are left out, as is the display-preferences
// popover (language, theme, layout: browser-local state, no /api request). Each control is tagged with
// data-probe-id so the caller can click it; key = scope + kind + accessible name, so
// one of each repeated row button is clicked, not every row's.
function inventory() {
  const SEL = 'button, [role=button], [role=menuitem], [role=menuitemcheckbox], [role=menuitemradio], [role=switch], ' +
    '[role=checkbox], [role=tab], [role=option], [role=combobox], input, textarea, select';
  const visible = (el) => {
    const r = el.getBoundingClientRect();
    const st = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && st.visibility !== "hidden" && st.display !== "none";
  };
  const overlays = [...document.querySelectorAll("[role=dialog],[role=alertdialog],[role=menu],[role=listbox]")].filter(visible);
  const roots = overlays.length ? [overlays[overlays.length - 1]] : [document.body];
  const scopeOf = (el) => (el.closest("[role=menu],[role=listbox]") ? "menu" : el.closest("[role=dialog],[role=alertdialog]") ? "dialog" : "main");
  const out = [];
  let n = Number(document.body.dataset.probeNext || 0);
  for (const root of roots) {
    for (const el of root.querySelectorAll(SEL)) {
      if (!visible(el) || el.closest("[data-slot=sidebar]")) continue;
      // The display-preferences popover (language, theme, layout) is client-only state.
      if (el.matches("header button:has(svg.lucide-settings)")) continue;
      const tag = el.tagName.toLowerCase();
      const type = (el.getAttribute("type") || "").toLowerCase();
      if (tag === "input" && ["hidden", "file", "password"].includes(type)) continue;
      const disabled = el.matches(":disabled") || el.readOnly === true || el.getAttribute("aria-disabled") === "true" ||
        el.hasAttribute("data-disabled") || !!el.closest("fieldset:disabled,[aria-disabled=true]");
      const name = (el.getAttribute("aria-label") || el.getAttribute("title") || el.innerText || el.getAttribute("placeholder") || el.id || "")
        .replace(/\s+/g, " ").trim().slice(0, 60);
      const kind = `${tag}${type ? ":" + type : ""}${el.getAttribute("role") ? "(" + el.getAttribute("role") + ")" : ""}`;
      const fill = (tag === "input" && !["checkbox", "radio", "button", "submit", "reset", "range", "color"].includes(type)) || tag === "textarea";
      if (!el.dataset.probeId) el.dataset.probeId = String(n++);
      out.push({ id: el.dataset.probeId, key: `${scopeOf(el)} ${kind} "${name}"`, name, type, disabled, fill });
    }
  }
  document.body.dataset.probeNext = String(n);
  return out;
}

module.exports = { inventory, OVERLAY };
