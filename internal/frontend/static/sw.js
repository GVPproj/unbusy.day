// Minimal passthrough service worker.
//
// It caches nothing and intercepts nothing — its only job is to exist so iOS
// classifies the home-screen app as an installed PWA and gives it a durable
// storage container, instead of the ephemeral "web clip" container that WebKit
// reclaims aggressively (which was evicting the auth cookie ~daily). See
// docs/notes/ios-pwa-service-worker.md.
//
// Deliberately NO offline cache: the server render is the source of truth, so a
// stale cache would fight the architecture (CLAUDE.md — no SPA, no client-side
// business logic). This file is the whole service worker; keep it that way.

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => event.waitUntil(self.clients.claim()));

// Present only to satisfy WebKit's installability heuristic. Not calling
// respondWith() lets the browser handle every request normally over the network.
self.addEventListener("fetch", () => {});
