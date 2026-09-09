(() => {
  const create = document.getElementById("habit-create");
  if (!create) return;

  const start = document.getElementById("habit-start");
  let draft = false;
  let defaulting = false;
  const seedDate = () => {
    const today = new Date();
    start.value = [today.getFullYear(), String(today.getMonth() + 1).padStart(2, "0"),
      String(today.getDate()).padStart(2, "0")].join("-");
  };
  // This classic script seeds the input before Datastar initializes its binding.
  seedDate();
  create.addEventListener("input", () => { if (!defaulting) draft = true; });
  create.addEventListener("change", () => { if (!defaulting) draft = true; });
  // Re-selecting the same date need not emit a change; date interaction cedes ownership.
  start.addEventListener("pointerdown", () => { draft = true; });
  start.addEventListener("keydown", (event) => {
    if (!["Tab", "Escape"].includes(event.key)) draft = true;
  });
  document.getElementById("habit-create-dialog").addEventListener("beforetoggle", (event) => {
    if (event.newState !== "open" || draft) return;
    defaulting = true;
    seedDate();
    start.dispatchEvent(new Event("input", { bubbles: true }));
    defaulting = false;
  });

  for (const form of document.querySelectorAll(".habit-dialog-form")) {
    const feedback = () => form.querySelector("output");
    form.addEventListener("input", () => { feedback().textContent = ""; });
    form.addEventListener("invalid", (event) => {
      feedback().textContent = event.target.validationMessage;
    }, true);
  }

  // Only save-status sharing is deferred; local defaults and draft listeners bind now.
  const saveStatus = import("../save-status.js");
  const forms = new Map();
  const failureTypes = new Set(["error", "retries-failed"]);
  document.addEventListener("datastar-fetch", async (event) => {
    const form = event.detail?.el?.closest?.("#habit-create, #habit-edit form, #habit-delete form");
    if (!form) return;
    const { setSaveState } = await saveStatus;
    const source = form.id || form.closest("dialog").id;
    const type = event.detail.type;
    if (type === "started") {
      forms.set(source, { failed: false });
      setSaveState(document, source, "saving");
      return;
    }
    const attempt = forms.get(source);
    if (!attempt) return;
    if (type === "retrying") {
      attempt.failed = false;
      setSaveState(document, source, "saving");
    } else if (failureTypes.has(type)) {
      attempt.failed = true;
      setSaveState(document, source, "failed");
    } else if (type === "finished") {
      forms.delete(source);
      if (!attempt.failed) setSaveState(document, source, "saved");
    }
  });
})();
