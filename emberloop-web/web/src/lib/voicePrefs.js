// Voice input preferences, per device, in localStorage. Voice is on by
// default; cleanup (punctuation and filler fixes by a small cloud model) is
// off by default because it adds a model round trip.
export const VOICE_KEY = 'el.voice';
export const VOICE_EVENT = 'el:voice-prefs';
export const DEFAULT_VOICE = { enabled: true, cleanup: false };

function store() {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
}

export function readVoicePrefs() {
  try {
    const raw = store()?.getItem(VOICE_KEY);
    if (!raw) return { ...DEFAULT_VOICE };
    const p = JSON.parse(raw);
    return { enabled: p.enabled !== false, cleanup: p.cleanup === true };
  } catch {
    return { ...DEFAULT_VOICE };
  }
}

export function writeVoicePrefs(next) {
  const prefs = { enabled: next.enabled !== false, cleanup: next.cleanup === true };
  try {
    store()?.setItem(VOICE_KEY, JSON.stringify(prefs));
  } catch {
    // Storage blocked: the choice lasts for this page only.
  }
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(VOICE_EVENT, { detail: prefs }));
  return prefs;
}

// Calls cb(prefs) on every change from this tab or another; returns unsubscribe.
export function onVoicePrefs(cb) {
  if (typeof window === 'undefined') return () => {};
  const local = (e) => cb(e.detail);
  const remote = (e) => {
    if (e.key === VOICE_KEY) cb(readVoicePrefs());
  };
  window.addEventListener(VOICE_EVENT, local);
  window.addEventListener('storage', remote);
  return () => {
    window.removeEventListener(VOICE_EVENT, local);
    window.removeEventListener('storage', remote);
  };
}
