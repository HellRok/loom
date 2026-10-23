// Refetch a live pane from the address bar, so a pane keeps showing what the
// page shows however the user got there. The pane's own hx-get is a fallback.
document.body.addEventListener("htmx:configRequest", function (e) {
  if (e.detail.elt.hasAttribute("data-live")) {
    e.detail.path = location.pathname + location.search;
  }
});

// A swap of #main rebuilds the panes. Carry the navigation's scroll across it,
// so following a view or label link doesn't jump the sidebar back to the top.
var navScroll = 0;
document.body.addEventListener("htmx:beforeSwap", function (e) {
  if (e.detail.target.id === "main") {
    var nav = document.getElementById("nav");
    navScroll = nav ? nav.scrollTop : 0;
  }
});
document.body.addEventListener("htmx:afterSwap", function (e) {
  var nav = e.detail.target.id === "main" && document.getElementById("nav");
  if (nav) nav.scrollTop = navScroll;
});
