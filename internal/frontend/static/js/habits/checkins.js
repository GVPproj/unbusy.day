import { setSaveState } from "../save-status.js";

const failureTypes = new Set(["error", "retries-failed"]);
const documents = new WeakMap();
const failureMessage = "Save not confirmed. Press the same date again to retry the original change.";
const readFailureMessage = "Saved, but refresh not confirmed. Press the same date again to retry loading.";

function stateFor(document) {
  if (!documents.has(document)) documents.set(document, { attempts: new Map(), sources: new WeakMap(), readGeneration: 0, reading: 0 });
  return documents.get(document);
}

function checkInSource(id) {
  return `habit-checkin:${id}`;
}

function settle(button) {
  if (button.dataset.saveState) delete button.dataset.saveState;
  if (button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
}

function updateFeedback(document) {
  const status = document.getElementById("habit-checkin-feedback");
  if (!status || status.dataset.result === "rejected") return;
  const failed = [...stateFor(document).attempts.entries()]
    .filter(([id, attempt]) => document.getElementById(id) && attempt.failed && !attempt.confirmed)
    .map(([, attempt]) => attempt);
  const message = [...new Set(failed.map((attempt) => attempt.readGeneration ? readFailureMessage : failureMessage))].join(" ");
  if (message) {
    if (status.textContent !== message) status.textContent = message;
  } else if (status.textContent.includes(failureMessage) || status.textContent.includes(readFailureMessage)) {
    status.textContent = "";
  }
}

function reconcileAttempts(document) {
  const { attempts } = stateFor(document);
  const renderedRead = Number(document.getElementById("habit-grid")?.dataset.read || 0);
  for (const [id, attempt] of attempts) {
    const button = document.getElementById(id);
    if (!button) {
      if (!attempt.finished) continue;
      // Navigation hides recovery, but must not discard an unknown write's intent.
      if (attempt.rejected || attempt.confirmed) attempts.delete(id);
      setSaveState(document, checkInSource(id), "saved");
      continue;
    }
    // A receipt fences the read, not its value: another device may already have won.
    attempt.confirmed ||= attempt.readGeneration > 0 && renderedRead >= attempt.readGeneration;
    if (attempt.finished && (attempt.confirmed || attempt.rejected)) {
      attempts.delete(id);
      settle(button);
      setSaveState(document, checkInSource(id), "saved");
    } else if (attempt.failed && !attempt.confirmed) {
      if (button.dataset.saveState !== "failed") button.dataset.saveState = "failed";
      if ((attempt.finished || attempt.readGeneration) && button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
      setSaveState(document, checkInSource(id), "failed");
    } else {
      if (button.dataset.saveState !== "pending") button.dataset.saveState = "pending";
      if (button.getAttribute("aria-busy") !== "true") button.setAttribute("aria-busy", "true");
      setSaveState(document, checkInSource(id), attempt.confirmed ? "saved" : "saving");
    }
  }
  updateFeedback(document);
}

function requestRead(document, attempt) {
  attempt.readGeneration = ++stateFor(document).readGeneration;
  attempt.failed = false;
  document.getElementById("habit-matrix").dispatchEvent(new document.defaultView.CustomEvent("habit-checkin-read", {
    detail: { read: attempt.readGeneration },
  }));
}

function recoverCheckIn(document, event) {
  const button = event.target.closest?.("[data-checkin-action]");
  const attempt = stateFor(document).attempts.get(button?.id);
  if (!attempt || attempt.confirmed || attempt.rejected) return;
  if (attempt.readGeneration) {
    event.preventDefault();
    event.stopImmediatePropagation();
    if (attempt.failed) {
      requestRead(document, attempt);
      reconcileAttempts(document);
    }
  } else {
    // The rendered toggle may now be the inverse of the unacknowledged write.
    button.dataset.desired = attempt.desired;
  }
}

function acknowledge(document, event) {
  const receipt = event.detail?._habitcheckinack;
  if (typeof receipt !== "string") return;
  const { attempts } = stateFor(document);
  const [token, result] = receipt.split(":");
  const attempt = [...attempts.values()].find((attempt) => attempt.token === token);
  if (!attempt || attempt.readGeneration || attempt.rejected) return;
  if (result === "rejected") {
    attempt.rejected = true;
  } else if (result === "committed") {
    requestRead(document, attempt);
  }
  reconcileAttempts(document);
}

export function handleCheckInFetch(document, event) {
  if (event.detail?.el?.id === "habit-matrix") {
    const state = stateFor(document);
    const type = event.detail.type;
    if (type === "started") state.reading++;
    if (type === "finished") state.reading = Math.max(0, state.reading - 1);
    for (const attempt of state.attempts.values()) {
      if (!attempt.readGeneration || attempt.confirmed) continue;
      // An empty/truncated stream is retryable, but canceled older reads can finish last.
      if ((failureTypes.has(type) && state.reading <= 1) || (type === "finished" && state.reading === 0)) attempt.failed = true;
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
      readGeneration: 0,
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
    if (!attempt.readGeneration && !attempt.rejected) attempt.failed = true;
  } else return;
  reconcileAttempts(document);
}

function preserveMatrixState(document) {
  const matrix = document.getElementById("habit-matrix");
  if (!matrix) return;
  const view = document.defaultView;
  const scroller = () => document.getElementById("habit-scroll");
  let currentScroll = scroller();
  let scrollLeft = currentScroll?.scrollLeft || 0;
  let restoredScroll;
  let focusedID = "";

  matrix.addEventListener("scroll", (event) => {
    const scroll = scroller();
    if (event.target !== scroll) return;
    if (restoredScroll?.element === scroll && restoredScroll.left === scroll.scrollLeft) {
      restoredScroll = undefined;
      return;
    }
    restoredScroll = undefined;
    scrollLeft = scroll.scrollLeft;
  }, true);
  document.addEventListener("focusin", (event) => {
    focusedID = event.target.closest?.("[data-checkin-action], .habit-action")?.id || "";
  });
  new view.MutationObserver(() => {
    const scroll = scroller();
    // Retained scrollers already preserve their position; only replacement loses it.
    if (scroll && scroll !== currentScroll) {
      scroll.scrollLeft = scrollLeft;
      restoredScroll = { element: scroll, left: scroll.scrollLeft };
    }
    currentScroll = scroll;
    if (focusedID && (!document.activeElement || document.activeElement === document.body)
      && !document.querySelector?.("dialog[open]")) {
      (document.getElementById(focusedID) || matrix).focus({ preventScroll: true });
    }
    reconcileAttempts(document);
  }).observe(matrix, { attributes: true, characterData: true, childList: true, subtree: true });
}

export function initCheckIns(document) {
  document.addEventListener("click", (event) => recoverCheckIn(document, event), true);
  document.addEventListener("datastar-signal-patch", (event) => acknowledge(document, event));
  document.addEventListener("datastar-fetch", (event) => handleCheckInFetch(document, event));
  preserveMatrixState(document);
}

if (typeof window !== "undefined") initCheckIns(window.document);
