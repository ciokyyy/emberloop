// Web Push client: subscribe this browser through the service worker and
// register the subscription with the Daemon.
import { apiBase } from './api.js';

const url = (path) => apiBase() + path;

// Why push cannot work here, or '' when it can. Browsers allow push only in
// a secure context: https, or http on localhost. A plain http tailnet IP is
// not secure; serve the UI over https (`tailscale serve`).
export function pushUnsupported() {
  if (typeof window === 'undefined') return 'No window.';
  if (!window.isSecureContext) return 'Push needs https or localhost. Open the UI through `tailscale serve` (https), not a plain http tailnet address.';
  if (!('serviceWorker' in navigator) || !('PushManager' in window) || !('Notification' in window)) return 'This browser has no Web Push support.';
  return '';
}

const toKey = (s) => Uint8Array.from(atob(s.replace(/-/g, '+').replace(/_/g, '/')), (c) => c.charCodeAt(0));

async function registration() {
  await navigator.serviceWorker.register('/sw.js');
  return navigator.serviceWorker.ready;
}

export async function pushEnabled() {
  if (pushUnsupported()) return false;
  const reg = await navigator.serviceWorker.getRegistration('/sw.js');
  return !!(reg && (await reg.pushManager.getSubscription()));
}

export async function enablePush() {
  const why = pushUnsupported();
  if (why) throw new Error(why);
  if ((await Notification.requestPermission()) !== 'granted') throw new Error('Notifications are blocked for this site.');
  const { key } = await fetch(url('/api/push/key')).then((r) => r.json());
  const reg = await registration();
  const sub =
    (await reg.pushManager.getSubscription()) ??
    (await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: toKey(key) }));
  const res = await fetch(url('/api/push/subscribe'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(sub.toJSON()),
  });
  if (!res.ok) throw new Error(`subscribe failed: ${res.status}`);
}

export async function disablePush() {
  const reg = await navigator.serviceWorker.getRegistration('/sw.js');
  const sub = await reg?.pushManager.getSubscription();
  if (!sub) return;
  await fetch(url('/api/push/subscribe'), {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ endpoint: sub.endpoint }),
  });
  await sub.unsubscribe();
}
