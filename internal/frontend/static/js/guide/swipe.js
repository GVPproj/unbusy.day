// Swipe stepping for the guide's panes. On touch, app.css lays .guide-body out
// as a scroll-snap carousel; this keeps the scroll offset and $_guidestep in
// sync both ways — a settled swipe dispatches `guidestep`, and a step change
// from anywhere else is seen through the .showing class Datastar toggles.
// On pointer:fine the body never scrolls, so stride() is 0 and both no-op.
const behavior = matchMedia("(prefers-reduced-motion: reduce)").matches
  ? "instant"
  : "smooth";

for (const body of document.querySelectorAll(".guide-body")) initSwipe(body);

function initSwipe(body) {
  const panes = [...body.querySelectorAll(".guide-pane")];
  const dialog = body.closest("dialog");
  if (panes.length < 2 || !dialog) return;

  // Distance between snap positions. 0 when the panes are stacked (pointer:fine)
  // or the dialog is closed — both cases where there is nothing to sync.
  const stride = () => panes[1].offsetLeft - panes[0].offsetLeft;
  const showing = () => panes.findIndex((p) => p.classList.contains("showing"));

  let step = Math.max(1, showing() + 1);
  let idle = 0;

  // Read the offset only once scrolling settles. A moving scroller passes
  // through every pane's neighbourhood, so reading mid-flight would publish a
  // stale step — and the scroll a Back/Next click starts would then fight its
  // own destination back to where it came from.
  body.addEventListener("scroll", () => {
    clearTimeout(idle);
    idle = setTimeout(settled, 120);
  });

  function settled() {
    const s = stride();
    if (!s) return;
    const i = Math.min(panes.length, Math.max(1, Math.round(body.scrollLeft / s) + 1));
    if (i === step) return;
    step = i;
    body.dispatchEvent(new CustomEvent("guidestep", { detail: { step: i } }));
  }

  const classes = new MutationObserver(() => scrollToShowing(behavior));
  for (const p of panes) {
    classes.observe(p, { attributes: true, attributeFilter: ["class"] });
  }

  // A closed dialog can't scroll, so the on-close reset to step 1 lands here
  // instead: jump to the showing pane the moment it opens.
  new MutationObserver(() => dialog.open && scrollToShowing("instant")).observe(
    dialog,
    { attributes: true, attributeFilter: ["open"] },
  );

  function scrollToShowing(how) {
    const i = showing();
    if (i < 0) return;
    step = i + 1;
    const left = i * stride();
    // Already there — this is the swipe's own round trip back through Datastar.
    if (Math.abs(body.scrollLeft - left) < 2) return;
    body.scrollTo({ left, behavior: how });
  }
}
