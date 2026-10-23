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

// Following a link swaps only the Issue pane, so the list's selected row goes
// stale. Once the address has changed, refetch the list for it. The list is
// the request's source so the morph extension, set on the panes, applies.
var issueLinkSwap = false;
document.body.addEventListener("htmx:beforeSwap", function (e) {
  issueLinkSwap = e.detail.target.id === "issue-pane" && e.detail.requestConfig.elt.hasAttribute("hx-push-url");
});
document.body.addEventListener("htmx:pushedIntoHistory", function () {
  var list = document.getElementById("list-pane");
  if (issueLinkSwap && list) {
    htmx.ajax("GET", location.pathname + location.search, { source: list, target: list, swap: "morph:outerHTML" });
  }
  issueLinkSwap = false;
});
