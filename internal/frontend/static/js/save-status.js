const documents = new WeakMap();
const priority = { saved: 0, saving: 1, failed: 2, offline: 3 };
const labels = {
  saved: "Saved",
  saving: "Saving…",
  failed: "Not confirmed — try again",
  offline: "Offline — will retry",
};

// Each writer owns its state; completing one must not hide another's pending save.
export function setSaveState(document, source, state) {
  let states = documents.get(document);
  if (!states) documents.set(document, states = new Map());
  if (state === "saved") states.delete(source);
  else states.set(source, state);
  const current = [...states.values()].reduce(
    (a, b) => priority[b] > priority[a] ? b : a, "saved",
  );
  const output = document.getElementById("companion-status");
  if (!output) return;
  if (output.dataset.state !== current) output.dataset.state = current;
  if (output.textContent !== labels[current]) output.textContent = labels[current];
}
