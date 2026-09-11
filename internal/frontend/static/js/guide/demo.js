// Drag + stretch for the Guide demo. JavaScript computes the real push cascade;
// CSS interpolates targets and the final placement is swapped in as a FLIP.
import { pushLayout } from "../blocks/push.js";
import { waitForTransitions } from "../transitions.js";

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
  const dialog = col.closest("dialog");
  const slotPitch = () => {
    const slots = col.querySelectorAll(".gc-slot");
    return slots[1].getBoundingClientRect().top - slots[0].getBoundingClientRect().top;
  };

  let gesture = null;
  let settling = false;
  let generation = 0;

  col.addEventListener("pointerdown", (e) => {
    if (gesture || settling || e.button !== 0) return;
    const el = e.target.closest(".gc-block");
    if (!el) return;
    e.preventDefault();
    el.setPointerCapture(e.pointerId);
    const orig = placementOf(el);
    const pitch = slotPitch();
    gesture = {
      el,
      orig,
      pitch,
      resize: !!e.target.closest(".gc-grip"),
      current: layoutIn(),
      valid: { slot: orig.slot, span: orig.span, layout: layoutIn() },
      margin: orig.span * pitch - el.getBoundingClientRect().height,
      pointerId: e.pointerId,
      startY: e.clientY,
      generation: ++generation,
    };
    for (const block of blocksIn()) {
      block.style.height = placementOf(block).span * pitch - gesture.margin + "px";
    }
    el.classList.add(gesture.resize ? "gc-resizing" : "gc-dragging");
  });

  col.addEventListener("pointermove", (e) => {
    if (!gesture || e.pointerId !== gesture.pointerId) return;
    const dy = e.clientY - gesture.startY;
    if (gesture.resize) {
      previewResize(gesture.orig.span + Math.round(dy / gesture.pitch));
    } else {
      const y = Math.max(
        (bounds.start - gesture.orig.slot) * gesture.pitch,
        Math.min((bounds.end - gesture.orig.span - gesture.orig.slot) * gesture.pitch, dy),
      );
      gesture.el.style.translate = `0 ${y}px`;
      previewDrag(gesture.orig.slot + Math.round(y / gesture.pitch));
    }
  });

  col.addEventListener("pointerup", (e) => settle(e, true));
  col.addEventListener("pointercancel", (e) => settle(e, false));
  dialog?.addEventListener("close", abort);

  function previewDrag(slot) {
    slot = Math.max(bounds.start, Math.min(bounds.end - gesture.orig.span, slot));
    if (slot === gesture.valid.slot) return;
    const layout = pushLayout(bounds, gesture.current, {
      id: gesture.orig.id,
      slot,
      span: gesture.orig.span,
    });
    if (!layout) return;
    gesture.valid = { ...gesture.valid, slot, layout };
    moveSiblings(gesture, layout);
  }

  function previewResize(span) {
    span = Math.max(1, Math.min(bounds.end - gesture.orig.slot, span));
    if (span === gesture.valid.span) return;
    const layout = pushLayout(
      bounds,
      gesture.current,
      { id: gesture.orig.id, slot: gesture.orig.slot, span },
      { compress: true },
    );
    if (!layout) return;
    gesture.valid = { ...gesture.valid, span, layout };
    gesture.el.style.height = span * gesture.pitch - gesture.margin + "px";
    moveSiblings(gesture, layout);
  }

  function moveSiblings(active, layout) {
    const byID = new Map(layout.map((placement) => [placement.id, placement]));
    for (const el of blocksIn()) {
      if (el === active.el) continue;
      const placement = byID.get(el.dataset.id);
      const from = placementOf(el);
      if (!placement) continue;
      el.style.translate = `0 ${(placement.slot - from.slot) * active.pitch}px`;
      el.style.height = placement.span * active.pitch - active.margin + "px";
    }
  }

  async function settle(e, commit) {
    if (!gesture || e.pointerId !== gesture.pointerId) return;
    if (dialog && !dialog.open) return abort();
    const active = gesture;
    gesture = null;
    settling = true;
    if (!commit) {
      active.valid = { slot: active.orig.slot, span: active.orig.span, layout: active.current };
    }
    active.el.classList.remove("gc-dragging", "gc-resizing");
    active.el.style.translate = `0 ${(active.valid.slot - active.orig.slot) * active.pitch}px`;
    active.el.style.height = active.valid.span * active.pitch - active.margin + "px";
    moveSiblings(active, active.valid.layout);
    await waitForTransitions(blocksIn(), ["translate", "height"]);
    if (active.generation !== generation) return;
    // Hiding cancels transitions before the dialog's queued close event runs.
    if (dialog && !dialog.open) return abort();
    finish(active.valid.layout);
    settling = false;
  }

  function abort() {
    generation++;
    const active = gesture;
    gesture = null;
    if (active) finish(active.current);
    else clearTransient();
    settling = false;
  }

  function finish(layout) {
    const byID = new Map(layout.map((placement) => [placement.id, placement]));
    const elements = blocksIn().filter((el) => byID.has(el.dataset.id));
    for (const el of elements) {
      const placement = byID.get(el.dataset.id);
      el.dataset.slot = placement.slot;
      el.dataset.span = placement.span;
      el.style.gridRow = placement.slot + " / span " + placement.span;
    }
    clearTransient(elements);
  }

  function clearTransient(elements = blocksIn()) {
    for (const el of elements) {
      el.classList.add("gc-flip");
      el.style.translate = "";
      el.style.height = "";
      el.classList.remove("gc-dragging", "gc-resizing");
    }
    void col.offsetHeight;
    for (const el of elements) el.classList.remove("gc-flip");
  }
}
