// Play buttons: <button class="play" data-audio="URL">. One clip at a time.
// Review cards: reveal the answer, grade, undo, with keyboard shortcuts.
(function () {
  "use strict";
  var player = new Audio();
  function play(url) {
    player.pause();
    player.src = url;
    player.play().catch(function (err) { console.warn("audio:", err); });
  }

  var shownAt = Date.now();
  function cardShown(root) {
    shownAt = Date.now();
    var a = root.querySelector("[data-card] [data-autoplay]");
    if (a) play(a.dataset.audio);
  }

  function revealed(card) { return card.classList.contains("revealed"); }
  function reveal() {
    var card = document.querySelector("[data-card]");
    if (!card || revealed(card)) return;
    card.classList.add("revealed");
    document.querySelectorAll("[data-answer], [data-grades]").forEach(function (el) { el.hidden = false; });
    var r = document.querySelector("[data-reveal]");
    if (r) r.hidden = true;
    var a = card.querySelector("[data-autoplay-reveal]");
    if (a) play(a.dataset.audio);
  }

  document.addEventListener("click", function (e) {
    var btn = e.target.closest("button[data-audio]");
    if (btn) { play(btn.dataset.audio); return; }
    if (e.target.closest("[data-reveal]")) { reveal(); return; }
    var g = e.target.closest("[data-grades] button[name=rating]");
    if (g) g.form.elements.elapsed_ms.value = Date.now() - shownAt;
  });

  document.addEventListener("keydown", function (e) {
    if (e.ctrlKey || e.metaKey || e.altKey || e.repeat) return;
    if (e.target.closest("input, textarea, select, summary, [contenteditable]")) return;
    var card = document.querySelector("[data-card]");
    var key = e.key.toLowerCase();
    if (key === " " && card) {
      e.preventDefault();
      reveal();
    } else if (/^[1-4]$/.test(key) && card && revealed(card)) {
      var b = document.querySelector('[data-grades] button[value="' + key + '"]');
      if (b) b.click();
    } else if (key === "p") {
      var p = Array.prototype.find.call(document.querySelectorAll("[data-card] [data-audio]"),
        function (el) { return !el.closest("[hidden]"); });
      if (p) play(p.dataset.audio);
    } else if (key === "u") {
      var u = document.querySelector("[data-undo]");
      if (u) u.click();
    }
  });

  document.addEventListener("htmx:afterSwap", function (e) {
    if (e.detail.target.id === "review") cardShown(e.detail.target);
  });
  document.addEventListener("DOMContentLoaded", function () { cardShown(document); });
})();
