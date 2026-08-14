// Passthrough service worker (ADR 0010): exists only so iOS treats this as an
// installed PWA with durable storage. Caches and intercepts nothing.
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));
self.addEventListener("fetch", () => {});