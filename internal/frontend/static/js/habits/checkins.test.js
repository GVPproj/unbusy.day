import assert from "node:assert/strict";
import test from "node:test";

import { handleCheckInFetch, initCheckIns } from "./checkins.js";

function fixture() {
  const listeners = new Map();
  const reads = [];
  let sequence = 0;
  const grid = { dataset: { read: "0", view: "0", month: "2026-01" } };
  const status = { dataset: {}, textContent: "" };
  const shared = { dataset: { state: "saved" }, textContent: "Saved" };
  const elements = new Map([
    ["habit-checkin-feedback", status],
    ["companion-status", shared],
    ["habit-grid", grid],
    ["habit-matrix", { id: "habit-matrix", scrollLeft: 0, addEventListener() {}, dispatchEvent(event) { reads.push(event.detail.read); } }],
  ]);
  let reconcile;
  const document = {
    getElementById(id) { return elements.get(id) ?? null; },
    addEventListener(type, callback) { listeners.set(type, callback); },
    defaultView: {
      CustomEvent: class { constructor(type, options) { this.type = type; this.detail = options.detail; } },
      navigator: { onLine: true },
      MutationObserver: class {
        constructor(callback) { reconcile = callback; }
        observe() {}
      },
    },
  };
  function addButton(id = "checkin-1-2026-01-01") {
    const attrs = new Map([["aria-pressed", "false"]]);
    const button = {
      id,
      dataset: { desired: "true" },
      closest(selector) { return selector === "[data-checkin-action]" ? this : null; },
      getAttribute(name) { return attrs.get(name) ?? null; },
      hasAttribute(name) { return attrs.has(name); },
      setAttribute(name, value) { attrs.set(name, value); },
      removeAttribute(name) { attrs.delete(name); },
    };
    elements.set(id, button);
    return button;
  }
  const button = addButton();
  initCheckIns(document);
  return {
    button, document, status, shared, elements, addButton, grid, reads,
    reconcile() { reconcile(); },
    click(el = button) {
      let prevented = false;
      listeners.get("click")?.({ target: el, preventDefault() { prevented = true; }, stopImmediatePropagation() {} });
      return prevented;
    },
    ack(result = "committed", el = button) {
      listeners.get("datastar-signal-patch")?.({ detail: { _habitcheckinack: `${el.dataset.attempt}:${result}` } });
    },
    render(read = reads.at(-1)) { grid.dataset.read = String(read); reconcile(); },
    fetch(type, el = button) {
      if (type === "started" && el.dataset) el.dataset.attempt = String(++sequence);
      handleCheckInFetch(document, { detail: { type, el } });
    },
  };
}

function assertState(f, state) {
  assert.equal(f.shared.dataset.state, state);
  const labels = { saved: /Saved/, saving: /Saving/, failed: /Not confirmed/, offline: /Offline/ };
  assert.match(f.shared.textContent, labels[state]);
}

function confirm(button) {
  button.setAttribute("aria-pressed", button.dataset.desired);
}

test("committed recovery requests a fresh read instead of activating the inverse toggle", () => {
  const f = fixture();
  f.fetch("started");
  confirm(f.button);
  f.ack();
  f.fetch("finished");
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started", matrix);
  f.fetch("finished", matrix);
  assert.match(f.status.textContent, /saved.*refresh/i);
  assert.doesNotMatch(f.shared.textContent, /not saved/i);
  assert.equal(f.click(), true);
  assert.deepEqual(f.reads, [1, 2]);
  assertState(f, "saving");
  f.button.setAttribute("aria-pressed", "false");
  f.render(1);
  assertState(f, "saving");
  f.render(2);
  assertState(f, "saved");
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
});

test("a failed read remains recoverable while the receipt stream is still open", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started", matrix);
  f.fetch("finished", matrix);
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.equal(f.click(), true);
  assert.deepEqual(f.reads, [1, 2]);
  f.render(2);
  assertState(f, "saved");
  f.fetch("finished");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
});

test("a post-commit month read settles a check-in superseded by another device", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  f.fetch("finished");
  assertState(f, "saving");
  f.render(1);
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.equal(f.button.dataset.saveState, undefined);
  assertState(f, "saved");
});

test("a month morph cannot lose the submitting fetch before finished", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  delete f.button.dataset.attempt;
  f.render();
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  f.fetch("finished");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assertState(f, "saved");
});

test("matching values and unrelated reads cannot replace a post-commit read", () => {
  const f = fixture();
  f.fetch("started");
  confirm(f.button);
  f.render(0);
  assertState(f, "saving");
  f.ack();
  f.fetch("finished");
  f.render(0);
  assertState(f, "saving");
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  f.render(1);
  assertState(f, "saved");
});

test("concurrent cells require their own receipt and a sufficiently new read", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  f.ack();
  f.fetch("finished");
  f.render(1);
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.equal(other.getAttribute("aria-busy"), "true");
  assertState(f, "saving");
  f.ack("committed", other);
  f.fetch("finished", other);
  f.render(1);
  assertState(f, "saving");
  f.render(2);
  assertState(f, "saved");
  assert.deepEqual(f.reads, [1, 2]);
});

test("a correlated read cannot clear another cell's unacknowledged failure", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  f.fetch("error", other);
  f.fetch("finished", other);
  f.ack();
  f.fetch("finished");
  f.render();
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.equal(other.dataset.saveState, "failed");
  assertState(f, "failed");
});

test("read recovery and unknown write recovery remain independent across dates", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  f.fetch("error", other);
  f.fetch("finished", other);
  f.ack();
  f.fetch("finished");
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started", matrix);
  f.fetch("finished", matrix);
  assert.match(f.status.textContent, /saved, but refresh not confirmed/i);
  assert.match(f.status.textContent, /save not confirmed/i);
  assert.equal(f.click(), true);
  f.render(2);
  assertState(f, "failed");
  assert.equal(other.dataset.saveState, "failed");
  other.dataset.desired = "false";
  assert.equal(f.click(other), false);
  assert.equal(other.dataset.desired, "true");
  f.fetch("started", other);
  f.ack("committed", other);
  f.fetch("finished", other);
  f.render(3);
  assertState(f, "saved");
  assert.deepEqual(f.reads, [1, 2, 3]);
});

test("another cell's rejection cannot settle a committed check-in", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  f.ack();
  f.status.dataset.result = "rejected";
  f.ack("rejected", other);
  f.fetch("finished", other);
  f.fetch("finished");
  assert.equal(other.hasAttribute("aria-busy"), false);
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  assertState(f, "saving");
  f.render();
  assertState(f, "saved");
});

for (const failure of ["error", "retries-failed"]) {
  test(`post-commit month ${failure} releases busy and reconnect reconciles the last committed value`, () => {
    const f = fixture();
    f.fetch("started");
    f.ack();
    f.fetch("finished");
    const matrix = { id: "habit-matrix" };
    f.fetch(failure, matrix);
    f.fetch("finished", matrix);
    assertState(f, "failed");
    assert.equal(f.button.hasAttribute("aria-busy"), false);
    assert.equal(f.button.getAttribute("aria-pressed"), "false");
    f.fetch("started", matrix);
    assertState(f, "saving");
    f.render();
    assertState(f, "saved");
    assert.equal(f.button.dataset.saveState, undefined);
  });
}

test("a completed month read without an authoritative patch releases busy for retry", () => {
  const f = fixture();
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started");
  f.ack();
  f.fetch("finished");
  f.fetch("started", matrix);
  f.fetch("finished", matrix);
  assertState(f, "failed");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.match(f.status.textContent, /retry/i);
});

test("an older canceled read finishing cannot fail a newer read still in flight", () => {
  const f = fixture();
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started");
  f.ack();
  f.fetch("finished");
  f.fetch("started", matrix);
  f.fetch("started", matrix);
  f.fetch("finished", matrix);
  assertState(f, "saving");
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  f.render();
  f.fetch("finished", matrix);
  assertState(f, "saved");
});

test("an older canceled read error cannot fail a newer read still in flight", () => {
  const f = fixture();
  const matrix = f.elements.get("habit-matrix");
  f.fetch("started");
  f.ack();
  f.fetch("finished");
  f.fetch("started", matrix);
  f.fetch("started", matrix);
  f.fetch("error", matrix);
  f.fetch("finished", matrix);
  assertState(f, "saving");
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  f.render();
  f.fetch("finished", matrix);
  assertState(f, "saved");
});

test("a received commit receipt survives a lost POST response and supersession", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  f.fetch("error");
  f.fetch("finished");
  assertState(f, "failed");
  f.render();
  assertState(f, "saved");
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
});

test("a superseded value without a commit receipt remains retryable, not falsely saved", () => {
  const f = fixture();
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  f.render(10);
  assertState(f, "failed");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
});

test("a confirmed attempt stays settled when superseded before POST finished", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  confirm(f.button);
  f.render();
  f.button.setAttribute("aria-pressed", "false");
  f.reconcile();
  f.fetch("finished");
  assertState(f, "saved");
  assert.equal(f.button.hasAttribute("aria-busy"), false);
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
});

test("a retry ignores the old attempt's receipt and fetch lifecycle", () => {
  const f = fixture();
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  const retry = f.addButton(f.button.id);
  f.fetch("started", retry);
  f.ack();
  f.fetch("finished");
  assert.deepEqual(f.reads, []);
  assertState(f, "saving");
  f.ack("committed", retry);
  f.fetch("finished", retry);
  f.render();
  assertState(f, "saved");
});

test("duplicate and abandoned receipts do not trigger unrelated reads", () => {
  const f = fixture();
  f.fetch("started");
  f.ack();
  f.ack();
  f.fetch("finished");
  f.render();
  f.ack();
  assert.deepEqual(f.reads, [1]);
  assertState(f, "saved");
});

test("check-in fetch shows shared pending without changing confirmed pressed state", () => {
  const f = fixture();
  f.fetch("started");
  assert.equal(f.button.dataset.saveState, "pending");
  assert.equal(f.button.getAttribute("aria-busy"), "true");
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
  assertState(f, "saving");
  assert.equal(f.status.textContent, "");
});

for (const failure of ["error", "retries-failed"]) {
  test(`${failure} remains retryable with shared failure status after finished`, () => {
    const f = fixture();
    f.fetch("started");
    f.fetch("retrying");
    assertState(f, "saving");
    assert.equal(f.status.textContent, "");
    f.fetch(failure);
    f.fetch("finished");
    assertState(f, "failed");
    assert.equal(f.button.dataset.saveState, "failed");
    assert.equal(f.button.hasAttribute("aria-busy"), false);
    assert.match(f.status.textContent, /save not confirmed.*retry/i);
    f.fetch("started");
    assertState(f, "saving");
    assert.equal(f.status.textContent, "");
    confirm(f.button);
    f.ack();
    f.fetch("finished");
    f.render();
    assertState(f, "saved");
  });
}

test("offline check-in failures require explicit retry", () => {
  const f = fixture();
  f.document.defaultView.navigator.onLine = false;
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  assertState(f, "failed");
  assert.match(f.status.textContent, /retry/i);
});

for (const desired of ["true", "false"]) {
  test(`unknown outcomes preserve desired ${desired} even after matching live HTML`, () => {
    const f = fixture();
    f.button.dataset.desired = desired;
    f.fetch("started");
    confirm(f.button);
    f.button.dataset.desired = desired === "true" ? "false" : "true";
    f.fetch("error");
    f.fetch("finished");
    assertState(f, "failed");
    assert.match(f.status.textContent, /save not confirmed/i);
    assert.equal(f.click(), false);
    assert.equal(f.button.dataset.desired, desired);
    assert.deepEqual(f.reads, []);
  });
}

test("matching live HTML does not confirm a lost receipt", () => {
  const f = fixture();
  f.fetch("started");
  confirm(f.button);
  f.fetch("error");
  f.fetch("finished");
  assert.equal(f.button.dataset.saveState, "failed");
  assertState(f, "failed");
  assert.match(f.status.textContent, /save not confirmed/i);
});

test("a delayed receipt and its read reconcile a failure after a matching morph", () => {
  const f = fixture();
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  assertState(f, "failed");
  const replacement = f.addButton(f.button.id);
  confirm(replacement);
  f.reconcile();
  assertState(f, "failed");
  f.ack();
  f.render();
  assertState(f, "saved");
  assert.equal(replacement.dataset.saveState, undefined);
  assert.equal(replacement.hasAttribute("aria-busy"), false);
  assert.equal(f.status.textContent, "");
});

test("successful finish waits for authoritative state without local success text", () => {
  const f = fixture();
  f.fetch("started");
  f.status.dataset.result = "committed";
  f.ack();
  f.fetch("finished");
  assertState(f, "saving");
  assert.equal(f.status.textContent, "");
  confirm(f.button);
  f.render();
  assert.equal(f.button.dataset.saveState, undefined);
  assertState(f, "saved");
  assert.equal(f.status.textContent, "");
});

test("one confirmed check-in cannot hide another pending check-in", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  confirm(f.button);
  f.ack();
  f.fetch("finished");
  f.render();
  assertState(f, "saving");
  assert.equal(f.status.textContent, "");
  confirm(other);
  f.ack("committed", other);
  f.fetch("finished", other);
  f.render();
  assertState(f, "saved");
});

test("a concurrent success preserves another check-in's failure and retry instruction", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("started", other);
  f.fetch("error");
  f.fetch("finished");
  confirm(other);
  f.ack("committed", other);
  f.fetch("finished", other);
  f.render();
  assertState(f, "failed");
  assert.match(f.status.textContent, /retry/i);
  confirm(f.button);
  f.ack();
  f.render();
  assertState(f, "saved");
  assert.equal(f.status.textContent, "");
});

test("reconciling a failure preserves another pending check-in", () => {
  const f = fixture();
  const other = f.addButton("checkin-1-2026-01-02");
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  f.fetch("started", other);
  assertState(f, "failed");
  confirm(f.button);
  f.ack();
  f.render();
  assertState(f, "saving");
  assert.equal(f.status.textContent, "");
});

test("a new attempt clears a stale rejected result", () => {
  const f = fixture();
  f.status.dataset.result = "rejected";
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  assert.equal(f.button.dataset.saveState, "failed");
  assertState(f, "failed");
});

test("unknown retry intent survives navigating away and returning to the date", () => {
  const f = fixture();
  f.fetch("started");
  f.elements.delete(f.button.id);
  f.fetch("error");
  f.fetch("finished");
  const returned = f.addButton(f.button.id);
  returned.setAttribute("aria-pressed", "true");
  returned.dataset.desired = "false";
  f.reconcile();
  assertState(f, "failed");
  assert.equal(f.click(returned), false);
  assert.equal(returned.dataset.desired, "true");
});

test("a response from a month that was left does not alter the new month", () => {
  const f = fixture();
  f.fetch("started");
  f.button.isConnected = false;
  f.elements.delete(f.button.id);
  f.status.textContent = "Current month status";
  f.reconcile();
  assertState(f, "saving");
  f.fetch("finished");
  assert.equal(f.status.textContent, "Current month status");
  assertState(f, "saved");
});

test("a failed response from an abandoned month cannot pin shared failure", () => {
  const f = fixture();
  f.fetch("started");
  f.button.isConnected = false;
  f.elements.delete(f.button.id);
  f.fetch("error");
  f.fetch("finished");
  assertState(f, "saved");
});

test("matrix removal clears abandoned check-in sources", () => {
  const f = fixture();
  f.fetch("started");
  f.fetch("error");
  f.fetch("finished");
  f.elements.delete(f.button.id);
  f.reconcile();
  assertState(f, "saved");
});

test("a rejected write clears pending and preserves inline validation", () => {
  const f = fixture();
  f.fetch("started");
  f.status.dataset.result = "rejected";
  f.status.textContent = "This date is before the habit start date.";
  f.ack("rejected");
  f.fetch("finished");
  f.reconcile();
  assert.equal(f.button.dataset.saveState, undefined);
  assert.equal(f.button.getAttribute("aria-pressed"), "false");
  assert.equal(f.status.textContent, "This date is before the habit start date.");
  assertState(f, "saved");
});

test("unrelated fetches do not change shared save state", () => {
  const f = fixture();
  const other = { closest() { return null; } };
  for (const type of ["started", "error", "retries-failed", "finished"]) f.fetch(type, other);
  assertState(f, "saved");
});
