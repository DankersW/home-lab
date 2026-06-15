// Small client helpers for the Receipts UI. htmx does the heavy lifting; this
// file covers the few interactions htmx can't: the tag-chip editor, staging
// files before a create, modal/overlay dismissal, search-clear, tag filtering,
// desktop selection highlight, and the auto-dismissing toast.
(function () {
  'use strict';

  // ---- toast ----
  function armToasts(root) {
    (root || document).querySelectorAll('.toast.show').forEach(function (t) {
      setTimeout(function () { t.classList.remove('show'); }, 2200);
    });
  }

  // ---- tag chip editor ----
  function tagsIn(box) {
    return Array.prototype.map.call(box.querySelectorAll('.tagchip'), function (c) { return c.dataset.tag; });
  }
  function syncTagCsv(box) {
    box.querySelector('[data-tagcsv]').value = tagsIn(box).join(', ');
  }
  function addTag(box, raw) {
    var name = raw.trim().replace(/,+$/, '').toLowerCase();
    if (!name || tagsIn(box).indexOf(name) !== -1) return;
    var chip = document.createElement('span');
    chip.className = 'tagchip';
    chip.dataset.tag = name;
    chip.appendChild(document.createTextNode(name));
    var rm = document.createElement('button');
    rm.type = 'button';
    rm.setAttribute('data-removetag', '');
    rm.setAttribute('aria-label', 'Remove tag');
    rm.innerHTML = '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/></svg>';
    chip.appendChild(rm);
    box.insertBefore(chip, box.querySelector('[data-taginput]'));
    syncTagCsv(box);
  }

  // ---- file staging (add mode) ----
  function stageFor(cap) {
    if (!cap._stage) cap._stage = new DataTransfer();
    return cap._stage;
  }
  function fileKey(f) { return f.name + ':' + f.size + ':' + f.lastModified; }
  function renderThumbs(cap) {
    var files = cap.querySelector('[data-files]').files;
    var box = cap.querySelector('[data-thumbs]');
    box.innerHTML = '';
    Array.prototype.forEach.call(files, function (f, i) {
      var t = document.createElement('div');
      t.className = 'thumb';
      if (f.type && f.type.indexOf('image') === 0) {
        var img = document.createElement('img');
        img.src = URL.createObjectURL(f);
        t.appendChild(img);
      } else {
        t.innerHTML = '<div class="pdf"><svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linejoin="round"><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/></svg><small>PDF</small></div>';
      }
      var rm = document.createElement('button');
      rm.type = 'button';
      rm.className = 'rm';
      rm.setAttribute('data-thumb-rm', i);
      rm.setAttribute('aria-label', 'Remove');
      rm.innerHTML = '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/></svg>';
      t.appendChild(rm);
      box.appendChild(t);
    });
    var prompt = cap.querySelector('[data-capprompt]');
    if (prompt) prompt.textContent = files.length ? 'Add another file' : 'Snap a photo or add a PDF';
  }
  function addStagedFiles(cap, picked) {
    var stage = stageFor(cap);
    var have = {};
    Array.prototype.forEach.call(stage.files, function (f) { have[fileKey(f)] = true; });
    Array.prototype.forEach.call(picked, function (f) {
      if (!have[fileKey(f)]) { stage.items.add(f); have[fileKey(f)] = true; }
    });
    cap.querySelector('[data-files]').files = stage.files;
    renderThumbs(cap);
  }
  function removeStaged(cap, index) {
    var stage = stageFor(cap);
    var next = new DataTransfer();
    Array.prototype.forEach.call(stage.files, function (f, i) { if (i !== index) next.items.add(f); });
    cap._stage = next;
    cap.querySelector('[data-files]').files = next.files;
    renderThumbs(cap);
  }

  // ---- overlays ----
  function closeOverlays() {
    ['d-modal', 'm-overlay'].forEach(function (id) {
      var el = document.getElementById(id);
      if (el) el.innerHTML = '';
    });
  }

  // ---- save-button gating ----
  function gateSave(titleInput) {
    var form = titleInput.form;
    if (!form) return;
    var save = form.querySelector('[data-save]');
    if (!save && form.id) save = document.querySelector('[data-save][form="' + form.id + '"]');
    if (save) save.disabled = !titleInput.value.trim();
  }

  // ---- desktop selection highlight ----
  function highlightSelected(id) {
    var scroll = document.getElementById('d-results');
    if (!scroll) return;
    scroll.querySelectorAll('.card.active').forEach(function (c) { c.classList.remove('active'); });
    if (id) {
      var card = scroll.querySelector('.card[data-id="' + id + '"]');
      if (card) card.classList.add('active');
    }
  }

  // ---- event wiring (delegated, so dynamically swapped content works) ----
  document.addEventListener('click', function (e) {
    var rm = e.target.closest('[data-removetag]');
    if (rm) { var box = rm.closest('[data-tageditor]'); rm.closest('.tagchip').remove(); syncTagCsv(box); return; }

    var pick = e.target.closest('[data-pick]');
    if (pick) {
      var cap = pick.closest('[data-capture]');
      cap.querySelector(pick.dataset.pick === 'camera' ? '[data-camera]' : '[data-files]').click();
      return;
    }
    var pickEdit = e.target.closest('[data-pick-edit]');
    if (pickEdit) {
      var form = pickEdit.closest('[data-capture-edit]');
      form.querySelector(pickEdit.dataset.pickEdit === 'camera' ? '[data-edit-camera]' : '[data-edit-upload]').click();
      return;
    }

    var thumbRm = e.target.closest('[data-thumb-rm]');
    if (thumbRm) { removeStaged(thumbRm.closest('[data-capture]'), parseInt(thumbRm.dataset.thumbRm, 10)); return; }

    var chip = e.target.closest('[data-tagchip]');
    if (chip) {
      var shell = chip.closest('.web') || chip.closest('.app');
      var filterForm = shell && shell.querySelector('[data-filterform]');
      if (filterForm) {
        // Re-clicking the active tag clears the filter (matches the prototype).
        var next = chip.classList.contains('on') ? '' : chip.dataset.tagchip;
        filterForm.querySelector('[data-tagfilter]').value = next;
        chip.parentElement.querySelectorAll('[data-tagchip]').forEach(function (c) { c.classList.remove('on'); });
        var active = next ? chip : chip.parentElement.querySelector('[data-tagchip=""]');
        if (active) active.classList.add('on');
        var searchInput = filterForm.querySelector('[data-search]');
        window.htmx && searchInput && window.htmx.trigger(searchInput, 'filterchange');
      }
      return;
    }

    var clear = e.target.closest('[data-clearsearch]');
    if (clear) {
      var input = clear.closest('form').querySelector('[data-search]');
      input.value = '';
      clear.hidden = true;
      input.dispatchEvent(new Event('search'));
      return;
    }

    if (e.target.closest('[data-closemodal]')) { closeOverlays(); return; }
    if (e.target.matches('[data-backdrop]')) { closeOverlays(); return; }
    if (e.target.closest('[data-flashclose]')) { document.getElementById('flash').innerHTML = ''; return; }

    var card = e.target.closest('.card[data-id]');
    if (card) { window.__selId = card.dataset.id; highlightSelected(card.dataset.id); }
  });

  document.addEventListener('keydown', function (e) {
    var input = e.target.closest('[data-taginput]');
    if (!input) return;
    var box = input.closest('[data-tageditor]');
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addTag(box, input.value);
      input.value = '';
    } else if (e.key === 'Backspace' && !input.value) {
      var chips = box.querySelectorAll('.tagchip');
      if (chips.length) { chips[chips.length - 1].remove(); syncTagCsv(box); }
    }
  });

  document.addEventListener('input', function (e) {
    if (e.target.matches('[data-title]')) gateSave(e.target);
    if (e.target.matches('[data-search]')) {
      var clear = e.target.closest('form').querySelector('[data-clearsearch]');
      if (clear) clear.hidden = !e.target.value;
    }
  });

  document.addEventListener('focusout', function (e) {
    var input = e.target.closest('[data-taginput]');
    if (input) { var box = input.closest('[data-tageditor]'); addTag(box, input.value); input.value = ''; }
  });

  document.addEventListener('change', function (e) {
    if (e.target.matches('[data-files]')) {
      var cap = e.target.closest('[data-capture]');
      addStagedFiles(cap, e.target.files);
    } else if (e.target.matches('[data-camera]')) {
      var capc = e.target.closest('[data-capture]');
      addStagedFiles(capc, e.target.files);
      e.target.value = '';
    }
  });

  // After a desktop list swap (search / filter), restore the selection ring.
  document.addEventListener('htmx:afterSwap', function (e) {
    if (e.target && e.target.id === 'd-results') highlightSelected(window.__selId);
    armToasts(e.target);
  });

  document.addEventListener('DOMContentLoaded', function () {
    armToasts(document);
    var active = document.querySelector('#d-results .card.active[data-id]');
    if (active) window.__selId = active.dataset.id;
  });
})();
