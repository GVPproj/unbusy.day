const failureTypes = new Set(["error", "retries-failed"]);
const attempts = new Map();
const failureMessage = "Not saved. Press the same date again to retry.";

function settle(button) {
  delete button.dataset.saveState;
  button.removeAttribute("aria-busy");
}

function isConfirmed(button, desired) {
  return button?.getAttribute("aria-pressed") === desired;
}

export function handleCheckInFetch(document, event) {
  const source = event.detail?.el?.closest?.("[data-checkin-action]");
  if (!source) return;
  const button = document.getElementById(source.id) || source;
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
      status.textContent = "Saving…";
    }
    return;
  }
  const attempt = attempts.get(source.id);
  if (!attempt) return;
  if (event.detail.type === "retrying") {
    if (status) status.textContent = "Connection interrupted; retrying…";
    return;
  }
  if (failureTypes.has(event.detail.type)) {
    attempt.failed = true;
    attempt.confirmed ||= isConfirmed(button, attempt.desired);
    button.dataset.saveState = attempt.confirmed ? "pending" : "failed";
    if (status) status.textContent = attempt.confirmed ? "Saved." : failureMessage;
    return;
  }
  if (event.detail.type !== "finished") return;

  attempt.finished = true;
  attempt.confirmed ||= isConfirmed(button, attempt.desired);
  if (status?.dataset.result === "rejected") {
    attempts.delete(source.id);
    settle(button);
  } else if (attempt.confirmed) {
    attempts.delete(source.id);
    settle(button);
    if (status) status.textContent = "Saved.";
  } else if (attempt.failed) {
    settle(button);
    button.dataset.saveState = "failed";
    if (status) status.textContent = failureMessage;
  }
}

function preserveMatrixState(document) {
  const matrix = document.getElementById("habit-matrix");
  if (!matrix) return;
  const view = document.defaultView;
  let scrollLeft = matrix.scrollLeft;
  let scrollFrame;
  let focusedID = "";

  matrix.addEventListener("scroll", () => {
    view.cancelAnimationFrame(scrollFrame);
    scrollFrame = view.requestAnimationFrame(() => { scrollLeft = matrix.scrollLeft; });
  });
  document.addEventListener("focusin", (event) => {
    focusedID = event.target.closest?.("[data-checkin-action]")?.id || "";
  });
  new view.MutationObserver(() => {
    matrix.scrollLeft = scrollLeft;
    if (focusedID && (!document.activeElement || document.activeElement === document.body)) {
      (document.getElementById(focusedID) || matrix).focus();
    }
    const status = document.getElementById("habit-checkin-feedback");
    for (const [id, attempt] of attempts) {
      const button = document.getElementById(id);
      if (isConfirmed(button, attempt.desired)) {
        attempt.confirmed = true;
        if (attempt.finished) {
          attempts.delete(id);
          settle(button);
        }
        if (status && status.textContent !== "Saved.") status.textContent = "Saved.";
      } else if (attempt.failed && attempt.finished && button) {
        if (button.dataset.saveState !== "failed") button.dataset.saveState = "failed";
        if (button.hasAttribute("aria-busy")) button.removeAttribute("aria-busy");
        if (status && status.textContent !== failureMessage) status.textContent = failureMessage;
      }
    }
  }).observe(matrix, { attributes: true, characterData: true, childList: true, subtree: true });
}

export function initCheckIns(document) {
  document.addEventListener("datastar-fetch", (event) => handleCheckInFetch(document, event));
  preserveMatrixState(document);
}

if (typeof window !== "undefined") initCheckIns(window.document);
