// CoffeeProof service worker, build {{version}}.
//
// What it is for: the site installs as an app, and open pages learn that a new
// deploy is out. This file carries the build (a hash of the binary, see
// buildVersion), so it changes with every deploy; the browser notices and the
// page shows its "new version" bar (static/site.js). The sister site gets the
// same from Angular's service worker.
//
// Pages are never answered from a cache: prices must be live. Only files under
// /static/ are kept, one cache per build; the CSS and the script carry their
// content hash in their address, the rest changes rarely.
const CACHE = 'static-{{version}}';

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((key) => key !== CACHE).map((key) => caches.delete(key))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('message', (event) => {
  if (event.data === 'SKIP_WAITING') self.skipWaiting();
});

self.addEventListener('fetch', (event) => {
  const request = event.request;
  if (request.method !== 'GET') return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || !url.pathname.startsWith('/static/')) return;
  event.respondWith(
    caches.open(CACHE).then((cache) =>
      cache.match(request).then(
        (hit) =>
          hit ||
          fetch(request).then((response) => {
            if (response.ok) cache.put(request, response.clone());
            return response;
          }),
      ),
    ),
  );
});
