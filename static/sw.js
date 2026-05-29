// Service Worker — offline strategy per tech-design §4.2.
// cache-first for static assets; network-first w/ cache fallback for full-page
// navigations AND for HTMX partial GETs (so tapping through to a recipe works
// offline, not just a full reload).
//
// IMPORTANT: a route like /recipe/{id} returns a FULL page for a normal
// navigation but only the inner content fragment when requested by HTMX (it
// varies on the HX-Request header, with no Vary). Caching both under the same
// key would serve a bare fragment to a full navigation (no <html>/CSS) or a
// whole document into an HTMX swap target. We therefore key HTMX partials into a
// separate cache so the two representations never collide.

const SHELL_CACHE = 'cooking-shell-v2';
const HTMX_CACHE = 'cooking-htmx-v2';
const KEEP = [SHELL_CACHE, HTMX_CACHE];

const SHELL = [
  '/',
  '/static/css/app.css',
  '/static/js/htmx.min.js',
  '/static/js/cooking-steps.js',
  '/static/js/shopping-filter.js',
  '/manifest.webmanifest',
  '/static/icons/icon-192.png',
  '/static/icons/icon-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(SHELL_CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => !KEEP.includes(k)).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  if (request.method !== 'GET') {
    return;
  }

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) {
    return;
  }

  // cache-first for static assets
  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(request).then((cached) => cached || fetchAndCache(request, SHELL_CACHE))
    );
    return;
  }

  // HTMX partial GET (hx-get) — network-first against a dedicated cache so the
  // fragment never overwrites the full-page entry for the same URL.
  if (request.headers.get('HX-Request') === 'true') {
    event.respondWith(networkFirst(request, HTMX_CACHE));
    return;
  }

  // full-page navigation — network-first, fall back to cache, then the shell.
  if (request.mode === 'navigate') {
    event.respondWith(
      networkFirst(request, SHELL_CACHE).catch(() =>
        caches.match(request).then((c) => c || caches.match('/'))
      )
    );
  }
});

// networkFirst tries the network and caches a successful response, falling back
// to the cached copy when offline.
function networkFirst(request, cacheName) {
  return fetchAndCache(request, cacheName).catch(() =>
    caches.match(request).then((cached) => {
      if (cached) return cached;
      throw new Error('offline and uncached');
    })
  );
}

function fetchAndCache(request, cacheName) {
  return fetch(request).then((response) => {
    if (response && response.ok) {
      const copy = response.clone();
      caches.open(cacheName).then((cache) => cache.put(request, copy));
    }
    return response;
  });
}
