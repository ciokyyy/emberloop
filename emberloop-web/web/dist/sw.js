// Service worker: shows Web Push notifications and focuses the app on click.
self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { body: event.data ? event.data.text() : '' };
  }
  event.waitUntil(
    self.registration.showNotification(data.title || 'Enderbloom', {
      body: data.body || '',
      tag: data.tag,
      icon: './favicon.png',
      data: { sessionId: data.sessionId },
    }),
  );
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((all) => {
      if (all.length) return all[0].focus();
      return self.clients.openWindow('./');
    }),
  );
});
