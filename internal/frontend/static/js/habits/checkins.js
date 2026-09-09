import { setSaveState } from "../save-status.js";

const failureTypes = new Set(["error", "retries-failed"]);
const documents = new WeakMap();
const failureMessage = "Not saved. Press the same date again to retry.";

function stateFor(document) {
  if (!documents.has(document)) documents.set(document, { attempts: new Map(), sources: new WeakMap(), forms: new Map(), read: 0, reading: 0 });
  return documents.get(document);
}

function checkInSource(id) {
  return `habit-checkin:${id}`;
}

function settle(button) {
  if (button.dataset.saveState) delete button.dataset.saveState;
  if (button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
}

function isConfirmed(button, desired) {
  return button?.getAttribute("aria-pressed") === desired;
}

function updateFeedback(document) {
  const status = document.getElementById("habit-checkin-feedback");
  if (!status || status.dataset.result === "rejected") return;
  const failed = [...stateFor(document).attempts.values()].some((attempt) => attempt.failed && !attempt.confirmed);
  if (failed) {
    if (status.textContent !== failureMessage) status.textContent = failureMessage;
  } else if (status.textContent === failureMessage) {
    status.textContent = "";
  }
}

function handleFormFetch(document, event) {
  const form = event.detail?.el?.closest?.("#habit-create, #habit-edit form, #habit-delete form");
  if (!form) return;
  const source = form.id || form.closest("dialog").id;
  const { forms } = stateFor(document);
  const type = event.detail.type;
  if (type === "started") {
    forms.set(source, { failed: false });
    setSaveState(document, source, "saving");
    return;
  }
  const attempt = forms.get(source);
  if (!attempt) return;
  if (type === "retrying") {
    attempt.failed = false;
    setSaveState(document, source, "saving");
  } else if (failureTypes.has(type)) {
    attempt.failed = true;
    setSaveState(document, source, "failed");
  } else if (type === "finished") {
    forms.delete(source);
    if (!attempt.failed) setSaveState(document, source, "saved");
  }
}

function reconcileAttempts(document) {
  const { attempts } = stateFor(document);
  const read = Number(document.getElementById("habit-grid")?.dataset.read || 0);
  for (const [id, attempt] of attempts) {
    const button = document.getElementById(id);
    if (!button) {
      if (!attempt.finished) continue;
      attempts.delete(id);
      setSaveState(document, checkInSource(id), "saved");
      continue;
    }
    // A receipt fences the read, not its value: another device may already have won.
    attempt.confirmed ||= (attempt.read > 0 && read >= attempt.read)
      || (!attempt.read && attempt.failed && isConfirmed(button, attempt.desired));
    if (attempt.finished && (attempt.confirmed || attempt.rejected)) {
      attempts.delete(id);
      settle(button);
      setSaveState(document, checkInSource(id), "saved");
    } else if (attempt.failed && !attempt.confirmed) {
      if (button.dataset.saveState !== "failed") button.dataset.saveState = "failed";
      if (attempt.finished && button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
      setSaveState(document, checkInSource(id), "failed");
    } else {
      if (button.dataset.saveState !== "pending") button.dataset.saveState = "pending";
      if (button.getAttribute("aria-busy") !== "true") button.setAttribute("aria-busy", "true");
      setSaveState(document, checkInSource(id), attempt.confirmed ? "saved" : "saving");
    }
  }
  updateFeedback(document);
}

function acknowledge(document, event) {
  const receipt = event.detail?._habitcheckinack;
  if (typeof receipt !== "string") return;
  const { attempts } = stateFor(document);
  const [token, result] = receipt.split(":");
  const attempt = [...attempts.values()].find((attempt) => attempt.token === token);
  if (!attempt || attempt.read || attempt.rejected) return;
  if (result === "rejected") {
    attempt.rejected = true;
  } else if (result === "committed") {
    const state = stateFor(document);
    attempt.read = ++state.read;
    document.getElementById("habit-matrix").dispatchEvent(new document.defaultView.CustomEvent("habit-checkin-read", {
      detail: { read: attempt.read },
    }));
  }
  reconcileAttempts(document);
}

export function handleCheckInFetch(document, event) {
  handleFormFetch(document, event);
  if (event.detail?.el?.id === "habit-matrix") {
    const state = stateFor(document);
    const type = event.detail.type;
    if (type === "started") state.reading++;
    if (type === "finished") state.reading = Math.max(0, state.reading - 1);
    for (const attempt of state.attempts.values()) {
      if (!attempt.read || attempt.confirmed) continue;
      // An empty/truncated stream is retryable, but canceled older reads can finish last.
      if (failureTypes.has(type) || (type === "finished" && state.reading === 0)) attempt.failed = true;
      else if (["started", "retrying"].includes(type)) attempt.failed = false;
    }
    reconcileAttempts(document);
    return;
  }
  const source = event.detail?.el?.closest?.("[data-checkin-action]");
  if (!source) return;
  const { attempts, sources } = stateFor(document);
  const liveButton = document.getElementById(source.id);
  const button = liveButton || source;
  const status = document.getElementById("habit-checkin-feedback");

  if (event.detail.type === "started") {
    attempts.set(source.id, {
      token: source.dataset.attempt,
      desired: source.dataset.desired,
      read: 0,
      rejected: false,
      confirmed: false,
      failed: false,
      finished: false,
    });
    sources.set(source, attempts.get(source.id));
    button.dataset.saveState = "pending";
    button.setAttribute("aria-busy", "true");
    if (status) {
      status.dataset.result = "";
      status.textContent = "";
    }
    setSaveState(document, checkInSource(source.id), "saving");
    updateFeedback(document);
    return;
  }
  const attempt = attempts.get(source.id);
  if (!attempt || attempt !== sources.get(source)) return;
  if (event.detail.type === "retrying") attempt.failed = false;
  else if (failureTypes.has(event.detail.type)) attempt.failed = true;
  else if (event.detail.type === "finished") {
    attempt.finished = true;
    // Missing receipts (including interrupted SSE responses) remain explicitly retryable.
    if (!attempt.read && !attempt.rejected) attempt.failed = true;
  } else return;
  reconcileAttempts(document);
}

function preserveMatrixState(document) {
  const matrix = document.getElementById("habit-matrix");
  if (!matrix) return;
  const view = document.defaultView;
  const scroller = () => document.getElementById("habit-scroll");
  let scrollLeft = scroller()?.scrollLeft || 0;
  let scrollFrame;
  let focusedID = "";

  matrix.addEventListener("scroll", (event) => {
    if (event.target !== scroller()) return;
    view.cancelAnimationFrame(scrollFrame);
    scrollFrame = view.requestAnimationFrame(() => { scrollLeft = scroller()?.scrollLeft || 0; });
  }, true);
  document.addEventListener("focusin", (event) => {
    focusedID = event.target.closest?.("[data-checkin-action], .habit-action")?.id || "";
  });
  new view.MutationObserver(() => {
    const scroll = scroller();
    if (scroll) scroll.scrollLeft = scrollLeft;
    if (focusedID && (!document.activeElement || document.activeElement === document.body)) {
      (document.getElementById(focusedID) || matrix).focus();
    }
    reconcileAttempts(document);
  }).observe(matrix, { attributes: true, characterData: true, childList: true, subtree: true });
}

export function initCheckIns(document) {
  document.addEventListener("datastar-signal-patch", (event) => acknowledge(document, event));
  document.addEventListener("datastar-fetch", (event) => handleCheckInFetch(document, event));
  preserveMatrixState(document);
}

if (typeof window !== "undefined") initCheckIns(window.document);
