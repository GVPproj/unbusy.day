// Minimal passthrough service worker (ADR 0010). It caches nothing and
// intercepts nothing — its only job is to exist so iOS classifies the
// home-screen app as an installed PWA with durable storage (the ephemeral "web
// clip" container was evicting the auth cookie ~daily). Deliberately NO
// offline cache: the server render is the source of truth.

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

// Satisfies WebKit's installability heuristic; no respondWith() means the
// browser handles every request normally.
self.addEventListener("fetch", () => {});
