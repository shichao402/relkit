/* Release catalog SPA. Zero framework, zero build: this file is embedded in
 * the relkit binary and served as a static shell. All data is fetched at
 * runtime from the same origin:
 *   catalog.json                     -> product list
 *   channel/<product>/<channel>.json -> channel directory (history list)
 *   release/<product>/<channel>/<version>.json -> one version's download card
 * Humans only: protocol clients never read these endpoints. */
(function () {
  "use strict";

  var view = document.getElementById("view");
  var heading = document.getElementById("heading");
  var sub = document.getElementById("sub");
  var crumbs = document.getElementById("crumbs");
  var catalogCache = null;

  function el(tag, attrs, children) {
    var node = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (key) {
        if (key === "text") node.textContent = attrs[key];
        else if (key === "href" || key === "class" || key.indexOf("data-") === 0) {
          if (attrs[key] !== null && attrs[key] !== undefined) node.setAttribute(key, attrs[key]);
        }
        else node[key] = attrs[key];
      });
    }
    (children || []).forEach(function (child) { node.appendChild(child); });
    return node;
  }

  function fmtSize(n) {
    if (!n && n !== 0) return "";
    var units = ["B", "KiB", "MiB", "GiB"];
    var value = n, i = 0;
    while (value >= 1024 && i < units.length - 1) { value /= 1024; i++; }
    return (value >= 10 || i === 0 ? Math.round(value) : value.toFixed(1)) + " " + units[i];
  }

  function fmtDate(iso) {
    if (!iso) return "";
    var d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toISOString().slice(0, 10);
  }

  function fetchJSON(url) {
    return fetch(url, { headers: { Accept: "application/json" } }).then(function (resp) {
      if (!resp.ok) throw new Error(url + " -> HTTP " + resp.status);
      return resp.json();
    });
  }

  function loadCatalog() {
    if (catalogCache) return Promise.resolve(catalogCache);
    return fetchJSON("catalog.json").then(function (doc) {
      catalogCache = doc;
      return doc;
    });
  }

  function setCrumbs(items) {
    crumbs.textContent = "";
    items.forEach(function (item, i) {
      if (i) crumbs.appendChild(document.createTextNode(" / "));
      if (item.href) crumbs.appendChild(el("a", { href: item.href, text: item.label }));
      else crumbs.appendChild(document.createTextNode(item.label));
    });
  }

  function channelHref(product, channel) {
    return "#/p/" + encodeURIComponent(product) + "/" + encodeURIComponent(channel);
  }

  function historyHref(product, channel) {
    return channelHref(product, channel) + "/history";
  }

  function defaultChannel(product) {
    var names = (product.channels || []).map(function (ch) { return ch.name; });
    if (names.indexOf("stable") >= 0) return "stable";
    return names[0] || "stable";
  }

  /* ---- views ---- */

  function renderHome(doc) {
    heading.textContent = "Releases";
    sub.textContent = "";
    setCrumbs([]);
    view.textContent = "";
    if (!doc.products || !doc.products.length) {
      view.appendChild(el("p", { class: "sub", text: "No published products yet." }));
      return;
    }
    doc.products.forEach(function (product) {
      var head = el("div", { class: "cardhead" }, [
        el("h2", null, [el("a", { href: "#/p/" + encodeURIComponent(product.id), text: product.title || product.id })]),
      ]);
      if (product.homepage) head.appendChild(el("a", { href: product.homepage, text: "project", class: "sub" }));
      var card = el("div", { class: "card" }, [head]);
      if (product.description) card.appendChild(el("p", { class: "sub", text: product.description }));
      var chips = (product.channels || []).map(function (ch) {
        return ch.name + " " + ch.version;
      }).join(" · ");
      if (chips) card.appendChild(el("p", { class: "sub", text: chips }));
      view.appendChild(card);
    });
  }

  function artifactRow(artifact) {
    var platform = Object.keys(artifact.selectors || {}).sort().map(function (key) {
      return artifact.selectors[key];
    }).join(" · ") || artifact.kind || "";
    var row = el("div", { class: "download" }, [
      el("div", null, [
        el("div", { class: "mono", text: artifact.filename }),
        el("div", { class: "platform", text: platform + (artifact.size ? " · " + fmtSize(artifact.size) : "") }),
      ]),
      el("a", { class: "btn", href: (artifact.urls && artifact.urls[0]) || "#", text: "Download" }),
    ]);
    return row;
  }

  function renderChannelEntry(entry, product, channel, expanded) {
    var head = el("div", { class: "cardhead" }, [
      el("h2", { class: entry.yanked ? "yanked" : null, text: entry.version }),
      el("span", { class: "sub", text: "code " + entry.code }),
      el("span", { class: "sub", text: fmtDate(entry.releasedAt) }),
    ]);
    if (entry.yanked) head.appendChild(el("span", { class: "badge", text: "yanked" }));
    if (entry.notesUrl) head.appendChild(el("a", { href: entry.notesUrl, text: "notes" }));
    var card = el("div", { class: "card" }, [head]);

    if (expanded) {
      var holder = el("div", { class: "busy", text: "Loading downloads…" });
      card.appendChild(holder);
      fetchJSON(entry.releaseDoc).then(function (release) {
        holder.textContent = "";
        (release.artifacts || []).forEach(function (artifact) { holder.appendChild(artifactRow(artifact)); });
        if (!holder.firstChild) holder.textContent = "No user-facing artifacts for this release.";
      }).catch(function (err) {
        holder.textContent = "Could not load release details: " + err.message;
      });
    } else {
      card.appendChild(el("a", { class: "btn", href: historyHref(product, channel) + "/" + encodeURIComponent(entry.version), text: "Downloads" }));
    }
    return card;
  }

  function renderChannel(product, channelName) {
    var productTitle = product.title || product.id;
    heading.textContent = productTitle;
    setCrumbs([
      { href: "#/", label: "Releases" },
      { label: productTitle },
      { label: channelName },
    ]);

    var channelUrl = "channel/" + encodeURIComponent(product.id) + "/" + encodeURIComponent(channelName) + ".json";
    view.textContent = "";
    view.appendChild(el("p", { class: "busy", text: "Loading…" }));

    fetchJSON(channelUrl).then(function (channel) {
      view.textContent = "";
      sub.textContent = (product.description || "") + " · " + (channel.versions ? channel.versions.length : 0) + " version(s) retained";

      var chips = el("div", { class: "chips" });
      (product.channels || []).forEach(function (ch) {
        var active = ch.name === channelName;
        chips.appendChild(el("a", {
          href: active ? null : channelHref(product.id, ch.name),
          class: "chip",
          text: ch.name + " " + ch.version,
          "aria-current": active ? "true" : null,
        }));
      });
      view.appendChild(chips);

      var latest = channel.latest;
      if (latest) view.appendChild(renderChannelEntry(latest, product.id, channelName, true));
      view.appendChild(el("a", { class: "chip", href: historyHref(product.id, channelName), text: "History →" }));
    }).catch(function (err) {
      view.textContent = "";
      view.appendChild(el("p", { class: "sub", text: "Could not load channel " + channelName + ": " + err.message }));
    });
  }

  function renderHistory(product, channelName) {
    var productTitle = product.title || product.id;
    heading.textContent = productTitle + " history";
    setCrumbs([
      { href: "#/", label: "Releases" },
      { href: "#/p/" + encodeURIComponent(product.id), label: productTitle },
      { href: channelHref(product.id, channelName), label: channelName },
      { label: "history" },
    ]);

    var channelUrl = "channel/" + encodeURIComponent(product.id) + "/" + encodeURIComponent(channelName) + ".json";
    view.textContent = "";
    view.appendChild(el("p", { class: "busy", text: "Loading…" }));

    fetchJSON(channelUrl).then(function (channel) {
      view.textContent = "";
      sub.textContent = (product.description || "") + " · " + channel.versions.length + " version(s) retained";

      var table = el("table", null, [el("tr", null, [
        el("th", { text: "Version" }), el("th", { text: "Code" }),
        el("th", { text: "Released" }), el("th", { text: "Notes" }), el("th", { class: "num", text: "Downloads" }),
      ])]);
      channel.versions.forEach(function (entry) {
        var link = historyHref(product.id, channelName) + "/" + encodeURIComponent(entry.version);
        var row = el("tr", { class: entry.yanked ? "yanked" : null }, [
          el("td", null, [el("a", { href: link, text: entry.version })]),
          el("td", { class: "mono", text: String(entry.code) }),
          el("td", { text: fmtDate(entry.releasedAt) }),
          el("td", null, entry.notesUrl ? [el("a", { href: entry.notesUrl, text: "notes" })] : []),
          el("td", { class: "num", text: "" }),
        ]);
        table.appendChild(row);
      });
      var card = el("div", { class: "card" }, [table]);
      view.appendChild(card);
    }).catch(function (err) {
      view.textContent = "";
      view.appendChild(el("p", { class: "sub", text: "Could not load history: " + err.message }));
    });
  }

  function renderVersion(product, channelName, version) {
    var productTitle = product.title || product.id;
    heading.textContent = productTitle + " " + version;
    setCrumbs([
      { href: "#/", label: "Releases" },
      { href: "#/p/" + encodeURIComponent(product.id), label: productTitle },
      { href: channelHref(product.id, channelName), label: channelName },
      { href: historyHref(product.id, channelName), label: "history" },
      { label: version },
    ]);
    sub.textContent = "";

    var releaseUrl = "release/" + encodeURIComponent(product.id) + "/" + encodeURIComponent(channelName) + "/" + encodeURIComponent(version) + ".json";
    view.textContent = "";
    view.appendChild(el("p", { class: "busy", text: "Loading…" }));

    fetchJSON(releaseUrl).then(function (release) {
      view.textContent = "";
      var head = el("div", { class: "cardhead" }, [
        el("h2", { text: release.version }),
        el("span", { class: "sub", text: "code " + release.code }),
        el("span", { class: "sub", text: fmtDate(release.releasedAt) }),
      ]);
      if (release.notesUrl) head.appendChild(el("a", { href: release.notesUrl, text: "notes" }));
      var card = el("div", { class: "card" }, [head]);
      var list = el("div", null);
      (release.artifacts || []).forEach(function (artifact) { list.appendChild(artifactRow(artifact)); });
      if (!list.firstChild) list.appendChild(el("p", { class: "sub", text: "No user-facing artifacts." }));
      card.appendChild(list);
      view.appendChild(card);
    }).catch(function (err) {
      view.textContent = "";
      view.appendChild(el("p", { class: "sub", text: "Could not load release " + version + ": " + err.message }));
    });
  }

  /* ---- router ---- */

  function findProduct(doc, id) {
    return (doc.products || []).filter(function (p) { return p.id === id; })[0] || null;
  }

  function route() {
    var hash = location.hash || "#/";
    var parts = hash.replace(/^#\//, "").split("/").map(decodeURIComponent);

    if (!parts[0]) {
      loadCatalog().then(renderHome).catch(fail("the catalog"));
      return;
    }
    if (parts[0] !== "p" || !parts[1]) return notFound(hash);

    loadCatalog().then(function (doc) {
      var product = findProduct(doc, parts[1]);
      if (!product) return notFound(hash);
      var channels = product.channels || [];
      if (!parts[2]) {
        if (!channels.length) {
          view.textContent = "";
          view.appendChild(el("p", { class: "sub", text: product.title + " has no published channels." }));
          return;
        }
        location.replace(channelHref(product.id, defaultChannel(product)));
        return;
      }
      var known = channels.some(function (ch) { return ch.name === parts[2]; });
      if (!known) return notFound(hash);
      if (!parts[3]) return renderChannel(product, parts[2]);
      if (parts[3] === "history") {
        if (parts[4]) return renderVersion(product, parts[2], parts[4]);
        return renderHistory(product, parts[2]);
      }
      notFound(hash);
    }).catch(fail("the catalog"));
  }

  function notFound(hash) {
    heading.textContent = "Not found";
    sub.textContent = "";
    setCrumbs([{ href: "#/", label: "Releases" }]);
    view.textContent = "";
    view.appendChild(el("p", { class: "sub", text: "No page for " + hash + "." }));
  }

  function fail(what) {
    return function (err) {
      heading.textContent = "Releases";
      view.textContent = "";
      view.appendChild(el("p", { class: "sub", text: "Could not load " + what + ": " + err.message }));
    };
  }

  window.addEventListener("hashchange", route);
  route();
})();
