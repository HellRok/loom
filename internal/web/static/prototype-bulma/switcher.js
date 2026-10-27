// PROTOTYPE (Bulma look): a floating bar that cycles the ?variant= looks.
(function () {
  var variants = [["0", "today"], ["a", "Quiet defaults"], ["b", "Panels"], ["c", "Cards & messages"]];
  var m = document.cookie.match(/(?:^|; )loom-variant=([^;]*)/);
  var cur = m ? m[1] : "0";
  var i = Math.max(0, variants.findIndex(function (v) { return v[0] === cur; }));
  function go(d) {
    var v = variants[(i + d + variants.length) % variants.length][0];
    document.cookie = "loom-variant=" + v + "; path=/";
    var u = new URL(location.href); u.searchParams.delete("variant");
    location.replace(u);
  }
  var bar = document.createElement("div");
  bar.setAttribute("style", "position:fixed;bottom:12px;left:50%;transform:translateX(-50%);z-index:9999;background:#111;color:#fff;border-radius:999px;padding:4px 8px;font:13px system-ui;box-shadow:0 2px 10px rgba(0,0,0,.4);display:flex;gap:8px;align-items:center");
  bar.innerHTML = '<button data-d="-1" style="all:unset;cursor:pointer;padding:0 6px">&larr;</button><span>' + variants[i][0].toUpperCase() + " (" + variants[i][1] + ')</span><button data-d="1" style="all:unset;cursor:pointer;padding:0 6px">&rarr;</button>';
  bar.addEventListener("click", function (e) { var d = e.target.getAttribute("data-d"); if (d) go(+d); });
  document.addEventListener("keydown", function (e) {
    if (e.target.closest("input, textarea, [contenteditable]")) return;
    if (e.key === "ArrowLeft") go(-1);
    if (e.key === "ArrowRight") go(1);
  });
  document.body.appendChild(bar);
})();
