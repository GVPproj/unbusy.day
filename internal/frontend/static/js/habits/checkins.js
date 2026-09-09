import { setSaveState } from "../save-status.js";

const failureTypes = new Set(["error", "retries-failed"]);
const documents = new WeakMap();
const failureMessage = "Not saved. Press the same date again to retry.";

function stateFor(document) {
  if (!documents.has(document)) documents.set(document, { attempts: new Map(), forms: new Map() });
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

export function handleCheckInFetch(document, event) {
  handleFormFetch(document, event);
  const source = event.detail?.el?.closest?.("[data-checkin-action]");
  if (!source) return;
  const { attempts } = stateFor(document);
  const liveButton = document.getElementById(source.id);
  const button = liveButton || source;
  const status = document.getElementById("habit-checkin-feedback");

  if (event.detail.type === "started") {
    attempts.set(source.id, {
      desired: source.dataset.desired,
      confirmed: false,
      failed: false,
      finished: false,
    });
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
  if (!attempt) return;
  if (!liveButton && source.isConnected === false) {
    if (failureTypes.has(event.detail.type)) {
      attempt.failed = true;
      setSaveState(document, checkInSource(source.id), "failed");
    } else if (event.detail.type === "finished") {
      attempts.delete(source.id);
      setSaveState(document, checkInSource(source.id), attempt.failed ? "failed" : "saved");
    }
    return;
  }
  if (event.detail.type === "retrying") {
    setSaveState(document, checkInSource(source.id), attempt.confirmed ? "saved" : "saving");
    return;
  }
  if (failureTypes.has(event.detail.type)) {
    attempt.failed = true;
    attempt.confirmed ||= isConfirmed(button, attempt.desired);
    button.dataset.saveState = attempt.confirmed ? "pending" : "failed";
    setSaveState(document, checkInSource(source.id), attempt.confirmed ? "saved" : "failed");
    updateFeedback(document);
    return;
  }
  if (event.detail.type !== "finished") return;

  attempt.finished = true;
  attempt.confirmed ||= isConfirmed(button, attempt.desired);
  if (status?.dataset.result === "rejected" || attempt.confirmed) {
    attempts.delete(source.id);
    settle(button);
    setSaveState(document, checkInSource(source.id), "saved");
  } else if (attempt.failed) {
    settle(button);
    button.dataset.saveState = "failed";
    setSaveState(document, checkInSource(source.id), "failed");
  }
  updateFeedback(document);
}

function preserveMatrixState(document) {
  const matrix = document.getElementById("habit-matrix");
  if (!matrix) return;
  const { attempts } = stateFor(document);
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
    for (const [id, attempt] of attempts) {
      const button = document.getElementById(id);
      if (!button) {
        if (!attempt.finished) continue;
        attempts.delete(id);
        setSaveState(document, checkInSource(id), "saved");
        continue;
      }
      if (isConfirmed(button, attempt.desired)) {
        attempt.confirmed = true;
        if (attempt.finished) {
          attempts.delete(id);
          settle(button);
        }
        setSaveState(document, checkInSource(id), "saved");
      } else if (attempt.failed && attempt.finished) {
        if (button.dataset.saveState !== "failed") button.dataset.saveState = "failed";
        if (button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
      } else {
        if (button.dataset.saveState !== "pending") button.dataset.saveState = "pending";
        if (button.getAttribute("aria-busy") !== "true") button.setAttribute("aria-busy", "true");
      }
    }
    updateFeedback(document);
  }).observe(matrix, { attributes: true, characterData: true, childList: true, subtree: true });
}

export function initCheckIns(document) {
  document.addEventListener("datastar-fetch", (event) => handleCheckInFetch(document, event));
  preserveMatrixState(document);
}

if (typeof window !== "undefined") initCheckIns(window.document);
