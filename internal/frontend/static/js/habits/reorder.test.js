import test from "node:test";
import assert from "node:assert/strict";
import { commitHabitOrder, initHabitReorder } from "./reorder.js";

class TestCustomEvent {
  constructor(type, options = {}) {
    this.type = type;
    this.detail = options.detail;
  }
}

function fixture() {
  const listeners = new Map();
  const documentListeners = new Map();
  class Observer {
    observe() {}
    disconnect() {}
    takeRecords() { return []; }
  }
  const document = {
    activeElement: null,
    body: {},
    defaultView: { CustomEvent: TestCustomEvent, MutationObserver: Observer },
    addEventListener(type, fn) { documentListeners.set(type, fn); },
    removeEventListener(type) { documentListeners.delete(type); },
    getElementById(id) { return rows.find((row) => row.id === id) || null; },
    createElement() { return { innerHTML: "", content: { querySelector() { return null; } } }; },
  };
  const tbody = {
    children: [],
    insertBefore(row, before) {
      this.children = this.children.filter((candidate) => candidate !== row);
      const index = before ? this.children.indexOf(before) : -1;
      if (index < 0) this.children.push(row);
      else this.children.splice(index, 0, row);
    },
    append(row) { this.insertBefore(row, null); },
  };
  const makeRow = (id, name, sortOrder) => ({
    id: `habit-${id}`,
    dataset: { sortOrder: String(sortOrder) },
    parentElement: tbody,
    ownerDocument: document,
    classList: { add() {}, remove() {} },
    querySelector(selector) { return selector === ".habit-name" ? { textContent: name } : null; },
    focus() { document.activeElement = this; },
    closest(selector) { return selector === "tr[data-sort-order]" ? this : null; },
    getBoundingClientRect() { return { top: sortOrder * 40, height: 40 }; },
  });
  const rows = [makeRow(11, "Read", 4), makeRow(22, "Walk", 9), makeRow(33, "Write", 15)];
  tbody.children = rows.slice();
  const classes = new Set();
  let captured = null;
  const matrix = {
    ownerDocument: document,
    classList: {
      add(name) { classes.add(name); },
      remove(name) { classes.delete(name); },
      contains(name) { return classes.has(name); },
    },
    setPointerCapture(id) { captured = id; },
    hasPointerCapture(id) { return captured === id; },
    releasePointerCapture() { captured = null; },
    dataset: { reorderable: "true" },
    events: [],
    addEventListener(type, fn) { listeners.set(type, fn); },
    dispatchEvent(event) { this.events.push(event); },
    querySelectorAll() { return tbody.children.slice(); },
    querySelector(selector) { return selector === "#habit-grid" ? this : null; },
  };
  return { matrix, rows, tbody, listeners, document };
}

test("commit keeps the dropped order visible while awaiting the authoritative patch", () => {
  const { matrix, rows, tbody } = fixture();
  tbody.children = [rows[2], rows[0], rows[1]];
  const said = [];
  const committed = commitHabitOrder(matrix, rows[2], [11, 22, 33], (message) => said.push(message), false);

  assert.equal(committed, true);
  assert.deepEqual(tbody.children.map((row) => row.id), ["habit-33", "habit-11", "habit-22"]);
  assert.deepEqual(tbody.children.map((row) => row.dataset.sortOrder), ["0", "1", "2"]);
  assert.deepEqual(said, ["Moved Write to position 1 of 3."]);
  assert.equal(matrix.events[0].type, "reorder");
  assert.deepEqual(matrix.events[0].detail.order, [
    { id: 33, sortOrder: 0 },
    { id: 11, sortOrder: 1 },
    { id: 22, sortOrder: 2 },
  ]);
});

test("commit ignores unchanged and detached gestures", () => {
  const { matrix, rows } = fixture();
  assert.equal(commitHabitOrder(matrix, rows[0], [11, 22, 33], () => assert.fail("announced"), false), false);
  rows[0].parentElement = null;
  assert.equal(commitHabitOrder(matrix, rows[0], [22, 11, 33], () => assert.fail("announced"), false), false);
  assert.equal(matrix.events.length, 0);
});

test("keyboard reorder commits through the shared order path", () => {
  const { matrix, rows, listeners, tbody, document } = fixture();
  initHabitReorder(matrix, () => {});
  let prevented = false;
  listeners.get("keydown")({
    altKey: true,
    key: "ArrowUp",
    target: rows[1],
    preventDefault() { prevented = true; },
  });
  assert.equal(prevented, true);
  assert.deepEqual(tbody.children.map((row) => row.id), ["habit-22", "habit-11", "habit-33"]);
  assert.deepEqual(matrix.events[0].detail.order.map(({ id }) => id), [22, 11, 33]);
  assert.equal(matrix.events[0].type, "reorder");
  assert.equal(document.activeElement, rows[1]);
});

test("historical grids remain read-only", () => {
  const { matrix, rows, listeners, tbody } = fixture();
  matrix.dataset.reorderable = "false";
  initHabitReorder(matrix, () => {});
  listeners.get("keydown")({
    altKey: true,
    key: "End",
    target: rows[0],
    preventDefault() { assert.fail("historical reorder was handled"); },
  });
  assert.deepEqual(tbody.children.map((row) => row.id), ["habit-11", "habit-22", "habit-33"]);
  assert.equal(matrix.events.length, 0);
});

test("the pointer path owns arbitration from pointerdown until cancellation", () => {
  const { matrix, rows, listeners } = fixture();
  const arb = initHabitReorder(matrix, () => {});
  const header = {
    closest(selector) { return selector === "tr[data-sort-order]" ? rows[0] : null; },
    setPointerCapture() {},
  };
  const target = {
    closest(selector) {
      if (selector === ".habit-action") return null;
      if (selector === "tbody th[scope='row']") return header;
      return null;
    },
  };
  listeners.get("pointerdown")({
    button: 0,
    pointerId: 5,
    clientX: 10,
    clientY: 20,
    target,
    preventDefault() {},
    stopPropagation() {},
  });
  assert.equal(arb.pointer.isActive(), true);
  assert.equal(matrix.classList.contains("reordering"), true);
  assert.equal(matrix.hasPointerCapture(5), true);
  listeners.get("keydown")({
    altKey: true,
    key: "End",
    target: rows[0],
    preventDefault() { assert.fail("keyboard stole an active pointer gesture"); },
  });
  assert.equal(matrix.events.length, 0);
  arb.pointer.cancel();
  assert.equal(arb.pointer.isActive(), false);
  assert.equal(matrix.classList.contains("reordering"), false);
  assert.equal(matrix.hasPointerCapture(5), false);
});
