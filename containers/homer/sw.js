// Overrides Homer's PWA service worker. We don't use the dashboard offline, and the
// default worker's cache only hides config.yml edits. With no fetch handler nothing is
// ever served from cache; on activation it clears old caches and unregisters itself.
self.addEventListener('install', () => self.skipWaiting());

self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.map((key) => caches.delete(key)));
      await self.registration.unregister();
    })(),
  );
});
