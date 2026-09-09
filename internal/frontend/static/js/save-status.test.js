import assert from "node:assert/strict";
import test from "node:test";
import { setSaveState } from "./save-status.js";

function fixture() {
  const output = { dataset: { state: "saved" }, textContent: "Saved" };
  const document = { getElementById: (id) => id === "companion-status" ? output : null };
  return { document, output };
}

test("a completed habit save cannot hide pending Jotpad edits", () => {
  const { document, output } = fixture();
  setSaveState(document, "jot", "saving");
  setSaveState(document, "checkin-1", "saving");
  setSaveState(document, "checkin-1", "saved");
  assert.equal(output.textContent, "Saving…");
  setSaveState(document, "jot", "saved");
  assert.equal(output.textContent, "Saved");
});

test("failures survive unrelated saves and clear when their own writer recovers", () => {
  const { document, output } = fixture();
  setSaveState(document, "checkin-1", "failed");
  setSaveState(document, "jot", "saved");
  assert.equal(output.dataset.state, "failed");
  setSaveState(document, "jot", "offline");
  assert.equal(output.dataset.state, "offline");
  setSaveState(document, "jot", "saved");
  assert.equal(output.dataset.state, "failed");
  setSaveState(document, "checkin-1", "saving");
  assert.equal(output.dataset.state, "saving");
  setSaveState(document, "checkin-1", "saved");
  assert.equal(output.dataset.state, "saved");
});

test("separate documents do not share save state", () => {
  const a = fixture();
  const b = fixture();
  setSaveState(a.document, "jot", "offline");
  setSaveState(b.document, "jot", "saved");
  assert.equal(a.output.dataset.state, "offline");
  assert.equal(b.output.dataset.state, "saved");
});
