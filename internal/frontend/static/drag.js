// Motion-powered drag + stretch for the #block-list day grid: transforms the
// real <li> (no ghost), previews the client-computed push cascade live
// (push.js, ADR 0005), commits as a FLIP, then dispatches one `layout` event
// carrying the full resulting layout. Listeners are delegated to #block-list
// (morph-stable) so wiring survives patches.
import {
  animate,
  motionValue,
  styleEffect,
} from "https://cdn.jsdelivr.net/npm/motion@12.40.0/+esm";
import { pushLayout } from "./push.js";
import { keyboardLayout } from "./keys.js";

const list = document.getElementById("block-list");
// Visually-hidden assertive live region in BlocksPage, OUTSIDE #block-list so it
// survives every morph; the keyboard path speaks action feedback through it.
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

// Row pitch from consecutive slot rows — every slot is a real fixed-height
// grid row now, so geometry is measured, never derived from a probe block.
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

// Past this many px the gesture is a drag, not a tap; below it a pointerup on
// a label opens the inline editor (TAP_SLOP keeps a shaky click still an edit).
const TAP_SLOP = 4;

list.addEventListener("pointerdown", (e) => {
  if (drag || resize || settling || e.button !== 0) return;
  // A pointer gesture supersedes an in-progress keyboard grab/resize.
  if (grab) cancelGrab();
  if (kresize) cancelKbResize();
  // A label already in edit mode owns its own pointer (caret/selection) —
  // don't capture it into a drag gesture.
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

// Keyboard move, delegated on #block-list (morph-stable) beside the pointer
// listeners. Only the block body (the <li>) is the move tab stop — the grip
// (resize), delete button, and rename editor each own their own keys. Arrows
// are inert until a block is grabbed, so they never fight focus navigation.
list.addEventListener("keydown", (e) => {
  if (drag || resize || settling) return;
  // Grip (resize separator) owns its own keys; e.target is the grip when focused.
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
    } else if (e.key === "F2") {
      // F2 enters the inline editor without a pointer (the established
      // rename-in-place key); Enter is taken by grab/drop, so it can't be reused.
      e.preventDefault();
      const label = el.querySelector(".block-label");
      if (label) enterEdit(label);
    }
    return;
  }
  if (e.key === "ArrowUp" || e.key === "ArrowDown") {
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

// Focus leaving a grabbed block (e.g. Tab) abandons the move and snaps it back;
// focus leaving a resizing grip COMMITS (the splitter convention — blur saves),
// so Tab moves on with the change kept. drop/commit/cancel already nulled state.
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
    // h0 is the block's natural height (margins make it ~3px under span*pitch);
    // springing height from here keeps unchanged siblings from twitching.
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

// Spring every other block to its slot delta — and, when compression changed
// its span, its height — under layout `lay` (the live preview). A sibling back
// at its original span springs height to its natural h0, undoing compression.
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

// Stop every sibling spring, detach its styleEffect, and clear the transform
// and height Motion left behind — shared by the drag and resize settle paths.
function teardownSibs(g) {
  g.sibs.forEach((s, c) => {
    if (s.yAnim) s.yAnim.stop();
    if (s.hAnim) s.hAnim.stop();
    s.detach();
    c.style.transform = "";
    c.style.height = "";
  });
}

// Every sibling's in-flight springs (position and height), for awaiting on settle.
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
    // Transform clamp: the held block can't translate past the day's first/last
    // legal slot. A translate beyond the grid grows the scroll container's
    // overflow without bound (and the auto-scroll loop would never hit a limit).
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
    // edge auto-scroll state: current per-frame velocity and the live rAF id
    scrollV: 0,
    raf: 0,
    // a tap (no movement) on the label opens the inline editor at pointerup
    moved: false,
    downTarget: e.target,
  };
  el.classList.add("dragging");
}

// Set the held block's transform from the last pointer position and preview the
// cascade under it. Split out of pointermove so the auto-scroll loop can re-apply
// it each frame (the pointer is stationary while the container scrolls under it).
function applyDrag() {
  const d = drag;
  const dx = d.lastX - d.startX;
  const dy = d.lastY - d.startY;
  if (Math.abs(dx) > TAP_SLOP || Math.abs(dy) > TAP_SLOP) d.moved = true;
  // Clamp into the day so the block can't be dragged past the first/last slot
  // (which would stretch the scroll container without bound).
  const y = Math.max(d.minY, Math.min(d.maxY, dy));
  d.x.set(dx);
  d.y.set(y);
  previewDrag(d.orig.slot + Math.round(y / d.pitch));
}

// Edge auto-scroll: when the pointer nears the top/bottom of the scroll container
// (#block-list is the only scroll region — ADR scroll commit), scroll it so the
// held block can be dragged past the visible viewport. Speed ramps with depth
// into the EDGE band; the rAF loop self-sustains while the pointer holds at an
// edge (no further pointermove fires), and stops at the scroll limits.
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

// One auto-scroll frame: scroll, then keep the transform math consistent by
// folding the actual scrolled distance into startY (the pointer's clientY is
// fixed, so the held block keeps tracking it and the target slot stays right).
function scrollTick() {
  const d = drag;
  if (!d) return; // settle nulled the gesture mid-flight
  d.raf = 0;
  if (!d.scrollV) return; // pointer left the edge band
  const before = list.scrollTop;
  list.scrollTop += d.scrollV;
  const moved = list.scrollTop - before; // 0 at a scroll limit
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
  // A tap (committed, no movement) on the label opens the inline editor once
  // the no-op settle finishes — caret placed where the pointer went down.
  const editLabel =
    commit && !d.moved ? d.downTarget.closest(".block-label") : null;
  if (!commit) d.valid = { slot: d.orig.slot, layout: d.current };
  if (d.raf) cancelAnimationFrame(d.raf);
  springSibs(d, d.valid.layout);
  drag = null;
  settling = true;
  try {
    // land the held block AND let in-flight sibling springs finish
    await Promise.all([
      animate(d.x, 0, SPRING),
      animate(d.y, (d.valid.slot - d.orig.slot) * d.pitch, SPRING),
      ...sibAnims(d),
    ]);
  } finally {
    // Teardown and the layout write below share one synchronous frame:
    // same pixels, new grid placement (FLIP).
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

// Turn a label into a plaintext editor, then commit on blur or Enter / revert on
// Escape. A pointer tap passes the click point (x, y) and leaves focus to the
// platform — tap-to-rename is unchanged. Keyboard entry (F2) passes no point: the
// caret goes to the end and focus is steered back to the block, since with no
// pointer there's nothing to land focus on after the commit morph. A changed,
// non-empty label dispatches `rename` on #block-list (the server's morph wins).
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

// Collapse the selection to the end of the label — the keyboard rename entry
// point, which has no pointer location to honour.
function caretToEnd(label) {
  const sel = getSelection();
  const range = document.createRange();
  range.selectNodeContents(label);
  range.collapse(false); // false = end
  sel.removeAllRanges();
  sel.addRange(range);
}

// Drop the caret at the click point, falling back to the standards/WebKit APIs.
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
    // WebKit fallback (older Safari lacks caretPositionFromPoint); the dynamic
    // key keeps TS from resolving the deprecated caretRangeFromPoint member.
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
    // vertical margin the CSS leaves around a block; preserved so a resized
    // block keeps the same gap a static one has rather than filling the slot.
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
// Space/Enter grabs the focused block, Up/Down move it one slot (optimistic,
// DOM-only via writeLayout — no spring, no per-key server round-trip), Space/
// Enter drops it with one `layout` event (the same the drag path dispatches),
// Escape cancels and snaps back. The rbd/dnd-kit convention; perceivability is
// carried by #sr-announce and the reused .dragging lift, not aria-grabbed.

// Mirror of the server timeLabel/blockTimeRange helpers (column.templ) so spoken
// times match the visible gutter: a slot index in 30-min steps from 00:00.
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
  // `start` is the immutable grab-origin layout every step's cascade is recomputed
  // from (so displacements can't accumulate into gaps a drag never makes); `slot`
  // is the mover's running target, playing the role the pointer does for a drag.
  grab = { el, id: el.dataset.id, bounds: boundsNow(), start, layout: start, slot: p.slot };
  el.classList.add("dragging");
  announce(labelOf(el) + " grabbed, " + timeLabel(p.slot) + ". Use up and down arrows to move.");
}

// One arrow step: the reducer recomputes the cascade from grab.start (never the
// running layout, so displacements don't accumulate) against the running target
// slot; we write the result straight to the DOM (moves are discrete, so no spring)
// and announce the new range — or announce a blocked edge and change nothing.
function moveGrab(key) {
  const res = keyboardLayout(grab.bounds, grab.start, { id: grab.id, mode: "move", slot: grab.slot }, key);
  if (!res) return;
  if (res.kind === "blocked") {
    // Blocked = no legal slot further this way (the day edge, or an immovable
    // cascade short of it); a direction-truthful phrasing covers both cases.
    announce(key === "ArrowUp" ? "Can't move earlier." : "Can't move later.");
    return;
  }
  grab.slot = res.slot;
  grab.layout = res.layout;
  writeLayout(grab.layout, grab.bounds.start);
  // Scroll the moved block into view so a keyboard move past the viewport edge
  // stays visible (#block-list is the contained scroll region).
  grab.el.scrollIntoView({ block: "nearest" });
  announce(rangeOf(grab.id, grab.layout));
}

// Commit: clear grab state, then (only if anything actually moved) dispatch the
// same `layout` event a drag drop does and steer focus back once the morph lands.
function dropGrab() {
  const g = grab;
  grab = null;
  g.el.classList.remove("dragging");
  announce("Dropped, " + rangeOf(g.id, g.layout) + ".");
  if (sameLayout(g.layout, g.start) || g.el.parentElement !== list) return;
  restoreFocusAfterMorph(() => document.getElementById(g.id));
  list.dispatchEvent(new CustomEvent("layout", { detail: { layout: g.layout } }));
}

// Escape (or focus leaving the block): snap the DOM back to the grab's start and
// dispatch nothing — the move is abandoned.
function cancelGrab() {
  const g = grab;
  grab = null;
  g.el.classList.remove("dragging");
  writeLayout(g.start, g.bounds.start);
  announce("Move cancelled.");
}

// After a commit morph, return focus to the acted-on target (resolved fresh each
// mutation, since the morph may replace it). idiomorph may preserve the element
// (stable id) and keep focus, or replace it and drop focus to <body>; refocus
// only in the latter case so we never yank focus the user has since moved
// elsewhere (e.g. a blur-commit where they Tabbed on). Bounded so it can't leak.
function restoreFocusAfterMorph(resolve) {
  const refocus = () => {
    const a = document.activeElement;
    if (a && a !== document.body) return; // focus already where the user wants it
    const el = resolve();
    if (el) el.focus();
  };
  const obs = new MutationObserver(refocus);
  obs.observe(list, { childList: true, subtree: true });
  setTimeout(() => obs.disconnect(), 1000);
}

// ---- keyboard resize (APG Window Splitter) ---------------------------
//
// The grip is role="separator"; Up/Down grow/shrink by one slot, Home/End jump
// to the min (one slot) / max legal span, all optimistic + DOM-only via the
// reducer + writeLayout (compress cascade, no spring). Enter or blur commits one
// `layout` event (the drag path's); Escape reverts. Each step recomputes from
// the grab-START layout against the running span cursor, so shrinking undoes a
// grow's compression. aria-valuenow/valuetext track the live span as a clock range.

// First arrow/Home/End on a focused grip starts the gesture; Enter/Escape only
// act once one is underway. Keys are preventDefaulted so they don't scroll.
function handleResizeKey(e, grip) {
  if (["ArrowUp", "ArrowDown", "Home", "End"].includes(e.key)) {
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
  // `start` is the immutable resize-origin layout; `span` is the running target,
  // threaded back into the reducer each step (the resize analog of grab.slot).
  kresize = { el, grip, id: el.dataset.id, bounds: boundsNow(), start, layout: start, span: p.span };
  el.classList.add("resizing");
}

function stepKbResize(key) {
  const r = kresize;
  const res = keyboardLayout(r.bounds, r.start, { id: r.id, mode: "resize", span: r.span }, key);
  if (!res) return;
  if (res.kind === "blocked") {
    announce(key === "ArrowUp" ? "Minimum length." : "Maximum length.");
    return;
  }
  r.span = res.span;
  r.layout = res.layout;
  writeLayout(r.layout, r.bounds.start);
  updateGripValue(r.grip, r.id, r.layout);
  // Keep the grip (the growing bottom edge) in view as the block stretches past
  // the contained scroll region's viewport.
  r.grip.scrollIntoView({ block: "nearest" });
  announce(rangeOf(r.id, r.layout));
}

// Keep the separator's reported value in step with the live span (Window
// Splitter): valuenow is the span in slots, valuetext the spoken clock range.
function updateGripValue(grip, id, layout) {
  const p = layout.find((q) => q.id === id);
  grip.setAttribute("aria-valuenow", p.span);
  grip.setAttribute("aria-valuetext", timeRange(p.slot, p.span));
}

// Commit: dispatch the same `layout` event a pointer resize does (only if the
// span changed). `refocus` is true on Enter (focus stays, steer it back across
// the morph) and false on blur (the user Tabbed on — leave focus where it went).
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

// Escape: snap the DOM (and the grip's reported value) back to the start span
// and dispatch nothing — the resize is abandoned.
function cancelKbResize() {
  const r = kresize;
  kresize = null;
  r.el.classList.remove("resizing");
  writeLayout(r.start, r.bounds.start);
  updateGripValue(r.grip, r.id, r.start);
  announce("Resize cancelled.");
}
