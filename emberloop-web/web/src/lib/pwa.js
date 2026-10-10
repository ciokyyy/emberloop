// PWA service worker registration at app start. Push opt-in (push.js) also
// registers /sw.js, but the installable shell must exist before the user ever
// opens settings, so main.js calls registerSW() on boot. Safe to call twice:
// a repeat registration for the same URL/scope is a no-op update check.
export async function registerSW() {
  if (typeof window === 'undefined') return null;
  if (!('serviceWorker' in navigator)) return null;
  // Dev (vite) serves no sw.js: skip so the console stays clean.
  if (window.location.hostname !== 'localhost' && !window.isSecureContext) return null;
  try {
    const reg = await navigator.serviceWorker.register('/sw.js', { scope: '/' });
    return reg;
  } catch {
    // No service worker (private mode, unsupported browser): the app works
    // the same, just without offline shell or push.
    return null;
  }
}
