import { initJotpadCM } from "./cm.js";
import { setSaveState } from "../save-status.js";

const root = document.querySelector(".jotpad");
const tabs = [...root.querySelectorAll('[role="tab"]')];
const remotes = new Map();
const statuses = new Map();
const storageKey = "jot-note";
let selected = "daily";
try {
  const stored = localStorage.getItem(storageKey);
  if (tabs.some((tab) => tab.dataset.note === stored)) selected = stored;
} catch { /* Storage may be disabled. */ }

const select = (note) => {
  selected = note;
  for (const tab of tabs) {
    const active = tab.dataset.note === note;
    const panel = document.getElementById(tab.getAttribute("aria-controls"));
    tab.setAttribute("aria-selected", String(active));
    tab.tabIndex = active ? 0 : -1;
    if (panel.hidden === active) {
      if (!active) panel.dispatchEvent(new Event("companion-hide"));
      panel.hidden = !active;
      if (active) panel.dispatchEvent(new Event("companion-show"));
    }
  }
  setSaveState(document, "jot", statuses.get(note) ?? "saved");
  try { localStorage.setItem(storageKey, note); } catch { /* Selection still works. */ }
};

for (const [index, tab] of tabs.entries()) {
  const note = tab.dataset.note;
  const label = tab.textContent.trim();
  const panel = document.getElementById(tab.getAttribute("aria-controls"));
  const mount = panel.querySelector(".jot-mount");
  const text = JSON.parse(document.getElementById(`${mount.id}-text`).textContent);
  initJotpadCM(mount, text, `/jot?note=${note}`, Number(mount.dataset.maxlen), {
    note, label,
    version: Number(mount.dataset.jotVersion),
    onReady: (remote) => remotes.set(note, remote),
    onState: (state) => {
      tab.dataset.state = state;
      const detail = state === "offline" ? "Offline, will retry" : state === "saving" ? "Unsaved changes" : "Saved";
      tab.setAttribute("aria-description", detail);
      tab.querySelector(".note-status").textContent = state === "offline" ? "!" : state === "saving" ? "•" : "";
    },
    onStatus: (state) => {
      statuses.set(note, state);
      if (note === selected) setSaveState(document, "jot", state);
    },
  });
  tab.addEventListener("click", () => select(note));
  tab.addEventListener("keydown", (event) => {
    let next;
    switch (event.key) {
      case "ArrowLeft": next = (index + tabs.length - 1) % tabs.length; break;
      case "ArrowRight": next = (index + 1) % tabs.length; break;
      case "Home": next = 0; break;
      case "End": next = tabs.length - 1; break;
      default: return;
    }
    event.preventDefault();
    select(tabs[next].dataset.note);
    tabs[next].focus();
  });
}
window.__jotRemote = (note, version, text) => remotes.get(note)?.(version, text);
for (const [note, [version, text]] of window.__jotPending ?? []) {
  window.__jotRemote(note, version, text);
}
delete window.__jotPending;
select(selected);
