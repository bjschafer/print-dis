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
  // colors in one print" once two colors are picked; the user can untick.
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
      let n = 0;
      form.querySelectorAll("[data-slot]").forEach((s) => {
        const c = s.querySelector("input[type=radio]:checked");
        if (c && c.value) n++;
      });
      if (n > 1) multi.checked = true;
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
      const r = t.closest(".dot").querySelector("[data-exact-pick]");
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
