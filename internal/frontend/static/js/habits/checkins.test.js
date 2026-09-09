import assert from "node:assert/strict";
import test from "node:test";

import { handleCheckInFetch } from "./checkins.js";

function fixture() {
  const status = { dataset: {}, textContent: "" };
  const attrs = new Map();
  const button = {
    id: "checkin-1-2026-01-01",
    dataset: { desired: "true" },
    closest(selector) { return selector === "[data-checkin-action]" ? this : null; },
    getAttribute(name) { return attrs.get(name) ?? null; },
    setAttribute(name, value) { attrs.set(name, value); },
    removeAttribute(name) { attrs.delete(name); },
  };
  const document = {
    getElementById(id) {
      if (id === "habit-checkin-feedback") return status;
      if (id === button.id) return button;
      return null;
    },
  };
  return { attrs, button, document, status };
}

test("check-in fetch shows pending without changing confirmed pressed state", () => {
  const f = fixture();
  f.attrs.set("aria-pressed", "false");
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  assert.equal(f.button.dataset.saveState, "pending");
  assert.equal(f.attrs.get("aria-busy"), "true");
  assert.equal(f.attrs.get("aria-pressed"), "false");
  assert.equal(f.status.textContent, "Saving…");
});

test("failed check-in remains retryable and visibly uncertain", () => {
  const f = fixture();
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  handleCheckInFetch(f.document, { detail: { type: "retrying", el: f.button } });
  assert.match(f.status.textContent, /interrupted.*retrying/i);
  handleCheckInFetch(f.document, { detail: { type: "error", el: f.button } });
  handleCheckInFetch(f.document, { detail: { type: "finished", el: f.button } });
  assert.equal(f.button.dataset.saveState, "failed");
  assert.equal(f.attrs.has("aria-busy"), false);
  assert.match(f.status.textContent, /not saved.*retry/i);
});

test("a lost response is reconciled when live HTML confirms the desired state", () => {
  const f = fixture();
  f.attrs.set("aria-pressed", "false");
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  f.attrs.set("aria-pressed", "true");
  handleCheckInFetch(f.document, { detail: { type: "error", el: f.button } });
  handleCheckInFetch(f.document, { detail: { type: "finished", el: f.button } });
  assert.equal(f.button.dataset.saveState, undefined);
  assert.equal(f.status.textContent, "Saved.");
});

test("successful finish clears pending after the live state arrives", () => {
  const f = fixture();
  f.attrs.set("aria-pressed", "false");
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  f.status.dataset.result = "committed";
  f.status.textContent = "Saved; syncing…";
  f.attrs.set("aria-pressed", "true");
  handleCheckInFetch(f.document, { detail: { type: "finished", el: f.button } });
  assert.equal(f.button.dataset.saveState, undefined);
  assert.equal(f.status.textContent, "Saved.");
});

test("a new attempt clears a stale rejected result", () => {
  const f = fixture();
  f.status.dataset.result = "rejected";
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  handleCheckInFetch(f.document, { detail: { type: "error", el: f.button } });
  handleCheckInFetch(f.document, { detail: { type: "finished", el: f.button } });
  assert.equal(f.button.dataset.saveState, "failed");
});

test("a rejected write clears pending without changing checked state", () => {
  const f = fixture();
  f.attrs.set("aria-pressed", "false");
  handleCheckInFetch(f.document, { detail: { type: "started", el: f.button } });
  f.status.dataset.result = "rejected";
  handleCheckInFetch(f.document, { detail: { type: "finished", el: f.button } });
  assert.equal(f.button.dataset.saveState, undefined);
  assert.equal(f.attrs.get("aria-pressed"), "false");
});
