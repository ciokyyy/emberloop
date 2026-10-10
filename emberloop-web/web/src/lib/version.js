// Build identity: the UI's own build hash (stamped at vite build time) and
// the Daemon's answer from GET /api/version. The badge compares the two and
// offers a reload when the Daemon serves a newer UI build.
import { getVersion } from './api.js';

// uiHash is this bundle's own build hash, stamped into it by vite.config.js
// from the same source digest `el update` stamps into the Daemon. "dev"
// outside a stamped build (vite dev, tests): never stale, never a pill.
export function uiHash() {
  try {
    const h = typeof __EL_UI_HASH__ !== 'undefined' ? __EL_UI_HASH__ : '';
    return h || 'dev';
  } catch {
    return 'dev';
  }
}

// fetchVersion reads the Daemon's build identity, or null when it is
// unreachable or answers badly. Never throws: the badge is a diagnostic,
// and a failed fetch resolves to keeping the last good value.
export async function fetchVersion(get = getVersion) {
  try {
    const body = await get();
    if (!body || typeof body.uiHash !== 'string' || typeof body.commit !== 'string') return null;
    return { version: body.version ?? 'dev', commit: body.commit, uiHash: body.uiHash };
  } catch {
    return null;
  }
}

// isStale reports a browser running an older UI build than the Daemon
// serves. Either side unknown ("dev", empty) is not stale: an unstamped
// Daemon or a dev bundle cannot name a newer build.
export function isStale(own, remote) {
  if (!own || own === 'dev') return false;
  if (!remote || remote === 'dev') return false;
  return own !== remote;
}

// versionText is the badge line and the copied string.
export function versionText(own, commit) {
  return `ui ${own} · be ${commit}`;
}

// copyText copies to the clipboard, falling back to the legacy execCommand
// path where the async API is missing (plain-http tailnet origins are not a
// secure context). Resolves true only when the text landed.
export async function copyText(text) {
  try {
    if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch {
    // Fall through to the legacy path.
  }
  try {
    const area = document.createElement('textarea');
    area.value = text;
    area.style.position = 'fixed';
    area.style.opacity = '0';
    document.body.appendChild(area);
    area.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(area);
    return ok;
  } catch {
    return false;
  }
}
