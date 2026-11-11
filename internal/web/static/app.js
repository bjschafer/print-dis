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

  // Pre-tick "needs enclosure" for materials that want one; the user can untick.
  const enclosureMaterials = /^(ABS|ASA|PC|PA)([-+ 0-9].*)?$/;
  // The filament picker is a select whose options carry data-material and
  // data-hex; the swatch beside it follows the choice.
  function onMaterial(e) {
    const m = e.target.closest("[data-material]");
    if (!m) return;
    let material = m.value;
    if (m.tagName === "SELECT") {
      const opt = m.selectedOptions[0];
      material = opt ? opt.dataset.material || "" : "";
      const sw = m.form && m.form.querySelector("[data-swatch]");
      if (sw) {
        const hex = opt ? opt.dataset.hex || "" : "";
        sw.hidden = !hex;
        sw.style.background = hex;
      }
    }
    const form = m.form;
    const box = form && form.querySelector("[data-enclosure]");
    if (box && !box.dataset.touched && enclosureMaterials.test(material.trim().toUpperCase())) box.checked = true;
  }
  document.addEventListener("input", onMaterial);
  document.addEventListener("change", onMaterial);
  document.addEventListener("change", (e) => {
    if (e.target.matches("[data-enclosure]")) e.target.dataset.touched = "1";
  });

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
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", initBoard);
  else initBoard();

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
