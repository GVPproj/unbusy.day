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

const list = document.getElementById("block-list");
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
let settling = false;

// Past this many px the gesture is a drag, not a tap; below it a pointerup on
// a label opens the inline editor (TAP_SLOP keeps a shaky click still an edit).
const TAP_SLOP = 4;

list.addEventListener("pointerdown", (e) => {
  if (drag || resize || settling || e.button !== 0) return;
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
    drag.x.set(e.clientX - drag.startX);
    drag.y.set(e.clientY - drag.startY);
    if (
      Math.abs(e.clientX - drag.startX) > TAP_SLOP ||
      Math.abs(e.clientY - drag.startY) > TAP_SLOP
    )
      drag.moved = true;
    previewDrag(drag.orig.slot + Math.round(drag.y.get() / drag.pitch));
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
  const x = motionValue(0);
  const y = motionValue(0);
  drag = {
    el,
    orig,
    x,
    y,
    bounds: boundsNow(),
    current: layoutIn(),
    pitch: slotPitch(),
    detach: styleEffect(el, { x, y }),
    sibs: sibsFor(el),
    // last layout the cascade accepted — what an invalid pointer snaps to
    valid: { slot: orig.slot, layout: layoutIn() },
    pointerId: e.pointerId,
    startX: e.clientX,
    startY: e.clientY,
    // a tap (no movement) on the label opens the inline editor at pointerup
    moved: false,
    downTarget: e.target,
  };
  el.classList.add("dragging");
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

// Turn a label into a plaintext editor focused at (x, y), then commit on blur
// or Enter / revert on Escape. A changed, non-empty label dispatches `rename`
// on #block-list (Datastar posts it); the server's morph re-asserts the truth.
function enterEdit(label, x, y) {
  const block = label.closest(".block-item");
  if (!block) return; // a morph detached the label during the settle await
  const id = block.dataset.id;
  const original = label.textContent;
  label.contentEditable = "plaintext-only";
  label.focus();
  placeCaret(x, y);

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
      return;
    }
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
