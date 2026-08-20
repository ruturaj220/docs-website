(function () {
  "use strict";

  var header = document.querySelector(".site-header");
  var input = document.getElementById("q");
  var panel = document.getElementById("search-results");
  var status = document.getElementById("search-status");
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

  /* ---- Documentation search ------------------------------------------------
     Queries /api/search, which ranks the MkDocs search index of every
     published service. Results render as a listbox under the input.          */

  var MIN_QUERY = 2;
  var DEBOUNCE_MS = 140;
  var LIMIT = 8;

  var timer = null;
  var inflight = null;
  var hits = [];
  var active = -1;

  function options() {
    return panel ? panel.querySelectorAll(".search-hit") : [];
  }

  function closePanel() {
    if (!panel) return;
    panel.hidden = true;
    panel.textContent = "";
    hits = [];
    active = -1;
    if (input) {
      input.setAttribute("aria-expanded", "false");
      input.removeAttribute("aria-activedescendant");
    }
  }

  function say(message) {
    if (status) status.textContent = message || "";
  }

  // Wrap every term occurrence in <mark>, building nodes rather than HTML so
  // documentation text can never be interpreted as markup.
  function highlight(parent, text, terms) {
    var lower = text.toLowerCase();
    var spans = [];
    terms.forEach(function (term) {
      if (!term) return;
      var from = 0;
      var at;
      while ((at = lower.indexOf(term, from)) !== -1) {
        spans.push([at, at + term.length]);
        from = at + term.length;
      }
    });
    spans.sort(function (a, b) { return a[0] - b[0]; });

    var merged = [];
    spans.forEach(function (span) {
      var last = merged[merged.length - 1];
      if (last && span[0] <= last[1]) last[1] = Math.max(last[1], span[1]);
      else merged.push([span[0], span[1]]);
    });

    var pos = 0;
    merged.forEach(function (span) {
      if (span[0] > pos) {
        parent.appendChild(document.createTextNode(text.slice(pos, span[0])));
      }
      var m = document.createElement("mark");
      m.textContent = text.slice(span[0], span[1]);
      parent.appendChild(m);
      pos = span[1];
    });
    if (pos < text.length) {
      parent.appendChild(document.createTextNode(text.slice(pos)));
    }
  }

  function setActive(i) {
    var nodes = options();
    if (!nodes.length) return;
    if (i < 0) i = nodes.length - 1;
    if (i >= nodes.length) i = 0;
    active = i;
    for (var j = 0; j < nodes.length; j++) {
      var on = j === i;
      nodes[j].classList.toggle("is-active", on);
      nodes[j].setAttribute("aria-selected", on ? "true" : "false");
    }
    nodes[i].scrollIntoView({ block: "nearest" });
    if (input) input.setAttribute("aria-activedescendant", nodes[i].id);
  }

  function message(text) {
    if (!panel) return;
    panel.textContent = "";
    var p = document.createElement("p");
    p.className = "search-note";
    p.textContent = text;
    panel.appendChild(p);
    panel.hidden = false;
    hits = [];
    active = -1;
    if (input) input.setAttribute("aria-expanded", "true");
  }

  function render(data) {
    if (!panel) return;
    hits = data.hits || [];
    var terms = data.terms || [];

    if (!hits.length) {
      message('No documentation matches "' + data.query + '".');
      say("No results.");
      return;
    }

    panel.textContent = "";
    active = -1;

    hits.forEach(function (hit, i) {
      var a = document.createElement("a");
      a.className = "search-hit";
      a.id = "search-hit-" + i;
      a.href = hit.url;
      a.setAttribute("role", "option");
      a.setAttribute("aria-selected", "false");

      var crumb = document.createElement("span");
      crumb.className = "search-crumb";
      crumb.textContent = hit.page
        ? hit.displayName + " · " + hit.page
        : hit.displayName + " · " + hit.version;
      a.appendChild(crumb);

      var title = document.createElement("span");
      title.className = "search-hit-title";
      highlight(title, hit.title || hit.url, terms);
      a.appendChild(title);

      if (hit.snippet) {
        var snip = document.createElement("span");
        snip.className = "search-snippet";
        highlight(snip, hit.snippet, terms);
        a.appendChild(snip);
      }

      a.addEventListener("mouseenter", function () { setActive(i); });
      panel.appendChild(a);
    });

    var foot = document.createElement("p");
    foot.className = "search-foot";
    foot.textContent = data.total > hits.length
      ? "Top " + hits.length + " of " + data.total + " matches — ↑ ↓ to move, Enter to open"
      : hits.length + (hits.length === 1 ? " match" : " matches") + " — ↑ ↓ to move, Enter to open";
    panel.appendChild(foot);

    panel.hidden = false;
    if (input) input.setAttribute("aria-expanded", "true");
    say(data.total + (data.total === 1 ? " result" : " results") + " for " + data.query);
  }

  function run() {
    if (!input) return;
    var q = (input.value || "").trim();
    if (q.length < MIN_QUERY) {
      closePanel();
      say("");
      return;
    }

    // Drop the previous request so a slow response can't overwrite a newer one.
    if (inflight) inflight.abort();
    var controller = "AbortController" in window ? new AbortController() : null;
    inflight = controller;

    var url = "/api/search?q=" + encodeURIComponent(q) + "&limit=" + LIMIT;
    fetch(url, controller ? { signal: controller.signal } : undefined)
      .then(function (r) {
        if (!r.ok) throw new Error("search returned " + r.status);
        return r.json();
      })
      .then(render)
      .catch(function (err) {
        if (err && err.name === "AbortError") return;
        message("Search is unavailable right now.");
        say("Search is unavailable.");
      });
  }

  if (input) {
    input.setAttribute("role", "combobox");
    input.setAttribute("aria-controls", "search-results");
    input.setAttribute("aria-expanded", "false");
    input.setAttribute("aria-autocomplete", "list");

    input.addEventListener("input", function () {
      clearTimeout(timer);
      timer = setTimeout(run, DEBOUNCE_MS);
    });

    input.addEventListener("keydown", function (e) {
      if (e.key === "ArrowDown" || e.key === "ArrowUp") {
        if (panel && panel.hidden && input.value.trim().length >= MIN_QUERY) {
          run();
          return;
        }
        if (!options().length) return;
        e.preventDefault();
        setActive(e.key === "ArrowDown" ? active + 1 : active - 1);
        return;
      }
      if (e.key === "Enter") {
        var nodes = options();
        if (!nodes.length) return;
        e.preventDefault();
        var pick = nodes[active < 0 ? 0 : active];
        if (pick) window.location.assign(pick.getAttribute("href"));
      }
    });
  }

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
    var key = e.key || "";
    if ((e.metaKey || e.ctrlKey) && key.toLowerCase() === "k") {
      e.preventDefault();
      focusSearch();
    }
    if (key === "Escape" && panel && !panel.hidden) {
      closePanel();
      say("");
    }
  });

  // A click anywhere outside the search box dismisses the results.
  document.addEventListener("click", function (e) {
    if (!panel || panel.hidden) return;
    var wrap = document.querySelector(".search-wrap");
    if (wrap && !wrap.contains(e.target)) closePanel();
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
