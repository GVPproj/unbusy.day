// Throwaway landing prototype: full-page variant navigation, local state only.
const switcher = document.querySelector(".prototype-switcher");

if (switcher) {
  for (const link of switcher.querySelectorAll("a[rel]")) {
    const url = new URL(window.location.href);
    url.searchParams.set("variant", new URL(link.href).searchParams.get("variant"));
    link.href = url.href;
  }

  document.addEventListener("keydown", (event) => {
    if (
      event.defaultPrevented || event.isComposing || event.repeat ||
      event.altKey || event.ctrlKey || event.metaKey || event.shiftKey
    ) return;
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    const editing = "input, textarea, select, [contenteditable]";
    if (
      event.composedPath().some((node) => node instanceof Element && node.closest(editing)) ||
      document.activeElement?.closest(editing)
    ) return;

    event.preventDefault();
    const rel = event.key === "ArrowLeft" ? "prev" : "next";
    // Navigate rather than morph so guide/demo.js initializes the new demo.
    window.location.assign(switcher.querySelector(`a[rel="${rel}"]`).href);
  });

  const form = document.querySelector("form#prototype-login");
  const status = document.querySelector("#prototype-login-status");
  status?.setAttribute("role", "status");
  form?.addEventListener("submit", (event) => {
    event.preventDefault();
    event.stopImmediatePropagation();
    if (status) status.textContent = "Preview only — no email sent.";
    logState("login preview");
  }, { capture: true });
  for (const button of document.querySelectorAll("[data-prototype-send]")) {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      form?.requestSubmit();
    });
  }

  for (const button of document.querySelectorAll("button[data-prototype-habit]")) {
    button.addEventListener("click", (event) => {
      event.preventDefault();
      button.setAttribute("aria-pressed", String(button.getAttribute("aria-pressed") !== "true"));
      logState("habit toggle");
    });
  }
  for (const note of document.querySelectorAll("[data-prototype-note]")) {
    note.addEventListener("input", () => logState("note input"));
  }

  const observer = new MutationObserver((records) => {
    if (records.some((record) => record.attributeName.startsWith("data-"))) {
      logState("demo layout");
    }
  });
  for (const demo of document.querySelectorAll(".gc-demo")) {
    observer.observe(demo, { attributes: true, subtree: true });
  }

  logState("load");
}

function logState(reason) {
  console.log("[landing prototype]", {
    reason,
    variant: switcher.dataset.variant,
    directionName: switcher.dataset.directionName,
    demoLayout: [...document.querySelectorAll(".gc-demo")].map((demo) => ({
      ...demo.dataset,
      blocks: [...demo.querySelectorAll(".gc-block")].map((block) => ({
        ...block.dataset,
        label: block.textContent.trim(),
        slot: Number(block.dataset.slot),
        span: Number(block.dataset.span) || 1,
      })),
    })),
    notes: [...document.querySelectorAll("[data-prototype-note]")].map((note) => ({
      ...note.dataset,
      id: note.id,
      value: note.value ?? note.textContent,
    })),
    habits: [...document.querySelectorAll("button[data-prototype-habit]")].map((button) => ({
      ...button.dataset,
      id: button.id,
      label: button.getAttribute("aria-label") ?? button.textContent.trim(),
      pressed: button.getAttribute("aria-pressed") === "true",
      disabled: button.disabled,
    })),
    loginStatus: document.querySelector("#prototype-login-status")?.textContent ?? "",
  });
}
