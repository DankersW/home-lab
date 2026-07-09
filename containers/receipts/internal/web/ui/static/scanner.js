// Document scanner: capture a photo, auto-detect the receipt edges, let the user
// nudge the corners, perspective-correct + clean the surface, and hand back a
// single-page PDF that flows through the existing upload path. OpenCV.js and
// jscanify are vendored under /static/vendor and loaded lazily on first use, so
// the ~9 MB engine never touches a normal page load.
(function () {
  'use strict';

  var VENDOR = '/static/vendor/';
  var MAX_WORK_DIM = 2000; // cap the working image so detection + warp stay bounded on phones
  var MAX_OUT_W = 2000;
  var MAX_OUT_H = 2600;
  var JPEG_QUALITY = 0.85;
  var FILTERS = [
    { id: 'color', label: 'Color' },
    { id: 'gray', label: 'Grayscale' },
    { id: 'bw', label: 'B&W' },
  ];

  // ---- lazy engine loader ----
  var enginePromise = null;
  var jscan = null;

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      var s = document.createElement('script');
      s.src = src;
      s.async = true;
      s.onload = function () { resolve(); };
      s.onerror = function () { reject(new Error('failed to load ' + src)); };
      document.head.appendChild(s);
    });
  }

  function ensureEngine() {
    if (enginePromise) return enginePromise;
    enginePromise = loadScript(VENDOR + 'opencv.js')
      .then(function () {
        return new Promise(function (resolve) {
          if (window.cv && window.cv.Mat) return resolve();
          window.cv = window.cv || {};
          window.cv.onRuntimeInitialized = function () { resolve(); };
        });
      })
      .then(function () { return loadScript(VENDOR + 'jscanify.js'); })
      .then(function () { jscan = new window.jscanify(); })
      .catch(function (err) {
        enginePromise = null; // allow a retry on the next attempt
        throw err;
      });
    return enginePromise;
  }

  // ---- small DOM helper ----
  function el(tag, attrs, kids) {
    var node = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) {
        if (k === 'class') node.className = attrs[k];
        else if (k === 'html') node.innerHTML = attrs[k];
        else node.setAttribute(k, attrs[k]);
      });
    }
    (kids || []).forEach(function (c) {
      node.appendChild(typeof c === 'string' ? document.createTextNode(c) : c);
    });
    return node;
  }

  function dist(a, b) { return Math.hypot(a.x - b.x, a.y - b.y); }
  function clamp(v, lo, hi) { return v < lo ? lo : v > hi ? hi : v; }

  function fitDims(w, h) {
    var maxW = Math.min(window.innerWidth - 32, 900);
    var maxH = window.innerHeight * 0.66;
    var scale = Math.min(maxW / w, maxH / h, 1);
    return { w: Math.round(w * scale), h: Math.round(h * scale), scale: scale };
  }

  var CLOSE_SVG = '<svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="6" y1="6" x2="18" y2="18"/><line x1="18" y1="6" x2="6" y2="18"/></svg>';
  var SPIN = '<span class="scanner-spin"></span>';

  // ---- session state (one active scan at a time) ----
  var session = null;

  // ---- entry: a photo has been captured for scanning ----
  function beginScan(file, deliver) {
    if (session) closeScanner();
    session = { deliver: deliver, cleanup: [] };
    buildOverlay();
    setStatus(SPIN + '<p>Preparing scanner…<br><small>first run downloads the engine</small></p>');

    var url = URL.createObjectURL(file);
    session.cleanup.push(function () { URL.revokeObjectURL(url); });

    var mine = session;
    var img = new Image();
    Promise.all([ensureEngine(), loadImage(img, url)])
      .then(function () { if (session === mine) startAdjust(img); })
      .catch(function (err) {
        if (session === mine) setStatus('<p class="scanner-err">Could not start the scanner.<br><small>' + escapeHtml(err.message) + '</small></p>');
      });
  }

  function loadImage(img, url) {
    return new Promise(function (resolve, reject) {
      img.onload = function () { resolve(img); };
      img.onerror = function () { reject(new Error('could not read the image')); };
      img.src = url;
    });
  }

  function escapeHtml(s) {
    return String(s).replace(/[&<>]/g, function (c) { return { '&': '&amp;', '<': '&lt;', '>': '&gt;' }[c]; });
  }

  // ---- overlay chrome ----
  function buildOverlay() {
    var root = el('div', { class: 'scanner', role: 'dialog', 'aria-modal': 'true', 'aria-label': 'Scan document' });
    var closeBtn = el('button', { class: 'scanner-close', type: 'button', 'aria-label': 'Cancel scan', html: CLOSE_SVG });
    closeBtn.addEventListener('click', closeScanner);
    var head = el('div', { class: 'scanner-head' }, [
      el('span', { class: 'scanner-title' }, ['Scan document']),
      closeBtn,
    ]);
    var body = el('div', { class: 'scanner-body' });
    var foot = el('div', { class: 'scanner-foot' });
    root.appendChild(head);
    root.appendChild(body);
    root.appendChild(foot);
    document.body.appendChild(root);
    document.body.classList.add('scanner-open');
    session.root = root;
    session.body = body;
    session.foot = foot;
    session.onKey = function (e) { if (e.key === 'Escape') closeScanner(); };
    document.addEventListener('keydown', session.onKey);
    // Re-fit the active step when the viewport changes (phone rotation, etc.).
    session.onResize = function () { if (session && session.relayout) session.relayout(); };
    window.addEventListener('resize', session.onResize);
    window.addEventListener('orientationchange', session.onResize);
  }

  function setStatus(html) {
    if (!session) return;
    session.body.innerHTML = '<div class="scanner-status">' + html + '</div>';
    session.foot.innerHTML = '';
  }

  function closeScanner() {
    if (!session) return;
    document.removeEventListener('keydown', session.onKey);
    window.removeEventListener('resize', session.onResize);
    window.removeEventListener('orientationchange', session.onResize);
    session.cleanup.forEach(function (fn) { try { fn(); } catch (e) { /* best effort */ } });
    if (session.root && session.root.parentNode) session.root.parentNode.removeChild(session.root);
    document.body.classList.remove('scanner-open');
    session = null;
  }

  // ---- step 1: adjust corners over the captured photo ----
  // startAdjust downscales the captured photo into a bounded working canvas and
  // runs auto-detection, then renders the adjust step.
  function startAdjust(img) {
    var scale = Math.min(MAX_WORK_DIM / img.naturalWidth, MAX_WORK_DIM / img.naturalHeight, 1);
    var work = el('canvas');
    work.width = Math.round(img.naturalWidth * scale);
    work.height = Math.round(img.naturalHeight * scale);
    work.getContext('2d').drawImage(img, 0, 0, work.width, work.height);
    session.work = work;
    session.corners = detectCorners(work);
    showAdjust();
  }

  // showAdjust renders the adjust step from session.work + session.corners. It is
  // separate from startAdjust so "Back" can return here without re-detecting
  // (preserving the user's corner tweaks) or rebuilding the working canvas.
  function showAdjust() {
    var work = session.work;
    var fit = fitDims(work.width, work.height);
    session.displayScale = fit.scale;

    var view = el('canvas', { class: 'scanner-canvas' });
    view.width = work.width;
    view.height = work.height;
    view.style.width = fit.w + 'px';
    view.style.height = fit.h + 'px';
    view.getContext('2d').drawImage(work, 0, 0);

    var stage = el('div', { class: 'scanner-stage' });
    stage.style.width = fit.w + 'px';
    stage.style.height = fit.h + 'px';
    stage.appendChild(view);

    var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
    svg.setAttribute('class', 'scanner-quad');
    svg.setAttribute('viewBox', '0 0 ' + fit.w + ' ' + fit.h);
    var poly = document.createElementNS('http://www.w3.org/2000/svg', 'polygon');
    svg.appendChild(poly);
    stage.appendChild(svg);
    session.svg = svg;
    session.poly = poly;
    session.stage = stage;
    session.view = view;

    session.handles = {};
    ['tl', 'tr', 'br', 'bl'].forEach(function (key) {
      var h = el('div', { class: 'scanner-handle', 'data-corner': key });
      attachDrag(h, key, stage);
      stage.appendChild(h);
      session.handles[key] = h;
    });

    session.body.innerHTML = '';
    session.body.appendChild(el('p', { class: 'scanner-hint' }, ['Drag the corners to frame the receipt']));
    session.body.appendChild(stage);
    positionHandles();
    session.relayout = relayoutAdjust;

    session.foot.innerHTML = '';
    var cancel = el('button', { class: 'btn ghost-d', type: 'button' }, ['Cancel']);
    cancel.addEventListener('click', closeScanner);
    var next = el('button', { class: 'btn btn-primary', type: 'button' }, ['Continue']);
    next.addEventListener('click', startPreview);
    session.foot.appendChild(cancel);
    session.foot.appendChild(next);
  }

  function relayoutAdjust() {
    var fit = fitDims(session.work.width, session.work.height);
    session.displayScale = fit.scale;
    session.view.style.width = fit.w + 'px';
    session.view.style.height = fit.h + 'px';
    session.stage.style.width = fit.w + 'px';
    session.stage.style.height = fit.h + 'px';
    session.svg.setAttribute('viewBox', '0 0 ' + fit.w + ' ' + fit.h);
    positionHandles();
  }

  // detectCorners returns corners in WORK-canvas coordinates, falling back to an
  // inset rectangle when auto-detection is unavailable or incomplete.
  function detectCorners(work) {
    var fallback = insetQuad(work.width, work.height);
    var src = null, contour = null;
    try {
      src = window.cv.imread(work);
      contour = jscan.findPaperContour(src);
      if (!contour) return fallback;
      var c = jscan.getCornerPoints(contour);
      if (!c.topLeftCorner || !c.topRightCorner || !c.bottomLeftCorner || !c.bottomRightCorner) {
        return fallback;
      }
      return {
        tl: { x: c.topLeftCorner.x, y: c.topLeftCorner.y },
        tr: { x: c.topRightCorner.x, y: c.topRightCorner.y },
        br: { x: c.bottomRightCorner.x, y: c.bottomRightCorner.y },
        bl: { x: c.bottomLeftCorner.x, y: c.bottomLeftCorner.y },
      };
    } catch (e) {
      return fallback;
    } finally {
      if (contour) contour.delete();
      if (src) src.delete();
    }
  }

  function insetQuad(w, h) {
    var m = 0.06;
    return {
      tl: { x: w * m, y: h * m },
      tr: { x: w * (1 - m), y: h * m },
      br: { x: w * (1 - m), y: h * (1 - m) },
      bl: { x: w * m, y: h * (1 - m) },
    };
  }

  function positionHandles() {
    var s = session.displayScale;
    var c = session.corners;
    ['tl', 'tr', 'br', 'bl'].forEach(function (key) {
      session.handles[key].style.left = c[key].x * s + 'px';
      session.handles[key].style.top = c[key].y * s + 'px';
    });
    session.poly.setAttribute('points',
      [c.tl, c.tr, c.br, c.bl].map(function (p) { return (p.x * s) + ',' + (p.y * s); }).join(' '));
  }

  function attachDrag(handle, key, stage) {
    handle.addEventListener('pointerdown', function (e) {
      e.preventDefault();
      handle.setPointerCapture(e.pointerId);
      var move = function (ev) {
        var rect = stage.getBoundingClientRect();
        var x = clamp(ev.clientX - rect.left, 0, rect.width);
        var y = clamp(ev.clientY - rect.top, 0, rect.height);
        session.corners[key] = { x: x / session.displayScale, y: y / session.displayScale };
        positionHandles();
      };
      var up = function () {
        handle.removeEventListener('pointermove', move);
        handle.removeEventListener('pointerup', up);
        handle.removeEventListener('pointercancel', up);
      };
      handle.addEventListener('pointermove', move);
      handle.addEventListener('pointerup', up);
      handle.addEventListener('pointercancel', up);
    });
  }

  // ---- step 2: warp + filter preview ----
  function startPreview() {
    setStatus(SPIN + '<p>Processing…</p>');
    // Defer so the spinner paints before the synchronous warp blocks the thread.
    setTimeout(function () {
      if (!session) return; // cancelled before the warp ran
      try {
        session.warped = warp(session.work, session.corners);
      } catch (e) {
        setStatus('<p class="scanner-err">Could not process the scan.<br><small>' + escapeHtml(e.message) + '</small></p>');
        return;
      }
      session.filter = 'color';
      renderPreview();
    }, 30);
  }

  function warp(work, corners) {
    var wTop = dist(corners.tl, corners.tr);
    var wBot = dist(corners.bl, corners.br);
    var hLeft = dist(corners.tl, corners.bl);
    var hRight = dist(corners.tr, corners.br);
    var outW = clamp(Math.round(Math.max(wTop, wBot)), 100, MAX_OUT_W);
    var outH = clamp(Math.round(Math.max(hLeft, hRight)), 100, MAX_OUT_H);
    var points = {
      topLeftCorner: corners.tl,
      topRightCorner: corners.tr,
      bottomLeftCorner: corners.bl,
      bottomRightCorner: corners.br,
    };
    var canvas = jscan.extractPaper(work, outW, outH, points);
    if (!canvas) throw new Error('perspective transform failed');
    return canvas;
  }

  function renderPreview() {
    var out = el('canvas', { class: 'scanner-preview' });
    session.preview = out;
    applyFilter(session.warped, session.filter, out);
    fitPreview();

    session.body.innerHTML = '';
    var chips = el('div', { class: 'scanner-filters' });
    FILTERS.forEach(function (f) {
      var chip = el('button', { class: 'scanner-chip' + (f.id === session.filter ? ' on' : ''), type: 'button' }, [f.label]);
      chip.addEventListener('click', function () { setFilter(f.id); });
      chips.appendChild(chip);
    });
    session.body.appendChild(chips);
    session.body.appendChild(out);
    session.relayout = fitPreview;

    session.foot.innerHTML = '';
    var back = el('button', { class: 'btn ghost-d', type: 'button' }, ['Back']);
    back.addEventListener('click', showAdjust);
    var use = el('button', { class: 'btn btn-primary', type: 'button' }, ['Use scan']);
    use.addEventListener('click', finish);
    session.foot.appendChild(back);
    session.foot.appendChild(use);
  }

  function fitPreview() {
    var fit = fitDims(session.preview.width, session.preview.height);
    session.preview.style.width = fit.w + 'px';
    session.preview.style.height = fit.h + 'px';
  }

  function setFilter(id) {
    if (id === session.filter) return;
    session.filter = id;
    applyFilter(session.warped, id, session.preview);
    fitPreview();
    session.body.querySelectorAll('.scanner-chip').forEach(function (c, i) {
      c.classList.toggle('on', FILTERS[i].id === id);
    });
  }

  function applyFilter(srcCanvas, filter, outCanvas) {
    var cv = window.cv;
    var src = cv.imread(srcCanvas);
    var rgb = new cv.Mat();
    cv.cvtColor(src, rgb, cv.COLOR_RGBA2RGB);
    if (filter === 'bw') {
      var gray = new cv.Mat();
      cv.cvtColor(rgb, gray, cv.COLOR_RGB2GRAY);
      var bw = new cv.Mat();
      cv.adaptiveThreshold(gray, bw, 255, cv.ADAPTIVE_THRESH_GAUSSIAN_C, cv.THRESH_BINARY, blockSize(gray), 12);
      cv.imshow(outCanvas, bw);
      gray.delete(); bw.delete();
    } else if (filter === 'gray') {
      var g = new cv.Mat();
      cv.cvtColor(rgb, g, cv.COLOR_RGB2GRAY);
      var ge = new cv.Mat();
      cv.convertScaleAbs(g, ge, 1.15, 5);
      cv.imshow(outCanvas, ge);
      g.delete(); ge.delete();
    } else {
      var ce = new cv.Mat();
      cv.convertScaleAbs(rgb, ce, 1.2, 6);
      cv.imshow(outCanvas, ce);
      ce.delete();
    }
    src.delete();
    rgb.delete();
  }

  function blockSize(mat) {
    var b = Math.floor(Math.min(mat.cols, mat.rows) / 20);
    if (b % 2 === 0) b += 1;
    return clamp(b, 11, 51);
  }

  // ---- finish: preview canvas -> JPEG -> single-page PDF -> deliver ----
  function finish() {
    setStatus(SPIN + '<p>Building PDF…</p>');
    var preview = session.preview;
    var w = preview.width, h = preview.height;
    var deliver = session.deliver; // captured before any async gap can close the session
    preview.toBlob(function (blob) {
      if (!session) return; // cancelled while encoding
      if (!blob) {
        setStatus('<p class="scanner-err">Could not encode the scan.</p>');
        return;
      }
      blob.arrayBuffer().then(function (buf) {
        if (!session) return; // cancelled while reading
        var pdf = buildPdf(new Uint8Array(buf), w, h);
        var file = new File([pdf], 'scan-' + Date.now() + '.pdf', { type: 'application/pdf' });
        closeScanner();
        deliver(file);
      }).catch(function (e) {
        if (session) setStatus('<p class="scanner-err">Could not build the PDF.<br><small>' + escapeHtml(e.message) + '</small></p>');
      });
    }, 'image/jpeg', JPEG_QUALITY);
  }

  // buildPdf wraps a JPEG in a minimal single-page PDF (DCTDecode). The page is
  // A4-width, height derived from the image aspect, image drawn to fill.
  function buildPdf(jpeg, pxW, pxH) {
    var pageW = 595.28;
    var pageH = +(pageW * pxH / pxW).toFixed(2);
    var enc = new TextEncoder();
    var chunks = [];
    var len = 0;
    var offsets = [];

    function put(data) {
      var bytes = typeof data === 'string' ? enc.encode(data) : data;
      chunks.push(bytes);
      len += bytes.length;
    }
    function obj(n, body) { offsets[n] = len; put(n + ' 0 obj\n' + body + '\nendobj\n'); }

    put('%PDF-1.4\n');
    put(new Uint8Array([0x25, 0xE2, 0xE3, 0xCF, 0xD3, 0x0A])); // binary marker

    obj(1, '<< /Type /Catalog /Pages 2 0 R >>');
    obj(2, '<< /Type /Pages /Kids [3 0 R] /Count 1 >>');
    obj(3, '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ' + pageW + ' ' + pageH + ']' +
      ' /Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>');

    // Image XObject with inline JPEG stream.
    offsets[4] = len;
    put('4 0 obj\n<< /Type /XObject /Subtype /Image /Width ' + pxW + ' /Height ' + pxH +
      ' /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ' + jpeg.length + ' >>\nstream\n');
    put(jpeg);
    put('\nendstream\nendobj\n');

    var content = 'q ' + pageW + ' 0 0 ' + pageH + ' 0 0 cm /Im0 Do Q';
    obj(5, '<< /Length ' + content.length + ' >>\nstream\n' + content + '\nendstream');

    var xrefStart = len;
    var count = 6;
    var xref = 'xref\n0 ' + count + '\n0000000000 65535 f \n';
    for (var i = 1; i < count; i++) {
      xref += String(offsets[i]).padStart(10, '0') + ' 00000 n \n';
    }
    put(xref);
    put('trailer\n<< /Size ' + count + ' /Root 1 0 R >>\nstartxref\n' + xrefStart + '\n%%EOF\n');

    var pdf = new Uint8Array(len);
    var at = 0;
    chunks.forEach(function (c) { pdf.set(c, at); at += c.length; });
    return pdf;
  }

  // ---- delivery into the existing capture flow ----
  // Add mode: merge into the staged file input (app.js re-renders thumbnails).
  // Edit mode: set the immediate-upload input, which htmx posts on change.
  function deliverToAdd(cap) {
    return function (file) {
      var input = cap.querySelector('[data-files]');
      var dt = new DataTransfer();
      dt.items.add(file);
      input.files = dt.files;
      input.dispatchEvent(new Event('change', { bubbles: true }));
    };
  }

  function deliverToEdit(wrap) {
    return function (file) {
      var input = wrap.querySelector('[data-edit-upload]');
      var dt = new DataTransfer();
      dt.items.add(file);
      input.files = dt.files;
      input.dispatchEvent(new Event('change', { bubbles: true }));
    };
  }

  // ---- wiring ----
  document.addEventListener('click', function (e) {
    var btn = e.target.closest('[data-scan]');
    if (!btn) return;
    var wrap = btn.closest('[data-capture], [data-capture-edit-wrap]');
    if (!wrap) return;
    var input = wrap.querySelector('[data-scan-input]');
    if (input) input.click();
  });

  document.addEventListener('change', function (e) {
    if (!e.target.matches('[data-scan-input]')) return;
    var input = e.target;
    var file = input.files && input.files[0];
    input.value = ''; // allow re-scanning the same source
    if (!file) return;
    var wrap = input.closest('[data-capture], [data-capture-edit-wrap]');
    var deliver = wrap.matches('[data-capture]') ? deliverToAdd(wrap) : deliverToEdit(wrap);
    beginScan(file, deliver);
  });
})();
