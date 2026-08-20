// Renders a Read-the-Docs style version flyout. Each service site is served
// at /<service>/<branch>/..., so the service and current branch are derived
// from the path and the version list is fetched from /<service>/versions.json.
(function () {
  "use strict";

  function parseLocation() {
    // Expect /<service>/<branch...>/<page path>. Branches can contain "/"
    // (feature/MJ-123), so the branch is whatever versions.json confirms.
    var parts = window.location.pathname.split("/").filter(Boolean);
    if (parts.length < 1) return null;
    return { service: parts[0], rest: parts.slice(1) };
  }

  function matchBranch(rest, versions) {
    // Longest match wins so "feature/x" beats a hypothetical "feature".
    var best = null;
    versions.forEach(function (v) {
      var segs = v.name.split("/");
      if (segs.length > rest.length) return;
      for (var i = 0; i < segs.length; i++) {
        if (rest[i] !== segs[i]) return;
      }
      if (!best || segs.length > best.split("/").length) best = v.name;
    });
    return best;
  }

  function build(service, current, data) {
    var wrap = document.createElement("div");
    wrap.className = "vs-flyout";

    var btn = document.createElement("button");
    btn.className = "vs-btn";
    btn.type = "button";
    btn.innerHTML =
      '<span class="vs-icon">⎇</span><span class="vs-current"></span>' +
      '<span class="vs-caret">▾</span>';
    btn.querySelector(".vs-current").textContent = current || "unknown";

    var panel = document.createElement("div");
    panel.className = "vs-panel";
    panel.hidden = true;

    var header = document.createElement("div");
    header.className = "vs-header";
    header.textContent = service;
    panel.appendChild(header);

    function section(title, items) {
      if (!items.length) return;
      var h = document.createElement("div");
      h.className = "vs-section";
      h.textContent = title;
      panel.appendChild(h);

      var list = document.createElement("div");
      list.className = "vs-list";
      items.forEach(function (v) {
        var a = document.createElement("a");
        a.href = "/" + service + "/" + v.name + "/";
        a.textContent = v.name;
        if (v.name === current) a.className = "vs-active";
        list.appendChild(a);
      });
      panel.appendChild(list);
    }

    var protectedVersions = data.versions.filter(function (v) {
      return v.protected;
    });
    var others = data.versions.filter(function (v) {
      return !v.protected;
    });

    section("Branches", protectedVersions);
    section("Recent", others);

    var footer = document.createElement("div");
    footer.className = "vs-footer";
    var all = document.createElement("a");
    all.href = "/";
    all.textContent = "All services";
    footer.appendChild(all);
    panel.appendChild(footer);

    btn.addEventListener("click", function (e) {
      e.stopPropagation();
      panel.hidden = !panel.hidden;
    });
    document.addEventListener("click", function () {
      panel.hidden = true;
    });

    wrap.appendChild(btn);
    wrap.appendChild(panel);
    document.body.appendChild(wrap);
  }

  var loc = parseLocation();
  if (!loc) return;

  fetch("/" + loc.service + "/versions.json", { cache: "no-store" })
    .then(function (r) {
      if (!r.ok) throw new Error("no versions.json");
      return r.json();
    })
    .then(function (data) {
      if (!data || !data.versions || !data.versions.length) return;
      build(loc.service, matchBranch(loc.rest, data.versions), data);
    })
    .catch(function () {
      /* version list unavailable; leave the page as-is */
    });
})();
