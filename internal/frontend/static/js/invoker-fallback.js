// Progressive-enhancement fallbacks for two young dialog features, each
// feature-detected so it stays inert on browsers that ship the native path.

// 1. Invoker Commands API (commandfor/command on buttons) — stable Safari only
//    from 26.2. Without it the open/close buttons silently do nothing.
if (!('commandForElement' in HTMLButtonElement.prototype)) {
  document.addEventListener('click', (e) => {
    const trigger = e.target.closest('[commandfor][command]');
    if (!trigger) return;

    const dialog = document.getElementById(trigger.getAttribute('commandfor'));
    if (!(dialog instanceof HTMLDialogElement)) return;

    switch (trigger.getAttribute('command')) {
      case 'show-modal':
        dialog.showModal();
        break;
      case 'close':
        dialog.close();
        break;
    }
  });
}

// 2. closedby="any" light dismiss — not honored before stable Safari 26.2.
//    Emulate backdrop-click close: a click whose coordinates fall outside the
//    dialog's own box is a backdrop hit (modal content is the dialog's border
//    box; everything else is ::backdrop).
if (!('closedBy' in HTMLDialogElement.prototype)) {
  document.addEventListener('click', (e) => {
    const dialog = e.target;
    if (!(dialog instanceof HTMLDialogElement) || dialog.getAttribute('closedby') !== 'any') return;

    const r = dialog.getBoundingClientRect();
    const inside = e.clientX >= r.left && e.clientX <= r.right && e.clientY >= r.top && e.clientY <= r.bottom;
    if (!inside) dialog.close();
  });
}
