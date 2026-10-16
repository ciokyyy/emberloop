// Cache only content-addressed build assets. HTML is user-specific (SSR) and
// must always reach the Daemon; /api/* and cross-origin requests are untouched.
const SHELL = 'el-assets-v1';
self.addEventListener('install', (event) => {
  event.waitUntil(self.skipWaiting());
});
self.addEventListener('activate', (event) => {
  event.waitUntil(
    (async () => {
      const keys = await caches.keys();
      await Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k)));
      await self.clients.claim();
    })(),
  );
});
self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);
  // API and SSE stay live: no cache, or the transcript stalls and the
  // version badge lies. Cross-origin (fonts, etc.) is left alone.
  if (url.pathname.startsWith('/api/')) return;
  if (url.origin !== self.location.origin) return;
  if (req.mode === 'navigate') return;
  if (!/(?:^|[-.])[A-Za-z0-9_-]{8,}\.[^/.]+$/.test(url.pathname)) return;
  event.respondWith(
    (async () => {
      try {
        const fresh = await fetch(req);
        if (fresh && fresh.ok) {
          const cache = await caches.open(SHELL);
          cache.put(req, fresh.clone()).catch(() => {});
        }
        return fresh;
      } catch {
        const cached = await caches.match(req, { ignoreSearch: false });
        if (cached) return cached;
        throw new Error('offline');
      }
    })(),
  );
});

// Service worker: plays push sounds through open tabs and shows an OS
// notification only when no window is visible. A custom sound inside a push
// notification is impossible (the OS plays its default), so the sound always
// comes from a tab, never from here.
self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : '' };
  }
  // Exact client route: /s/<id> for Sessions, /runs/<run>/<ticket> for Runs.
  const path = data.path || (data.sessionId ? `/s/${data.sessionId}` : '/');
  const sound = { type: 'el-sound', cue: data.cue, key: data.key };
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((all) => {
      // Every open tab gets the cue: the foreground tab sounds even while
      // visible, and key dedup keeps it to one chime when the live stream
      // already played the same event.
      for (const c of all) {
        try {
          c.postMessage(sound);
        } catch {
          // A tab going away costs nothing.
        }
      }
      const visible = all.some((c) => c.visibilityState === 'visible');
      // Unknown visibility counts as hidden: missing a sudo approval is
      // worse than a duplicate ping.
      if (visible) return undefined;
      const options = {
        body: data.body || '', tag: data.tag, icon: '/favicon.png',
        data: { path, profilePatchId: data.profilePatchId },
      };
      if (data.profilePatchId) options.actions = [{ action: 'approve', title: 'Approve' }, { action: 'reject', title: 'Reject' }];
      return self.registration.showNotification(data.title || 'Emberloop', options);
    }),
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const data = event.notification?.data ?? {};
  const path = data.path || (data.sessionId ? `/s/${data.sessionId}` : '/');
  event.waitUntil((async () => {
    if (data.profilePatchId && ['approve', 'reject'].includes(event.action)) {
      await fetch(`/api/profile/queue/${data.profilePatchId}/decide`, {
        method: 'POST', credentials: 'include', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ status: event.action === 'approve' ? 'approved' : 'rejected' }),
      });
    }
    const all = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    const client = all.find((c) => 'focus' in c);
    if (client) {
      try { client.postMessage({ type: 'el-navigate', path }); }
      catch { /* Focusing still lands the user in the app. */ }
      return client.focus();
    }
    return self.clients.openWindow(path);
  })());
});
