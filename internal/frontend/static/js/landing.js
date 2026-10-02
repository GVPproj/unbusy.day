// Landing demos are local to this page and never write account data.
for (const button of document.querySelectorAll("[data-demo-habit]")) {
  button.addEventListener("click", () => {
    button.setAttribute("aria-pressed", String(button.getAttribute("aria-pressed") !== "true"));
  });
}
