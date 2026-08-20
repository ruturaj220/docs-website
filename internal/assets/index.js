(function () {
  "use strict";

  var header = document.querySelector(".site-header");
  var input = document.getElementById("q");
  var cards = document.querySelectorAll("[data-card]");
  var empty = document.getElementById("no-results");
  var kbd = document.getElementById("search-kbd");
  var searchOpen = document.getElementById("search-open");

  if (kbd && !/Mac|iPhone|iPad/.test(navigator.platform)) {
    kbd.textContent = "Ctrl K";
  }

  function onScroll() {
    if (header) header.classList.toggle("is-scrolled", window.scrollY > 8);
  }
  onScroll();
  window.addEventListener("scroll", onScroll, { passive: true });

  function filter() {
    if (!cards.length) return;
    var q = ((input && input.value) || "").trim().toLowerCase();
    var shown = 0;
    cards.forEach(function (card) {
      var hay = card.getAttribute("data-q") || "";
      var hit = !q || hay.indexOf(q) !== -1;
      card.hidden = !hit;
      if (hit) shown += 1;
    });
    if (empty) empty.hidden = shown !== 0;
  }

  if (input) input.addEventListener("input", filter);

  function focusSearch() {
    if (!input) return;
    window.scrollTo({ top: 0, behavior: "smooth" });
    input.focus();
    input.select();
  }

  if (searchOpen) {
    searchOpen.addEventListener("click", focusSearch);
  }

  document.addEventListener("keydown", function (e) {
    if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
      e.preventDefault();
      focusSearch();
    }
    if (e.key === "Escape" && input && document.activeElement === input) {
      input.value = "";
      filter();
      input.blur();
    }
  });

  // Google Drive video modal
  var modal = document.getElementById("video-modal");
  var frame = document.getElementById("video-frame");
  document.querySelectorAll(".video-thumb[data-drive-id]").forEach(function (btn) {
    btn.addEventListener("click", function () {
      var id = btn.getAttribute("data-drive-id");
      if (!id || !modal || !frame) return;
      frame.src = "https://drive.google.com/file/d/" + id + "/preview";
      if (typeof modal.showModal === "function") modal.showModal();
      else window.open("https://drive.google.com/file/d/" + id + "/view", "_blank");
    });
  });
  if (modal) {
    modal.addEventListener("close", function () {
      if (frame) frame.src = "";
    });
  }

  // Video carousel controls
  var row = document.getElementById("video-row");
  var prev = document.getElementById("video-prev");
  var next = document.getElementById("video-next");
  var dots = document.querySelectorAll("#video-dots i");

  function cardStep() {
    var card = row && row.querySelector(".video-card");
    if (!card) return 320;
    var styles = window.getComputedStyle(row);
    var gap = parseFloat(styles.columnGap || styles.gap) || 0;
    return card.getBoundingClientRect().width + gap;
  }

  function syncDots() {
    if (!row || !dots.length) return;
    var step = cardStep();
    var idx = Math.round(row.scrollLeft / step);
    idx = Math.max(0, Math.min(dots.length - 1, idx));
    dots.forEach(function (dot, i) {
      dot.classList.toggle("on", i === idx);
    });
  }

  if (row) {
    row.addEventListener("scroll", syncDots, { passive: true });
    if (prev) {
      prev.addEventListener("click", function () {
        row.scrollBy({ left: -cardStep(), behavior: "smooth" });
      });
    }
    if (next) {
      next.addEventListener("click", function () {
        row.scrollBy({ left: cardStep(), behavior: "smooth" });
      });
    }
  }
})();
