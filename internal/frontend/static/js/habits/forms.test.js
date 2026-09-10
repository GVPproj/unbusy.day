import assert from "node:assert/strict";
import test from "node:test";

import { handleCheckInFetch } from "./checkins.js";

let sequence = 0;

async function fixture(t) {
  function target(properties = {}) {
    const listeners = new Map();
    return {
      ...properties,
      addEventListener(type, callback) {
        if (!listeners.has(type)) listeners.set(type, []);
        listeners.get(type).push(callback);
      },
      async dispatchEvent(event) {
        await Promise.all((listeners.get(event.type) || []).map((callback) => callback(event)));
      },
    };
  }
  const shared = { dataset: { state: "saved" }, textContent: "Saved" };
  const status = { dataset: {}, textContent: "" };
  const elements = new Map([
    ["companion-status", shared],
    ["habit-checkin-feedback", status],
    ["habit-start", target({ value: "" })],
    ["habit-create-dialog", target()],
  ]);
  const forms = new Map();
  for (const operation of ["create", "edit", "delete"]) {
    const feedback = { textContent: "" };
    const form = target({
      id: operation === "create" ? "habit-create" : "",
      closest(selector) {
        if (selector === "#habit-create, #habit-edit form, #habit-delete form") return this;
        if (selector === "dialog") return { id: `habit-${operation}` };
        return null;
      },
      querySelector(selector) { return selector === "output" ? feedback : null; },
    });
    forms.set(operation, form);
    elements.set(`habit-${operation}-feedback`, feedback);
  }
  elements.set("habit-create", forms.get("create"));
  const document = target({
    getElementById(id) { return elements.get(id) ?? null; },
    querySelectorAll(selector) { return selector === ".habit-dialog-form" ? [...forms.values()] : []; },
    defaultView: { navigator: { onLine: true } },
  });
  const previous = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", { configurable: true, writable: true, value: document });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "document", previous);
    else delete globalThis.document;
  });
  // A fresh script instance binds each document; its deferred save-status import stays real.
  await import(`./forms.js?fixture=${++sequence}`);
  return {
    document, elements, shared, status,
    form(operation) { return forms.get(operation); },
    fetch(type, el) {
      return document.dispatchEvent({ type: "datastar-fetch", detail: { type, el } });
    },
  };
}

function assertState(f, state) {
  assert.equal(f.shared.dataset.state, state);
  const labels = { saved: /Saved/, saving: /Saving/, failed: /Not confirmed/, offline: /Offline/ };
  assert.match(f.shared.textContent, labels[state]);
}

test("a saved creation event clears the draft so the next open reseeds the start date", async (t) => {
  const f = await fixture(t);
  const form = f.form("create");
  const start = f.elements.get("habit-start");
  const dialog = f.elements.get("habit-create-dialog");
  start.value = "2020-01-01";
  await form.dispatchEvent({ type: "input" });
  await dialog.dispatchEvent({ type: "beforetoggle", newState: "open" });
  assert.equal(start.value, "2020-01-01");
  await dialog.dispatchEvent({ type: "habit-create-saved" });
  await dialog.dispatchEvent({ type: "beforetoggle", newState: "open" });
  const now = new Date();
  assert.equal(start.value, [now.getFullYear(), String(now.getMonth() + 1).padStart(2, "0"),
    String(now.getDate()).padStart(2, "0")].join("-"));
});

for (const operation of ["create", "edit", "delete"]) {
  test(`${operation} tracks fetch lifecycle without changing inline domain feedback`, async (t) => {
    const f = await fixture(t);
    const form = f.form(operation);
    const feedback = f.elements.get(`habit-${operation}-feedback`);
    feedback.textContent = "Invalid habit name.";
    await f.fetch("started", form);
    assertState(f, "saving");
    await f.fetch("finished", form);
    assertState(f, "saved");
    assert.equal(feedback.textContent, "Invalid habit name.");
  });

  for (const failure of ["error", "retries-failed"]) {
    test(`${operation} ${failure} survives finished until a successful retry`, async (t) => {
      const f = await fixture(t);
      const form = f.form(operation);
      await f.fetch("started", form);
      await f.fetch(failure, form);
      assertState(f, "failed");
      await f.fetch("finished", form);
      assertState(f, "failed");
      await f.fetch("started", form);
      assertState(f, "saving");
      await f.fetch("finished", form);
      assertState(f, "saved");
    });
  }
}

test("form retries return to saving and clear after a successful finish", async (t) => {
  const f = await fixture(t);
  const form = f.form("create");
  await f.fetch("started", form);
  await f.fetch("error", form);
  await f.fetch("retrying", form);
  assertState(f, "saving");
  await f.fetch("finished", form);
  assertState(f, "saved");
});

test("offline form failures preserve explicit retry instructions", async (t) => {
  const f = await fixture(t);
  const form = f.form("delete");
  const feedback = f.elements.get("habit-delete-feedback");
  feedback.textContent = "Deletion not confirmed. Try again.";
  f.document.defaultView.navigator.onLine = false;
  await f.fetch("started", form);
  await f.fetch("error", form);
  await f.fetch("finished", form);
  assertState(f, "failed");
  assert.equal(feedback.textContent, "Deletion not confirmed. Try again.");
});

test("form completion cannot hide other form or check-in saves", async (t) => {
  const f = await fixture(t);
  const attrs = new Map([["aria-pressed", "false"]]);
  const button = {
    id: "checkin-1-2026-01-01",
    dataset: { desired: "true", attempt: "1" },
    closest(selector) { return selector === "[data-checkin-action]" ? this : null; },
    getAttribute(name) { return attrs.get(name) ?? null; },
    hasAttribute(name) { return attrs.has(name); },
    setAttribute(name, value) { attrs.set(name, value); },
    removeAttribute(name) { attrs.delete(name); },
  };
  f.elements.set(button.id, button);
  const checkInFetch = (type) => handleCheckInFetch(f.document, { detail: { type, el: button } });
  const create = f.form("create");
  const edit = f.form("edit");
  await f.fetch("started", create);
  await f.fetch("started", edit);
  checkInFetch("started");
  await f.fetch("finished", create);
  assertState(f, "saving");
  await f.fetch("error", edit);
  await f.fetch("finished", edit);
  button.setAttribute("aria-pressed", button.dataset.desired);
  checkInFetch("finished");
  assertState(f, "failed");
  await f.fetch("started", edit);
  await f.fetch("finished", edit);
  assertState(f, "failed");
  assert.match(f.status.textContent, /save not confirmed.*retry/i);
});

test("unrelated fetches do not change shared save state", async (t) => {
  const f = await fixture(t);
  const other = { closest() { return null; } };
  for (const type of ["started", "error", "retries-failed", "finished"]) await f.fetch(type, other);
  assertState(f, "saved");
});
