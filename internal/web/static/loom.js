// Refetch a live pane from the address bar, so a pane keeps showing what the
// page shows however the user got there. The pane's own hx-get is a fallback.
document.body.addEventListener("htmx:configRequest", function (e) {
  if (e.detail.elt.hasAttribute("data-live")) {
    e.detail.path = location.pathname + location.search;
  }
});
