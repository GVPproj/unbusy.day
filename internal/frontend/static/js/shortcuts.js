// The one page-global printable shortcut: `?` opens the shortcuts reference.
// Focus-guarded so it never fires while typing or over another dialog — the
// WCAG 2.1.4 clause every other (focus-scoped) key already satisfies.
const dialog = document.getElementById("shortcuts-modal");

if (dialog) {
  addEventListener("keydown", (e) => {
    if (e.key !== "?") return;
    const a = document.activeElement;
    if (a && (a.tagName === "INPUT" || a.tagName === "TEXTAREA" || a.isContentEditable))
      return;
    // Inert while any dialog is open (including this one).
    if (document.querySelector("dialog[open]")) return;
    e.preventDefault();
    dialog.showModal();
  });
}
