// print-dis: small progressive enhancements. Everything works without it.
(function () {
  "use strict";
  const csrf = () => document.body.dataset.csrf || "";

  // Filter bars submit on change.
  document.addEventListener("change", (e) => {
    const form = e.target.closest("form[data-autosubmit]");
    if (form) form.requestSubmit();
  });

  // Confirmation for destructive buttons.
  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-confirm]");
    if (b && !window.confirm(b.dataset.confirm)) e.preventDefault();
  });

  // Reset htmx forms after a successful post (e.g. the comment box).
  document.addEventListener("htmx:afterRequest", (e) => {
    const f = e.target.closest("form[data-reset-on-success]");
    if (f && e.detail.successful) f.reset();
  });

  // Material and color picker. Without script it still works: the radios
  // post, and "add another color" re-renders the form with a blank slot.
  const enclosureMaterials = /^(ABS|ASA|PC|PA)([-+ 0-9].*)?$/;
  function chosenMaterial(form) {
    const r = form.querySelector("input[name=material]:checked");
    let m = r ? r.value : "";
    const other = form.querySelector("[data-material-other]");
    if (m === "other" || (m === "" && other && other.value.trim())) m = other ? other.value : "";
    return m.trim().toUpperCase();
  }
  // Shelf tiles show only the chosen material and what the search matches;
  // a picked tile always shows.
  function filterShelf(form) {
    const m = chosenMaterial(form);
    form.querySelectorAll("[data-slot]").forEach((slot) => {
      const search = slot.querySelector("[data-shelf-search]");
      const q = search ? search.value.trim().toLowerCase() : "";
      slot.querySelectorAll("[data-shelf-group]").forEach((g) => {
        let any = false;
        g.querySelectorAll("[data-tile]").forEach((t) => {
          const ok = t.querySelector("input").checked ||
            ((!m || t.dataset.material.startsWith(m)) && (!q || t.dataset.search.includes(q)));
          t.hidden = !ok;
          if (ok) any = true;
        });
        g.hidden = !any;
      });
    });
  }
  // Pre-tick "needs enclosure" for materials that want one, and "several
  // colors in one print" once two colors are picked, unless each color is
  // filed on its own; the user can untick.
  function pretick(form) {
    const encl = form.querySelector("[data-enclosure]");
    if (encl && !encl.dataset.touched) {
      const ms = [chosenMaterial(form)];
      form.querySelectorAll("[data-slot] input[type=radio]:checked[data-fil-material]")
        .forEach((i) => ms.push(i.dataset.filMaterial.toUpperCase()));
      if (ms.some((m) => enclosureMaterials.test(m))) encl.checked = true;
    }
    const multi = form.querySelector("[data-multi]");
    if (multi && !multi.dataset.touched) {
      const split = form.querySelector("[data-split]");
      let n = 0;
      form.querySelectorAll("[data-slot]").forEach((s) => {
        const c = s.querySelector("input[type=radio]:checked");
        if (c && c.value) n++;
      });
      if (split && split.checked) multi.checked = false;
      else if (n > 1) multi.checked = true;
    }
  }
  function layoutSlots(form) {
    const box = form.querySelector("[data-slots]");
    if (!box) return;
    const slots = box.querySelectorAll("[data-slot]");
    slots.forEach((s, i) => {
      s.querySelector("[data-slot-title]").textContent = slots.length > 1 ? `Color ${i + 1}` : "Color";
      s.querySelector("[data-slot-remove]").hidden = slots.length < 2;
    });
    const add = form.querySelector("[data-add-color]");
    if (add) add.hidden = slots.length >= Number(box.dataset.max || 8);
  }
  function initPicker(form) {
    if (form.dataset.pickerReady) return;
    form.dataset.pickerReady = "1";
    form.querySelectorAll("[data-shelf-search]").forEach((i) => { i.hidden = false; });
    layoutSlots(form);
    filterShelf(form);
  }
  function addSlot(form) {
    const box = form.querySelector("[data-slots]");
    const tpl = form.querySelector("template[data-slot-template]");
    if (!box || !tpl) return;
    const used = new Set(Array.from(box.querySelectorAll("[data-slot] input[name^=color_hex_]")).map((i) => i.name.slice(10)));
    // The lowest free key: the server reads keys below 64.
    let key = 0;
    while (used.has(String(key))) key++;
    const div = document.createElement("div");
    div.innerHTML = tpl.innerHTML.replaceAll("__N__", String(key));
    const slot = div.firstElementChild;
    box.appendChild(slot);
    slot.querySelectorAll("[data-shelf-search]").forEach((i) => { i.hidden = false; });
    layoutSlots(form);
    filterShelf(form);
    const first = slot.querySelector("input[type=radio]");
    if (first) first.focus();
  }
  document.addEventListener("click", (e) => {
    const add = e.target.closest("[data-add-color]");
    if (add && add.form) {
      e.preventDefault();
      addSlot(add.form);
      return;
    }
    const rm = e.target.closest("[data-slot-remove]");
    if (rm) {
      const form = rm.form;
      rm.closest("[data-slot]").remove();
      if (form) { layoutSlots(form); filterShelf(form); }
    }
  });
  function onPicker(e) {
    const form = e.target.form;
    if (!form || !form.querySelector("[data-slots], [data-material-pick]")) return;
    const t = e.target;
    if (t.matches("[data-material-other]") && t.value.trim()) {
      const r = form.querySelector("[data-material-other-pick]");
      if (r) r.checked = true;
    }
    if (t.matches("[data-exact]")) {
      const r = t.closest(".color-opt").querySelector("[data-exact-pick]");
      if (r) r.checked = true;
    }
    if (t.matches("[data-enclosure], [data-multi]") && e.type === "change") {
      t.dataset.touched = "1";
      return;
    }
    if (t.matches("input[name=material], [data-material-other], [data-shelf-search]")) filterShelf(form);
    pretick(form);
  }
  document.addEventListener("input", onPicker);
  document.addEventListener("change", onPicker);
  function initPickers() { document.querySelectorAll("form").forEach((f) => { if (f.querySelector("[data-slots]")) initPicker(f); }); }

  // Board drag and drop, for admins.
  function initBoard() {
    const board = document.querySelector(".board[data-can-move]");
    if (!board || !window.Sortable) return;
    board.querySelectorAll(".column-list").forEach((list) => {
      window.Sortable.create(list, {
        group: "board",
        animation: 150,
        delayOnTouchOnly: true,
        delay: 180,
        ghostClass: "ghost",
        filter: ".card-deliver",
        preventOnFilter: false,
        onEnd: (evt) => {
          if (evt.from === evt.to && evt.oldIndex === evt.newIndex) return;
          const id = evt.item.dataset.id;
          const order = Array.from(evt.to.querySelectorAll(".card")).map((c) => c.dataset.id);
          const body = new URLSearchParams({ status: evt.to.dataset.status, order: order.join(",") });
          fetch(`/requests/${id}/move`, {
            method: "POST",
            headers: { "X-CSRF-Token": csrf(), "Content-Type": "application/x-www-form-urlencoded" },
            body,
            credentials: "same-origin",
          }).then((r) => {
            if (!r.ok) throw new Error(r.status);
            updateCounts(board);
          }).catch(() => {
            alert("Could not move that card. The board will reload.");
            location.reload();
          });
        },
      });
    });
  }
  function updateCounts(board) {
    board.querySelectorAll(".column").forEach((col) => {
      const c = col.querySelector(".count");
      if (c) c.textContent = col.querySelectorAll(".card").length;
    });
  }
  // A Ready card's "Delivered" button takes the card off the board in
  // place. Without script the form posts and the board reloads.
  document.addEventListener("submit", (e) => {
    const form = e.target;
    if (e.defaultPrevented || !form.matches("form[data-deliver]")) return;
    e.preventDefault();
    const card = form.closest(".card");
    const board = form.closest(".board");
    form.querySelectorAll("button").forEach((b) => { b.disabled = true; });
    fetch(form.action, {
      method: "POST",
      headers: { "HX-Request": "true", "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams(new FormData(form)),
      credentials: "same-origin",
    }).then((r) => {
      if (!r.ok) throw new Error(r.status);
      card.remove();
      if (board) updateCounts(board);
    }).catch(() => {
      alert("Could not mark that delivered. The board will reload.");
      location.reload();
    });
  });
  // Board columns collapse to a thin lane; which ones is kept in a cookie
  // so the board renders that way from the start.
  function saveCollapsed(board) {
    const ids = Array.from(board.querySelectorAll(".column.collapsed")).map((c) => c.dataset.column);
    document.cookie = `board_collapsed=${ids.join(".")}; path=/; max-age=31536000; samesite=lax`;
  }
  function toggleColumn(col) {
    const board = col.closest(".board");
    const collapsed = col.classList.toggle("collapsed");
    const b = col.querySelector("[data-col-toggle]");
    const name = col.querySelector("h2").textContent;
    b.setAttribute("aria-expanded", String(!collapsed));
    b.setAttribute("aria-label", `${collapsed ? "Show" : "Hide"} ${name}`);
    b.title = collapsed ? "Show this column" : "Hide this column";
    if (board) saveCollapsed(board);
  }
  document.addEventListener("click", (e) => {
    const b = e.target.closest("[data-col-toggle]");
    const col = b ? b.closest(".column") : e.target.closest(".column.collapsed");
    if (!col || (!b && e.target.closest(".card"))) return;
    e.preventDefault();
    toggleColumn(col);
  });
  function init() {
    initBoard();
    initPickers();
    document.querySelectorAll("[data-col-toggle]").forEach((b) => { b.hidden = false; });
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();

  // A form with files chosen is sent by XHR so the page shows how far the
  // upload has got and then that the server is saving it, rather than the
  // browser's spinner alone. Asked with HX-Request, the server answers success
  // with HX-Redirect; a page (a form with errors) replaces this one, and a
  // plain-text refusal shows in place. Without script the form posts normally.
  function uploadStatus(form, submitter) {
    let el = form.querySelector("[data-upload-status]");
    if (!el) {
      el = document.createElement("div");
      el.className = "progress upload-progress";
      el.dataset.uploadStatus = "";
      el.setAttribute("role", "status");
      el.innerHTML = '<span class="bar"><span></span></span><span class="progress-text"></span>';
      if (submitter) (submitter.closest(".actions") || submitter).after(el);
      else form.append(el);
    }
    el.classList.remove("saving", "failed");
    return el;
  }
  function showPage(html) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    document.title = doc.title;
    document.body.replaceWith(document.adoptNode(doc.body));
    if (window.htmx) window.htmx.process(document.body);
    init();
    window.scrollTo(0, 0);
  }
  document.addEventListener("submit", (e) => {
    const form = e.target;
    // A named button ("Add another color") re-renders the form; let it post.
    if (e.defaultPrevented || !form.matches("form[data-upload]") || (e.submitter && e.submitter.name)) return;
    const count = Array.from(form.querySelectorAll("input[type=file]")).reduce((n, i) => n + i.files.length, 0);
    if (!count) return;
    e.preventDefault();
    const data = new FormData(form);
    const buttons = form.querySelectorAll("button");
    const status = uploadStatus(form, e.submitter);
    const bar = status.querySelector(".bar > span");
    const text = status.querySelector(".progress-text");
    const fail = (msg) => {
      status.classList.remove("saving");
      status.classList.add("failed");
      bar.style.width = "0";
      text.textContent = msg;
      buttons.forEach((b) => { b.disabled = false; });
    };
    buttons.forEach((b) => { b.disabled = true; });
    bar.style.width = "0";
    text.textContent = "Uploading…";
    const xhr = new XMLHttpRequest();
    xhr.open("POST", form.action);
    xhr.setRequestHeader("HX-Request", "true");
    xhr.upload.addEventListener("progress", (ev) => {
      if (!ev.lengthComputable) return;
      const pct = Math.floor((100 * ev.loaded) / ev.total);
      bar.style.width = `${pct}%`;
      text.textContent = `Uploading… ${pct}%`;
    });
    xhr.upload.addEventListener("load", () => {
      status.classList.add("saving");
      bar.style.width = "100%";
      text.textContent = `Sent. Saving the ${count > 1 ? "files" : "file"}; this can take a little while…`;
    });
    xhr.addEventListener("load", () => {
      const to = xhr.getResponseHeader("HX-Redirect");
      if (to) { location.assign(to); return; }
      if ((xhr.getResponseHeader("Content-Type") || "").startsWith("text/html")) { showPage(xhr.responseText); return; }
      fail(xhr.responseText.trim() || "The upload failed. Try again in a moment.");
    });
    xhr.addEventListener("error", () => fail("The upload did not go through. Check your connection and try again."));
    xhr.send(data);
  });

  // Camera buttons open the printer's camera in a dialog. Without
  // JavaScript (or <dialog>) they are plain links to the camera page.
  document.addEventListener("click", (e) => {
    const a = e.target.closest("a[data-camera]");
    const dlg = document.getElementById("camera-dialog");
    if (!a || !dlg || typeof dlg.showModal !== "function" || e.metaKey || e.ctrlKey || e.shiftKey) return;
    e.preventDefault();
    e.stopPropagation(); // inside a <summary>, do not toggle it
    const body = dlg.querySelector(".camera-dialog-body");
    body.innerHTML = '<p class="camera-msg">Connecting to the camera…</p>';
    dlg.showModal();
    fetch(a.href, { headers: { "HX-Request": "true" }, credentials: "same-origin" })
      .then((r) => { if (!r.ok) throw new Error(r.status); return r.text(); })
      .then((html) => { if (dlg.open) body.innerHTML = html; })
      .catch(() => { body.innerHTML = '<p class="camera-msg">The camera is not available right now.</p>'; });
  });
  // Closing drops the image, which ends the stream.
  document.addEventListener("close", (e) => {
    if (e.target.id === "camera-dialog") e.target.querySelector(".camera-dialog-body").innerHTML = "";
  }, true);
  // Clicking the backdrop closes it.
  document.addEventListener("click", (e) => {
    if (e.target.id === "camera-dialog") e.target.close();
  });
  // A camera that fails shows a sentence instead of a broken image.
  document.addEventListener("error", (e) => {
    const img = e.target;
    if (!(img instanceof HTMLImageElement) || !img.classList.contains("camera-img")) return;
    img.hidden = true;
    const msg = img.parentElement.querySelector(".camera-msg");
    if (msg) msg.hidden = false;
  }, true);
})();
