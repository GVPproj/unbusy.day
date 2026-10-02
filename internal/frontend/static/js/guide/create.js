// The public demo owns only ephemeral DOM state; it never calls block endpoints.
export function initDemoCreate(col, layoutIn, bounds, busy) {
  const figure = col.closest(".landing-guide-figure");
  const dialog = figure?.querySelector("[data-demo-create]");
  if (!dialog) return () => {};
  col.removeAttribute("aria-hidden");
  const form = dialog.querySelector("form");
  const addTemplate = figure.querySelector("[data-demo-add]");
  const blockTemplate = figure.querySelector("[data-demo-block]");
  const buttons = [];
  const slots = [...col.querySelectorAll(".gc-slot")];
  let selectedSlot = null;
  let nextID = 0;
  const free = (slot) => !layoutIn().some((block) => slot >= block.slot && slot < block.slot + block.span);

  for (let slot = bounds.start; slot < bounds.end; slot++) {
    const button = addTemplate.content.firstElementChild.cloneNode(true);
    button.dataset.slot = slot;
    const row = slots[slot - bounds.start];
    const time = row.querySelector(".gc-gutter").textContent;
    button.setAttribute("aria-label", `Add block at ${time}`);
    button.addEventListener("click", (event) => {
      if (busy() || !free(slot)) {
        event.preventDefault();
        return;
      }
      selectedSlot = slot;
    });
    buttons.push({ button, row, slot });
  }

  dialog.addEventListener("close", () => {
    form.reset();
    selectedSlot = null;
  });
  dialog.addEventListener("keydown", (event) => {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault();
      form.requestSubmit();
    }
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const data = new FormData(form);
    const label = String(data.get("label") || "").trim();
    if (!label || selectedSlot === null || busy() || !free(selectedSlot)) return;
    const block = blockTemplate.content.firstElementChild.cloneNode(true);
    block.dataset.id = `demo-created-${++nextID}`;
    block.dataset.type = data.get("type");
    block.dataset.slot = selectedSlot;
    block.dataset.span = 1;
    block.style.gridRow = `${selectedSlot} / span 1`;
    block.querySelector(".gc-label").textContent = label;
    col.append(block);
    refresh();
    dialog.close();
  });

  function refresh() {
    for (const { button, row, slot } of buttons) {
      if (free(slot)) row.append(button);
      else button.remove();
    }
  }
  refresh();
  return refresh;
}
