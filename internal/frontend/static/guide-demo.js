// Drag + stretch for the guide's demo column (.gc-demo): the real grid's push
// cascade (push.js) driven pointer-only and committed nowhere — neighbours
// animate via the CSS translate/height transition on .gc-block, and a gesture
// settles by rewriting grid-row in place of the transforms (FLIP).
import { pushLayout } from "./push.js";

// Must match the .gc-demo .gc-block transition duration in app.css.
const SETTLE_MS = 180;

for (const col of document.querySelectorAll(".gc-demo")) initDemo(col);

function initDemo(col) {
  const blocksIn = () => [...col.querySelectorAll(".gc-block")];
  const placementOf = (el) => ({
    id: el.dataset.id,
    slot: parseInt(el.dataset.slot, 10),
    span: parseInt(el.dataset.span, 10) || 1,
  });
  const layoutIn = () => blocksIn().map(placementOf);
  const bounds = { start: 1, end: col.querySelectorAll(".gc-slot").length + 1 };
  // Measured per gesture: the column sits in a <dialog> and has no box while closed.
  const slotPitch = () => {
    const s = col.querySelectorAll(".gc-slot");
    return s[1].getBoundingClientRect().top - s[0].getBoundingClientRect().top;
  };

  let g = null; // active gesture
  let settling = false;

  col.addEventListener("pointerdown", (e) => {
    if (g || settling || e.button !== 0) return;
    const el = e.target.closest(".gc-block");
    if (!el) return;
    e.preventDefault();
    el.setPointerCapture(e.pointerId);
    const orig = placementOf(el);
    const pitch = slotPitch();
    g = {
      el,
      orig,
      pitch,
      resize: !!e.target.closest(".gc-grip"),
      current: layoutIn(),
      valid: { slot: orig.slot, span: orig.span, layout: layoutIn() },
      // CSS margin around a block, constant across blocks — preserved so a
      // resized block keeps the gap a static one has.
      margin: orig.span * pitch - el.getBoundingClientRect().height,
      pointerId: e.pointerId,
      startY: e.clientY,
    };
    // Prime every block with its natural height as explicit px (same pixels,
    // so nothing moves): a height transition from auto can't interpolate, so
    // an unprimed block would snap on the gesture's first compression.
    for (const b of blocksIn()) {
      b.style.height = placementOf(b).span * pitch - g.margin + "px";
    }
    el.classList.add(g.resize ? "gc-resizing" : "gc-dragging");
  });

  col.addEventListener("pointermove", (e) => {
    if (!g || e.pointerId !== g.pointerId) return;
    const dy = e.clientY - g.startY;
    if (g.resize) {
      previewResize(g.orig.span + Math.round(dy / g.pitch));
    } else {
      const y = Math.max(
        (bounds.start - g.orig.slot) * g.pitch,
        Math.min((bounds.end - g.orig.span - g.orig.slot) * g.pitch, dy),
      );
      g.el.style.translate = `0 ${y}px`;
      previewDrag(g.orig.slot + Math.round(y / g.pitch));
    }
  });

  col.addEventListener("pointerup", (e) => settle(e, true));
  col.addEventListener("pointercancel", (e) => settle(e, false));

  // Keep the last valid layout when the cascade rejects, so invalid drops
  // snap to legal positions — same contract as gestures/pointer.js.
  function previewDrag(slot) {
    slot = Math.max(bounds.start, Math.min(bounds.end - g.orig.span, slot));
    if (slot === g.valid.slot) return;
    const lay = pushLayout(bounds, g.current, {
      id: g.orig.id,
      slot,
      span: g.orig.span,
    });
    if (!lay) return;
    g.valid = { ...g.valid, slot, layout: lay };
    moveSibs(g, lay);
  }

  function previewResize(span) {
    span = Math.max(1, Math.min(bounds.end - g.orig.slot, span));
    if (span === g.valid.span) return;
    const lay = pushLayout(
      bounds,
      g.current,
      { id: g.orig.id, slot: g.orig.slot, span },
      { compress: true },
    );
    if (!lay) return;
    g.valid = { ...g.valid, span, layout: lay };
    g.el.style.height = span * g.pitch - g.margin + "px";
    moveSibs(g, lay);
  }

  // Offset every other block toward its previewed placement; the CSS
  // transition animates the change. Heights are always explicit px — a
  // transition to "" (auto) snaps, so a compressed block couldn't re-expand
  // smoothly.
  function moveSibs(d, lay) {
    const by = new Map(lay.map((p) => [p.id, p]));
    for (const el of blocksIn()) {
      if (el === d.el) continue;
      const p = by.get(el.dataset.id);
      const from = placementOf(el);
      el.style.translate = `0 ${(p.slot - from.slot) * d.pitch}px`;
      el.style.height = p.span * d.pitch - d.margin + "px";
    }
  }

  function settle(e, commit) {
    if (!g || e.pointerId !== g.pointerId) return;
    const d = g;
    g = null;
    settling = true;
    if (!commit) d.valid = { slot: d.orig.slot, span: d.orig.span, layout: d.current };
    // Re-enable the transition on the grabbed block and glide it to its
    // settled slot/size; siblings are already heading to d.valid.layout.
    d.el.classList.remove("gc-dragging", "gc-resizing");
    d.el.style.translate = `0 ${(d.valid.slot - d.orig.slot) * d.pitch}px`;
    d.el.style.height = d.valid.span * d.pitch - d.margin + "px";
    moveSibs(d, d.valid.layout);
    // Then swap the transforms for real grid placement in one frame — same
    // pixels, transitions suspended so nothing glides twice.
    setTimeout(() => {
      const by = new Map(d.valid.layout.map((p) => [p.id, p]));
      const els = blocksIn();
      for (const el of els) {
        const p = by.get(el.dataset.id);
        el.style.transition = "none";
        el.dataset.slot = p.slot;
        el.dataset.span = p.span;
        el.style.gridRow = p.slot + " / span " + p.span;
        el.style.translate = "";
        el.style.height = "";
      }
      requestAnimationFrame(() =>
        requestAnimationFrame(() => {
          for (const el of els) el.style.transition = "";
          settling = false;
        }),
      );
    }, SETTLE_MS);
  }
}
