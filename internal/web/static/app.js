// Play buttons: <button class="play" data-audio="URL">. One clip at a time.
(function () {
  "use strict";
  var current = null;
  document.addEventListener("click", function (e) {
    var btn = e.target.closest("button[data-audio]");
    if (!btn) return;
    if (current) current.pause();
    current = new Audio(btn.dataset.audio);
    current.play().catch(function (err) { console.error("audio:", err); });
  });
})();
