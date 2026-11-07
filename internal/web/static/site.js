// CoffeeProof's script, after the sister site (github.com/alparslanakbas/wheyproof):
// the price chart's readout (frontend core/chart-hover.ts and the product
// page's handlers), the "new version" bar (core/app-update.service.ts) and
// the install card (core/pwa-install.service.ts). Its texts come from the
// page, in the page's language; the CSP allows no inline script.
(function () {
  'use strict';

  // ---- price chart -----------------------------------------------------------
  // Near an edge the tag leans inward, or half of it is clipped
  // (chart-hover.ts hoverAlign: 12 % and 88 % of the width).
  function align(x) {
    return x < 12 ? 'left' : x > 88 ? 'right' : 'center';
  }

  // The reading nearest the pointer (chart-hover.ts nearestPointIndex). The
  // readings sit at percentages of the box, so the pointer becomes one too.
  function nearest(svg, clientX, points) {
    var rect = svg.getBoundingClientRect();
    if (!rect.width || !points.length) return -1;
    var x = ((clientX - rect.left) * 100) / rect.width;
    var best = 0;
    var dist = Infinity;
    for (var i = 0; i < points.length; i++) {
      var d = Math.abs(points[i][0] - x);
      if (d < dist) {
        dist = d;
        best = i;
      }
    }
    return best;
  }

  // A chart box carries its readings as [x %, y %, date, price].
  function chart(box) {
    var points;
    try {
      points = JSON.parse(box.getAttribute('data-points'));
    } catch (e) {
      return;
    }
    var svg = box.querySelector('svg.chart');
    var cursor = box.querySelector('.cursor');
    var dot = box.querySelector('.cursor-dot');
    var tip = box.querySelector('.chart-tip');
    if (!svg || !cursor || !dot || !tip || !points || !points.length) return;
    var date = tip.querySelector('.chart-tip-date');
    var price = tip.querySelector('.chart-tip-price');

    function show(i) {
      if (i < 0) return;
      var p = points[i];
      cursor.setAttribute('x1', p[0] + '%');
      cursor.setAttribute('x2', p[0] + '%');
      dot.setAttribute('cx', p[0] + '%');
      dot.setAttribute('cy', p[1] + '%');
      date.textContent = p[2];
      price.textContent = p[3];
      tip.style.left = p[0] + '%';
      tip.style.top = p[1] + '%';
      tip.setAttribute('data-align', align(p[0]));
      tip.hidden = false;
      box.classList.add('reading');
    }

    function hide() {
      tip.hidden = true;
      box.classList.remove('reading');
    }

    svg.addEventListener('mousemove', function (e) {
      show(nearest(svg, e.clientX, points));
    });
    svg.addEventListener('mouseleave', hide);
    // mousemove doesn't fire on touch screens, so a finger is followed on its
    // own. The page doesn't scroll while the finger is on the chart, and
    // lifting it keeps the last reading on screen, as on the sister site.
    function touch(e) {
      var t = e.touches[0];
      if (!t) return;
      e.preventDefault();
      show(nearest(svg, t.clientX, points));
    }
    svg.addEventListener('touchstart', touch, { passive: false });
    svg.addEventListener('touchmove', touch, { passive: false });
  }

  var boxes = document.querySelectorAll('.chart-box[data-points]');
  for (var i = 0; i < boxes.length; i++) chart(boxes[i]);

  // ---- new version bar -------------------------------------------------------
  // An open tab doesn't notice a deploy. The service worker file changes with
  // every build; once a new one is installed the bar asks for a reload, and
  // the new worker takes over in the background right away, so the next
  // request already goes through it. A worker never looks for updates by
  // itself: the page checks on load and every 10 minutes.
  var bar = document.getElementById('update-bar');
  var registration = null;

  function updateReady(worker) {
    if (!navigator.serviceWorker.controller) return; // the first install, not an update
    if (bar) bar.hidden = false;
    worker.postMessage('SKIP_WAITING');
  }

  if ('serviceWorker' in navigator) {
    navigator.serviceWorker
      .register('/sw.js')
      .then(function (reg) {
        registration = reg;
        if (reg.waiting) updateReady(reg.waiting);
        reg.addEventListener('updatefound', function () {
          var worker = reg.installing;
          if (!worker) return;
          worker.addEventListener('statechange', function () {
            if (worker.state === 'installed') updateReady(worker);
          });
        });
        reg.update().catch(function () {});
        setInterval(function () {
          reg.update().catch(function () {});
        }, 10 * 60 * 1000);
      })
      .catch(function () {});
  }

  if (bar) {
    // A reload alone changes nothing while the new worker still waits: it is
    // told to take over first, and the reload comes after (a production bug
    // on the sister site).
    bar.querySelector('[data-reload]').addEventListener('click', function () {
      var waiting = registration && registration.waiting;
      if (!waiting) {
        location.reload();
        return;
      }
      var done = false;
      function reload() {
        if (done) return;
        done = true;
        location.reload();
      }
      navigator.serviceWorker.addEventListener('controllerchange', reload);
      waiting.postMessage('SKIP_WAITING');
      setTimeout(reload, 3000);
    });
    bar.querySelector('[data-close]').addEventListener('click', function () {
      bar.hidden = true;
    });
  }

  // ---- install card ----------------------------------------------------------
  // The browser fires beforeinstallprompt only where the site can be installed
  // and isn't yet (Chromium: Android Chrome, desktop Chrome and Edge). With the
  // app installed, or opened as the app, it never fires and the card stays
  // hidden; iOS Safari never fires it either. Closing the card is remembered,
  // under the sister site's key.
  var card = document.getElementById('install-card');
  var KEY = 'pwa-install-dismissed';
  var dismissed = false;
  try {
    dismissed = !!localStorage.getItem(KEY);
  } catch (e) {}
  if (card && !dismissed) {
    var deferred = null;
    window.addEventListener('beforeinstallprompt', function (e) {
      e.preventDefault();
      deferred = e;
      card.hidden = false;
    });
    window.addEventListener('appinstalled', function () {
      deferred = null;
      card.hidden = true;
    });
    card.querySelector('[data-install]').addEventListener('click', function () {
      if (!deferred) return;
      var offer = deferred;
      deferred = null;
      offer.prompt();
      offer.userChoice.then(
        function () {
          card.hidden = true;
        },
        function () {
          card.hidden = true;
        },
      );
    });
    card.querySelector('[data-close]').addEventListener('click', function () {
      card.hidden = true;
      try {
        localStorage.setItem(KEY, '1');
      } catch (e) {}
    });
  }
})();
