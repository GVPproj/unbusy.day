(() => {
  const companion = document.querySelector(".companion");
  if (!companion) return;

  const tabs = [...companion.querySelectorAll('[role="tab"]')];
  const select = (selected) => {
    let shown;
    for (const tab of tabs) {
      const active = tab === selected;
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
      const panel = document.getElementById(tab.getAttribute("aria-controls"));
      if (panel.hidden === active) {
        // The editor snapshots before hiding and restores after becoming measurable.
        if (!active) panel.dispatchEvent(new Event("companion-hide"));
        panel.hidden = !active;
        if (active) shown = panel;
      }
    }
    shown?.dispatchEvent(new Event("companion-show"));
  };

  for (const [index, tab] of tabs.entries()) {
    tab.addEventListener("click", () => select(tab));
    tab.addEventListener("keydown", (event) => {
      let next;
      switch (event.key) {
        case "ArrowLeft": next = (index + tabs.length - 1) % tabs.length; break;
        case "ArrowRight": next = (index + 1) % tabs.length; break;
        case "Home": next = 0; break;
        case "End": next = tabs.length - 1; break;
        default: return;
      }
      event.preventDefault();
      select(tabs[next]);
      tabs[next].focus();
    });
  }
})();
