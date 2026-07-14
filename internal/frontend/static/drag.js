// Motion-powered drag + stretch for the #block-list day grid: transforms the
// real <li> (no ghost), previews the push cascade live (push.js, ADR 0005),
// commits as a FLIP, then dispatches one `layout` event with the full layout.
// Listeners are delegated to #block-list so wiring survives morphs.
import {
  animate,
  motionValue,
  styleEffect,
} from "https://cdn.jsdelivr.net/npm/motion@12.40.0/+esm";
import { pushLayout } from "./push.js";
import { keyboardLayout, normalizeKey } from "./keys.js";

const list = document.getElementById("block-list");
// Assertive live region outside #block-list so it survives every morph.
const announcer = document.getElementById("sr-announce");
const SPRING = { type: "spring", stiffness: 600, damping: 38 };

const blocksIn = () =>
  [...list.children].filter((c) => c.classList.contains("block-item"));
const boundsNow = () => ({
  start: parseInt(list.dataset.dayStart, 10),
  end: parseInt(list.dataset.dayEnd, 10),
});
const placementOf = (c) => ({
  id: c.dataset.id,
  slot: parseInt(c.dataset.slot, 10),
  span: parseInt(c.dataset.span, 10) || 1,
});
const layoutIn = () => blocksIn().map(placementOf);

// Row pitch measured from consecutive slot rows.
function slotPitch() {
  const slots = [...list.querySelectorAll(":scope > .slot")];
  if (slots.length > 1)
    return (
      slots[1].getBoundingClientRect().top -
      slots[0].getBoundingClientRect().top
    );
  return slots[0].getBoundingClientRect().height;
}

// Write a committed layout into the persisted attributes and grid placement —
// the same shape the server renders, so the patch lands as a no-op morph.
function writeLayout(layout, dayStart) {
  const by = new Map(layout.map((p) => [p.id, p]));
  for (const c of blocksIn()) {
    const p = by.get(c.dataset.id);
    if (!p) continue;
    c.dataset.slot = p.slot;
    c.dataset.span = p.span;
    c.style.setProperty("--span", p.span);
    c.style.gridRow = p.slot - dayStart + 1 + " / span " + p.span;
  }
}

const sameLayout = (a, b) =>
  a.every((p) => {
    const q = b.find((x) => x.id === p.id);
    return q && q.slot === p.slot && q.span === p.span;
  });

let drag = null;
let resize = null;
let grab = null; // active keyboard move (grab → arrow → drop), null when idle
let kresize = null; // active keyboard resize on a focused grip, null when idle
let settling = false;

// Past this many px the gesture is a drag, not a tap-to-edit.
const TAP_SLOP = 4;

list.addEventListener("pointerdown", (e) => {
  if (drag || resize || settling || e.button !== 0) return;
  // A pointer gesture supersedes an in-progress keyboard grab/resize.
  if (grab) cancelGrab();
  if (kresize) cancelKbResize();
  // A label in edit mode owns its own pointer (caret/selection).
  if (e.target.closest(".block-label[contenteditable]")) return;
  const el = e.target.closest(".block-item");
  if (!el || el.parentElement !== list) return;
  e.preventDefault();
  el.setPointerCapture(e.pointerId);
  if (e.target.closest(".grip")) startResize(e, el);
  else startDrag(e, el);
});

list.addEventListener("pointermove", (e) => {
  if (drag && e.pointerId === drag.pointerId) {
    drag.lastX = e.clientX;
    drag.lastY = e.clientY;
    applyDrag();
    autoScroll(e.clientY);
  } else if (resize && e.pointerId === resize.pointerId) {
    previewResize(
      resize.orig.span + Math.round((e.clientY - resize.startY) / resize.pitch),
    );
  }
});

list.addEventListener("pointerup", (e) => {
  settleDrag(e, true);
  settleResize(e, true);
});
list.addEventListener("pointercancel", (e) => {
  settleDrag(e, false);
  settleResize(e, false);
});

// Keyboard move. Only the block body is the move tab stop — the grip, delete
// button, and rename editor own their own keys. Arrows are inert until a block
// is grabbed, so they never fight focus navigation.
list.addEventListener("keydown", (e) => {
  if (drag || resize || settling) return;
  const grip = e.target.closest(".grip");
  if (grip && grip.closest(".block-item")?.parentElement === list) {
    handleResizeKey(e, grip);
    return;
  }
  const el = e.target.closest(".block-item");
  if (!el || el.parentElement !== list || e.target !== el) return;
  if (!grab) {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault();
      startGrab(el);
    } else if (e.key === "F2" || e.key === "r") {
      // F2 is the established rename-in-place key; `r` is the vim-flavored
      // alias. Enter is taken by grab/drop.
      e.preventDefault();
      const label = el.querySelector(".block-label");
      if (label) enterEdit(label);
    } else if (e.key === "Backspace" || e.key === "Delete" || e.key === "d") {
      // Calendar/canvas convention; `d` is the vim alias. Immediate, no confirm
      // — the server re-render is the only safety net, like the mouse control.
      e.preventDefault();
      deleteBlock(el);
    }
    return;
  }
  const moveKey = normalizeKey(e.key);
  if (moveKey === "ArrowUp" || moveKey === "ArrowDown") {
    e.preventDefault();
    moveGrab(e.key);
  } else if (e.key === " " || e.key === "Enter") {
    e.preventDefault();
    dropGrab();
  } else if (e.key === "Escape") {
    e.preventDefault();
    cancelGrab();
  }
});

// Focus leaving a grabbed block abandons the move; focus leaving a resizing
// grip COMMITS (the splitter convention — blur saves).
list.addEventListener("focusout", (e) => {
  if (grab && e.target === grab.el) cancelGrab();
  if (kresize && e.target === kresize.grip) commitKbResize(false);
});

// Per-gesture motionValues for every other block, NOT animate(element, …):
// teardown wipes style.transform behind Motion's back, and animate(element)
// would resume from Motion's stale cached y and teleport a sibling.
function sibsFor(el) {
  const sibs = new Map();
  for (const c of blocksIn()) {
    if (c === el) continue;
    const y = motionValue(0);
    // h0 is the natural height (margins put it under span*pitch); springing
    // from here keeps unchanged siblings from twitching.
    const h0 = c.getBoundingClientRect().height;
    const h = motionValue(h0);
    sibs.set(c, {
      y,
      h,
      h0,
      detach: styleEffect(c, { y, height: h }),
      yAnim: null,
      hAnim: null,
    });
  }
  return sibs;
}

// Spring every other block to its slot delta (and, under compression, its
// height) for the live preview layout `lay`.
function springSibs(g, lay) {
  const by = new Map(lay.map((p) => [p.id, p]));
  g.sibs.forEach((s, c) => {
    const p = by.get(c.dataset.id);
    const fromSlot = parseInt(c.dataset.slot, 10);
    const fromSpan = parseInt(c.dataset.span, 10) || 1;
    if (s.yAnim) s.yAnim.stop();
    s.yAnim = animate(s.y, (p.slot - fromSlot) * g.pitch, SPRING);
    // keep the natural margin (fromSpan*pitch - h0) when springing to a new span
    const toH = p.span === fromSpan ? s.h0 : p.span * g.pitch - (fromSpan * g.pitch - s.h0);
    if (s.hAnim) s.hAnim.stop();
    s.hAnim = animate(s.h, toH, SPRING);
  });
}

// Stop sibling springs and clear the styles Motion left behind.
function teardownSibs(g) {
  g.sibs.forEach((s, c) => {
    if (s.yAnim) s.yAnim.stop();
    if (s.hAnim) s.hAnim.stop();
    s.detach();
    c.style.transform = "";
    c.style.height = "";
  });
}

const sibAnims = (g) => [...g.sibs.values()].flatMap((s) => [s.yAnim, s.hAnim]);

// Tell the server the gesture's result — unless nothing changed, or a foreign
// patch replaced the block mid-gesture (the server's layout already won).
function dispatchLayout(g) {
  if (sameLayout(g.valid.layout, g.current)) return;
  if (g.el.parentElement !== list) return;
  list.dispatchEvent(
    new CustomEvent("layout", { detail: { layout: g.valid.layout } }),
  );
}

// ---- drag to a slot --------------------------------------------------

function startDrag(e, el) {
  const orig = placementOf(el);
  const bounds = boundsNow();
  const pitch = slotPitch();
  const x = motionValue(0);
  const y = motionValue(0);
  drag = {
    el,
    orig,
    x,
    y,
    bounds,
    current: layoutIn(),
    pitch,
    // Clamp: a translate past the grid grows the scroll container's overflow
    // without bound.
    minY: (bounds.start - orig.slot) * pitch,
    maxY: (bounds.end - orig.span - orig.slot) * pitch,
    detach: styleEffect(el, { x, y }),
    sibs: sibsFor(el),
    // last layout the cascade accepted — what an invalid pointer snaps to
    valid: { slot: orig.slot, layout: layoutIn() },
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    lastX: e.clientX,
    lastY: e.clientY,
    scrollV: 0,
    raf: 0,
    moved: false,
    downTarget: e.target,
  };
  el.classList.add("dragging");
}

// Split out of pointermove so the auto-scroll loop can re-apply the transform
// each frame (the pointer is stationary while the container scrolls under it).
function applyDrag() {
  const d = drag;
  const dx = d.lastX - d.startX;
  const dy = d.lastY - d.startY;
  if (Math.abs(dx) > TAP_SLOP || Math.abs(dy) > TAP_SLOP) d.moved = true;
  const y = Math.max(d.minY, Math.min(d.maxY, dy));
  d.x.set(dx);
  d.y.set(y);
  previewDrag(d.orig.slot + Math.round(y / d.pitch));
}

// Edge auto-scroll. Speed ramps with depth into the EDGE band; the rAF loop
// self-sustains while the pointer holds at an edge (no pointermove fires).
const EDGE = 48; // px band at each edge that triggers auto-scroll
const MAX_SPEED = 16; // px per frame at the very edge

function autoScroll(clientY) {
  const d = drag;
  if (!d) return;
  const r = list.getBoundingClientRect();
  let v = 0;
  if (clientY < r.top + EDGE) v = -((r.top + EDGE - clientY) / EDGE) * MAX_SPEED;
  else if (clientY > r.bottom - EDGE)
    v = ((clientY - (r.bottom - EDGE)) / EDGE) * MAX_SPEED;
  d.scrollV = Math.max(-MAX_SPEED, Math.min(MAX_SPEED, v));
  if (d.scrollV && !d.raf) d.raf = requestAnimationFrame(scrollTick);
}

// One auto-scroll frame: scroll, then fold the scrolled distance into startY
// so the held block keeps tracking the stationary pointer.
function scrollTick() {
  const d = drag;
  if (!d) return;
  d.raf = 0;
  if (!d.scrollV) return;
  const before = list.scrollTop;
  list.scrollTop += d.scrollV;
  const moved = list.scrollTop - before;
  if (!moved) return;
  d.startY -= moved;
  applyDrag();
  d.raf = requestAnimationFrame(scrollTick);
}

// Preview the cascade at `slot`: clamp into the day, keep the last valid
// layout when the push rejects, so invalid drops snap to legal positions.
function previewDrag(slot) {
  const d = drag;
  slot = Math.max(d.bounds.start, Math.min(d.bounds.end - d.orig.span, slot));
  if (slot === d.valid.slot) return;
  const lay = pushLayout(d.bounds, d.current, {
    id: d.orig.id,
    slot,
    span: d.orig.span,
  });
  if (!lay) return;
  d.valid = { slot, layout: lay };
  springSibs(d, lay);
}

async function settleDrag(e, commit) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  const d = drag;
  // A tap (no movement) on the label opens the inline editor after the settle.
  const editLabel =
    commit && !d.moved ? d.downTarget.closest(".block-label") : null;
  if (!commit) d.valid = { slot: d.orig.slot, layout: d.current };
  if (d.raf) cancelAnimationFrame(d.raf);
  springSibs(d, d.valid.layout);
  drag = null;
  settling = true;
  try {
    await Promise.all([
      animate(d.x, 0, SPRING),
      animate(d.y, (d.valid.slot - d.orig.slot) * d.pitch, SPRING),
      ...sibAnims(d),
    ]);
  } finally {
    // Teardown and the layout write share one synchronous frame: same pixels,
    // new grid placement (FLIP).
    d.detach();
    teardownSibs(d);
    d.el.style.transform = "";
    d.el.classList.remove("dragging");
    if (d.el.parentElement === list)
      writeLayout(d.valid.layout, d.bounds.start);
    settling = false;
  }
  if (editLabel && d.el.parentElement === list) {
    enterEdit(editLabel, d.startX, d.startY);
    return;
  }
  dispatchLayout(d);
}

// ---- inline label edit -----------------------------------------------

// Turn a label into a plaintext editor: commit on blur or Enter, revert on
// Escape. A pointer tap passes the click point; keyboard entry (F2) passes
// none — caret to the end, focus steered back to the block across the morph.
function enterEdit(label, x, y) {
  const block = label.closest(".block-item");
  if (!block) return; // a morph detached the label during the settle await
  const id = block.dataset.id;
  const byKeyboard = x === undefined;
  const original = label.textContent;
  label.contentEditable = "plaintext-only";
  label.focus();
  if (byKeyboard) caretToEnd(label);
  else placeCaret(x, y);

  let done = false;
  const finish = (save) => {
    if (done) return;
    done = true;
    label.removeAttribute("contenteditable");
    label.removeEventListener("keydown", onKey);
    label.removeEventListener("blur", onBlur);
    const next = label.textContent.trim();
    if (!save || next === "" || next === original.trim()) {
      label.textContent = original; // morph will re-assert anyway
      if (byKeyboard) block.focus(); // no commit morph coming — refocus directly
      return;
    }
    // Keyboard rename triggers a morph; steer focus back to the block after it.
    if (byKeyboard) restoreFocusAfterMorph(() => document.getElementById(id));
    list.dispatchEvent(
      new CustomEvent("rename", { detail: { id, label: next } }),
    );
  };
  const onKey = (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      finish(true);
    } else if (e.key === "Escape") {
      e.preventDefault();
      finish(false);
    }
  };
  const onBlur = () => finish(true);
  label.addEventListener("keydown", onKey);
  label.addEventListener("blur", onBlur);
}

function caretToEnd(label) {
  const sel = getSelection();
  const range = document.createRange();
  range.selectNodeContents(label);
  range.collapse(false);
  sel.removeAllRanges();
  sel.addRange(range);
}

// Drop the caret at the click point.
function placeCaret(x, y) {
  const sel = getSelection();
  let range = null;
  if (document.caretPositionFromPoint) {
    const p = document.caretPositionFromPoint(x, y);
    if (p) {
      range = document.createRange();
      range.setStart(p.offsetNode, p.offset);
      range.collapse(true);
    }
  } else {
    // Older Safari lacks caretPositionFromPoint; the dynamic key keeps TS from
    // resolving the deprecated caretRangeFromPoint member.
    const key = /** @type {string} */ ("caretRangeFromPoint");
    const fromPoint = document[key];
    if (fromPoint) range = fromPoint.call(document, x, y);
  }
  if (range) {
    sel.removeAllRanges();
    sel.addRange(range);
  }
}

// ---- stretch / compress ----------------------------------------------

function startResize(e, el) {
  const orig = placementOf(el);
  const pitch = slotPitch();
  const h0 = el.getBoundingClientRect().height;
  const h = motionValue(h0);
  resize = {
    el,
    orig,
    h,
    pitch,
    // CSS margin around a block, preserved so a resized block keeps the same
    // gap a static one has.
    margin: orig.span * pitch - h0,
    bounds: boundsNow(),
    current: layoutIn(),
    hAnim: null,
    detach: styleEffect(el, { height: h }),
    sibs: sibsFor(el),
    valid: { span: orig.span, layout: layoutIn() },
    pointerId: e.pointerId,
    startY: e.clientY,
  };
  el.classList.add("resizing");
}

// Preview the grown/shrunk span: clamp to the day end, keep the last valid
// layout when growing would push a block past the bottom.
function previewResize(span) {
  const r = resize;
  span = Math.max(1, Math.min(r.bounds.end - r.orig.slot, span));
  if (span === r.valid.span) return;
  const lay = pushLayout(
    r.bounds,
    r.current,
    { id: r.orig.id, slot: r.orig.slot, span },
    { compress: true },
  );
  if (!lay) return;
  r.valid = { span, layout: lay };
  if (r.hAnim) r.hAnim.stop();
  r.hAnim = animate(r.h, span * r.pitch - r.margin, SPRING);
  springSibs(r, lay);
}

async function settleResize(e, commit) {
  if (!resize || e.pointerId !== resize.pointerId) return;
  const r = resize;
  if (!commit) {
    r.valid = { span: r.orig.span, layout: r.current };
    if (r.hAnim) r.hAnim.stop();
    r.hAnim = animate(r.h, r.orig.span * r.pitch - r.margin, SPRING);
    springSibs(r, r.current);
  }
  resize = null;
  settling = true;
  try {
    await Promise.all(
      [r.hAnim, ...sibAnims(r)].filter(Boolean),
    );
  } finally {
    if (r.hAnim) r.hAnim.stop();
    r.detach();
    teardownSibs(r);
    r.el.style.height = "";
    r.el.classList.remove("resizing");
    if (r.el.parentElement === list)
      writeLayout(r.valid.layout, r.bounds.start);
    settling = false;
  }
  dispatchLayout(r);
}

// ---- keyboard move (grab → move → drop) ------------------------------
//
// Space/Enter grabs, Up/Down move one slot (optimistic, DOM-only), Space/Enter
// drops with one `layout` event, Escape cancels. The rbd/dnd-kit convention;
// perceivability is carried by #sr-announce, not aria-grabbed.

// Mirrors the server timeLabel/blockTimeRange helpers (column.templ) so spoken
// times match the visible gutter.
const timeLabel = (s) => Math.floor(s / 2) + (s % 2 ? ":30" : ":00");
const timeRange = (slot, span) => timeLabel(slot) + " to " + timeLabel(slot + span);
const rangeOf = (id, layout) => {
  const p = layout.find((q) => q.id === id);
  return timeRange(p.slot, p.span);
};
const labelOf = (el) => {
  const l = el.querySelector(".block-label");
  return (l && l.textContent.trim()) || "Block";
};

// Setting textContent re-announces; assertive jumps any queued output.
function announce(msg) {
  if (announcer) announcer.textContent = msg;
}

function startGrab(el) {
  const start = layoutIn();
  const p = start.find((q) => q.id === el.dataset.id);
  // `start` is the immutable grab-origin layout each step recomputes from;
  // `slot` is the running target, playing the pointer's role.
  grab = { el, id: el.dataset.id, bounds: boundsNow(), start, layout: start, slot: p.slot };
  el.classList.add("dragging");
  announce(labelOf(el) + " grabbed, " + timeLabel(p.slot) + ". Use up and down arrows to move.");
}

// One arrow step: recompute the cascade from grab.start (never the running
// layout, so displacements don't accumulate) and write it straight to the DOM.
function moveGrab(key) {
  const res = keyboardLayout(grab.bounds, grab.start, { id: grab.id, mode: "move", slot: grab.slot }, key);
  if (!res) return;
  if (res.kind === "blocked") {
    announce(normalizeKey(key) === "ArrowUp" ? "Can't move earlier." : "Can't move later.");
    return;
  }
  grab.slot = res.slot;
  grab.layout = res.layout;
  writeLayout(grab.layout, grab.bounds.start);
  grab.el.scrollIntoView({ block: "nearest" });
  announce(rangeOf(grab.id, grab.layout));
}

// Drop dispatches the same `layout` event a drag does (if anything moved).
function dropGrab() {
  const g = grab;
  grab = null;
  g.el.classList.remove("dragging");
  announce("Dropped, " + rangeOf(g.id, g.layout) + ".");
  if (sameLayout(g.layout, g.start) || g.el.parentElement !== list) return;
  restoreFocusAfterMorph(() => document.getElementById(g.id));
  list.dispatchEvent(new CustomEvent("layout", { detail: { layout: g.layout } }));
}

function cancelGrab() {
  const g = grab;
  grab = null;
  g.el.classList.remove("dragging");
  writeLayout(g.start, g.bounds.start);
  announce("Move cancelled.");
}

// ---- keyboard delete -------------------------------------------------
//
// Delete/Backspace/d on a focused block: dispatch the same `delete` event the
// per-block delete button posts, and steer focus to a neighbouring block after
// the commit morph so the keyboard user isn't stranded on <body>.
function deleteBlock(el) {
  const blocks = blocksIn();
  const i = blocks.indexOf(el);
  const neighbor = blocks[i + 1] || blocks[i - 1] || null;
  announce("Deleted " + labelOf(el) + ".");
  if (neighbor)
    restoreFocusAfterMorph(() => document.getElementById(neighbor.dataset.id));
  list.dispatchEvent(new CustomEvent("delete", { detail: { id: el.dataset.id } }));
}

// After a commit morph, return focus to the acted-on target. idiomorph may
// preserve the element and keep focus, or replace it and drop focus to <body>;
// refocus only in the latter case so we never yank focus the user moved
// elsewhere. Bounded so the observer can't leak.
function restoreFocusAfterMorph(resolve) {
  const refocus = () => {
    const a = document.activeElement;
    if (a && a !== document.body) return;
    const el = resolve();
    if (el) el.focus();
  };
  const obs = new MutationObserver(refocus);
  obs.observe(list, { childList: true, subtree: true });
  setTimeout(() => obs.disconnect(), 1000);
}

// ---- keyboard resize (APG Window Splitter) ---------------------------
//
// The grip is role="separator"; Up/Down grow/shrink one slot, Home/End jump to
// min/max span, all optimistic + DOM-only. Enter or blur commits one `layout`
// event; Escape reverts. Each step recomputes from the grab-START layout, so
// shrinking undoes a grow's compression.

function handleResizeKey(e, grip) {
  if (["ArrowUp", "ArrowDown", "Home", "End"].includes(normalizeKey(e.key))) {
    e.preventDefault();
    if (!kresize) startKbResize(grip);
    stepKbResize(e.key);
  } else if (e.key === "Enter") {
    e.preventDefault();
    if (kresize) commitKbResize(true);
  } else if (e.key === "Escape") {
    e.preventDefault();
    if (kresize) cancelKbResize();
  }
}

function startKbResize(grip) {
  const el = grip.closest(".block-item");
  const start = layoutIn();
  const p = start.find((q) => q.id === el.dataset.id);
  kresize = { el, grip, id: el.dataset.id, bounds: boundsNow(), start, layout: start, span: p.span };
  el.classList.add("resizing");
}

function stepKbResize(key) {
  const r = kresize;
  const res = keyboardLayout(r.bounds, r.start, { id: r.id, mode: "resize", span: r.span }, key);
  if (!res) return;
  if (res.kind === "blocked") {
    announce(normalizeKey(key) === "ArrowUp" ? "Minimum length." : "Maximum length.");
    return;
  }
  r.span = res.span;
  r.layout = res.layout;
  writeLayout(r.layout, r.bounds.start);
  updateGripValue(r.grip, r.id, r.layout);
  r.grip.scrollIntoView({ block: "nearest" });
  announce(rangeOf(r.id, r.layout));
}

// valuenow is the span in slots, valuetext the spoken clock range.
function updateGripValue(grip, id, layout) {
  const p = layout.find((q) => q.id === id);
  grip.setAttribute("aria-valuenow", p.span);
  grip.setAttribute("aria-valuetext", timeRange(p.slot, p.span));
}

// `refocus` is true on Enter (steer focus back across the morph) and false on
// blur (the user Tabbed on — leave focus where it went).
function commitKbResize(refocus) {
  const r = kresize;
  kresize = null;
  r.el.classList.remove("resizing");
  if (sameLayout(r.layout, r.start) || r.el.parentElement !== list) return;
  announce("Resized, " + rangeOf(r.id, r.layout) + ".");
  if (refocus)
    restoreFocusAfterMorph(() => document.getElementById(r.id)?.querySelector(".grip"));
  list.dispatchEvent(new CustomEvent("layout", { detail: { layout: r.layout } }));
}

function cancelKbResize() {
  const r = kresize;
  kresize = null;
  r.el.classList.remove("resizing");
  writeLayout(r.start, r.bounds.start);
  updateGripValue(r.grip, r.id, r.start);
  announce("Resize cancelled.");
}
