// Service worker: makes the app installable and lets it open instantly.
//
// The app's own files are answered from the phone's copy straight away and
// checked against the frame in the background, so opening the app never
// waits on the frame, which is a small board that may be busy drawing. When
// the check finds new frame software the page is told, and reloads itself.
// The API is never handled here; the app keeps its own record of answers.
const CACHE = 'photoframe-v4';
const SHELL = ['/', '/app.css', '/app.js', '/manifest.webmanifest', '/icons/icon.svg', '/icons/icon-192.png', '/icons/dev-badge.png', '/icons/dev-badge-full.png'];
// A change to one of these means a new version of the app.
const CODE = ['/', '/app.css', '/app.js'];

self.addEventListener('install', (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(SHELL)).then(() => self.skipWaiting()));
});

self.addEventListener('activate', (e) => {
  e.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

// refresh fetches key from the frame and stores it. When that turns out to
// be new app code, it brings the rest of the code up to date as well before
// telling the page, so a reload gets one version of everything.
async function refresh(request, key, had) {
  const res = await fetch(request);
  if (!res.ok) return res;
  const cache = await caches.open(CACHE);
  await cache.put(key, res.clone());
  const was = had && had.headers.get('ETag');
  const now = res.headers.get('ETag');
  const path = typeof key === 'string' ? key : new URL(key.url).pathname;
  if (was && now && was !== now && CODE.includes(path)) {
    await Promise.all(CODE.filter((p) => p !== path).map(async (p) => {
      const r = await fetch(p);
      if (r.ok) await cache.put(p, r);
    }));
    for (const client of await self.clients.matchAll()) client.postMessage({ type: 'app-updated' });
  }
  return res;
}

self.addEventListener('fetch', (e) => {
  const url = new URL(e.request.url);
  if (e.request.method !== 'GET' || url.origin !== location.origin || url.pathname.startsWith('/api/')) return;
  const key = e.request.mode === 'navigate' ? '/' : e.request;
  e.respondWith((async () => {
    const had = await caches.match(key);
    const fresh = refresh(e.request, key, had);
    if (!had) return fresh;
    e.waitUntil(fresh.catch(() => { /* frame unreachable: the copy will do */ }));
    return had;
  })());
});
