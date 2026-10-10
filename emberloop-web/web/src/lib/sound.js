// Interface sounds: uisfx@0.4.0 cues (MIT code, CC0 audio), pack `minimal`
// by default. The mp3s are vendored under `public/sounds/<pack>/<cue>.mp3`
// and played by relative URL through HTMLAudio: relative URLs do not bundle,
// so they must live under `public/`. The uisfx package is the source of the
// files and of the pack list below.
//
// One player over those files, client-only. It knows about semantic events
// (turn done, failed, sudo request, ...) and a per-device preference; it
// knows nothing about Sessions or Runs — the callers map their transitions
// to events. The service worker also plays through here: its push handler
// messages open tabs with a cue, which lands in `playCue`.
//
// The player is built on the first real gesture, not on load: browsers
// refuse audio nobody asked for, and a cue that arrives before that is
// dropped rather than queued, because a stale chime from before the person
// touched the page is worse than none.
import { packNames } from 'uisfx';

// The feel for productivity and system UI: dry, precise, almost invisible.
// A run notifies roughly once per ticket for hours, so the sound that
// disappears into the background is the right one.
export const SOUND_PACK = 'minimal';

// All twelve uisfx packs, in the package's own order. Every pack carries
// every cue below, so switching packs never loses a sound.
export const PACKS = [...packNames];

// Long enough that a ticket folding into the Run that raised it cannot
// double up, short enough that the next ticket is heard.
export const SOUND_THROTTLE_MS = 400;

export const SOUND_KEY = 'el.sound';

// Semantic event -> uisfx cue. Message sent and subagent finished are
// deliberately absent: they sound nothing.
export const EVENT_CUES = {
  turnDone: 'notification',
  failed: 'error',
  sudo: 'mention',
  needsYou: 'blocked',
  complete: 'complete',
  voice: 'hover',
};

// The settings rows, in display order.
export const SOUND_EVENTS = [
  { id: 'turnDone', label: 'Turn done', hint: 'A chat turn finishes.' },
  { id: 'failed', label: 'Turn failed', hint: 'A chat turn fails.' },
  { id: 'sudo', label: 'Sudo request', hint: 'An agent asks to run sudo.' },
  { id: 'needsYou', label: 'Runs needs-you', hint: 'A Run ticket stalls and needs you.' },
  { id: 'complete', label: 'Ticket merged', hint: 'A ticket merges or its Run completes.' },
  { id: 'voice', label: 'Voice ticks', hint: 'Quiet tick when recording starts and stops.' },
];

const CUE_EVENTS = Object.fromEntries(Object.entries(EVENT_CUES).map(([event, cue]) => [cue, event]));

// Most urgent first. Two events inside one throttle window play only the
// more urgent one, so a Run that blocks on the ticket it just merged makes
// one noise. The voice tick is last: it is ambience, never news.
const URGENCY = ['blocked', 'error', 'mention', 'notification', 'complete', 'hover'];

/** Whether `a` demands more of the user than `b`. False for the same cue on
 * both sides, so one event never chimes twice. */
export function moreUrgent(a, b) {
  return URGENCY.indexOf(a) < URGENCY.indexOf(b);
}

/** FNV-1a over the UTF-8 reason, matching push.go's needsYouKey: the Run
 * SSE and the push message name the same event, so a tab that already
 * sounded it over its live stream stays silent while a new reason sounds. */
export function reasonHash(reason) {
  let h = 0x811c9dc5;
  for (const b of new TextEncoder().encode(reason ?? '')) {
    h ^= b;
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(16);
}

export const DEFAULT_PREFS = {
  pack: SOUND_PACK,
  volume: 0.7,
  muted: false,
  events: Object.fromEntries(SOUND_EVENTS.map((e) => [e.id, true])),
};
const cueUrl = (pack, cue) => `/sounds/${pack}/${cue}.mp3`;

// The voice tick plays under the master volume: it marks a mic transition,
// it never interrupts.
const VOICE_SCALE = 0.35;

/** A stored volume outside 0-1 is not a setting anyone made; it reads as
 * the default. Writes clamp instead (see setPreference), so a drag past
 * the end keeps the intent. */
function validVolume(volume) {
  return typeof volume === 'number' && !Number.isNaN(volume) && volume >= 0 && volume <= 1
    ? volume
    : DEFAULT_PREFS.volume;
}


function clampVolume(volume) {
  return typeof volume !== 'number' || Number.isNaN(volume) ? DEFAULT_PREFS.volume : Math.min(1, Math.max(0, volume));
}

/** A stored value that is missing, malformed or out of range is not a
 * setting anyone made; it reads as the default rather than as silence or
 * full volume. Unknown packs fall back to minimal; unknown event keys are
 * dropped so a removed toggle cannot linger. */
export function readPrefs(store) {
  const fallback = () => JSON.parse(JSON.stringify(DEFAULT_PREFS));
  try {
    const saved = JSON.parse(store?.getItem(SOUND_KEY) ?? '{}') ?? {};
    const events = {};
    for (const e of SOUND_EVENTS) {
      events[e.id] = typeof saved.events?.[e.id] === 'boolean' ? saved.events[e.id] : true;
    }
    return {
      pack: PACKS.includes(saved.pack) ? saved.pack : DEFAULT_PREFS.pack,
      volume: validVolume(saved.volume ?? DEFAULT_PREFS.volume),
      muted: typeof saved.muted === 'boolean' ? saved.muted : false,
      events,
    };
  } catch {
    return fallback();
  }
}

function writePrefs(store, prefs) {
  try {
    store?.setItem(SOUND_KEY, JSON.stringify(prefs));
  } catch {
    // A full or blocked store costs the setting, not the sound.
  }
}

const defaultAudio = (url) => new Audio(url);
const browserStore = () => {
  try {
    return typeof localStorage === 'undefined' ? null : localStorage;
  } catch {
    return null;
  }
};

export function createSoundPlayer(deps = {}) {
  const makeAudio = deps.makeAudio ?? defaultAudio;
  const store = deps.store === undefined ? browserStore() : deps.store;
  const now = deps.now ?? Date.now;

  let prefs = readPrefs(store);
  let unlocked = false;
  let lastCue = null;
  let lastPlayedAt = 0;
  const seen = new Set();
  const seenOrder = [];

  function markSeen(key) {
    if (!key || seen.has(key)) return;
    seen.add(key);
    seenOrder.push(key);
    // Keys name events, and events are unique: the set only caps memory.
    if (seenOrder.length > 200) seen.delete(seenOrder.shift());
  }

  function warm(pack) {
    // Preload the pack after unlock so the first real cue starts at once.
    // A missing file fails here, silently, instead of on the cue.
    for (const cue of Object.values(EVENT_CUES)) {
      try {
        const el = makeAudio(cueUrl(pack, cue));
        el.preload = 'auto';
        el.load?.();
      } catch {
        // Sound is never worth an exception in the caller's path.
      }
    }
  }

  function unlock() {
    if (unlocked) return;
    unlocked = true;
    warm(prefs.pack);
  }

  function playCue(cue, { key = '', scale = 1 } = {}) {
    // Dropped before the first gesture: not recorded, so a later arrival
    // over the other channel may still sound it.
    if (!unlocked) return false;
    if (key && seen.has(key)) return false;
    // Muted or toggled off: the event occurred and stays silent. Recorded,
    // so enabling sound later does not replay stale news.
    if (prefs.muted) {
      markSeen(key);
      return false;
    }
    const event = CUE_EVENTS[cue];
    if (event && prefs.events[event] === false) {
      markSeen(key);
      return false;
    }
    const at = now();
    // Two events inside one window: the worse one wins, and the one that
    // already sounded stands when the newcomer is no worse. A refused cue
    // spends neither the window nor its key: the same event arriving over
    // the other channel still gets its chance.
    if (lastCue !== null && at - lastPlayedAt < SOUND_THROTTLE_MS && !moreUrgent(cue, lastCue)) return false;
    let el;
    try {
      el = makeAudio(cueUrl(prefs.pack, cue));
    } catch {
      return false;
    }
    if (!el) return false;
    try {
      el.volume = clampVolume(prefs.volume * scale);
      const played = el.play();
      // A rejected play (missing file, torn-down page) still counts the
      // window: the cue was attempted, not dropped.
      played?.catch?.(() => {});
      markSeen(key);
      lastCue = cue;
      lastPlayedAt = at;
      return true;
    } catch {
      return false;
    }
  }

  /** Play a semantic event if sound is allowed, audio is unlocked and
   * nothing louder said so inside the throttle window. Never throws. Says
   * whether the cue sounded. A repeated key (the same push arriving over
   * both the socket and the service worker message) sounds once. */
  function playEvent(id, { key = '' } = {}) {
    const cue = EVENT_CUES[id];
    if (!cue) return false;
    if (key && seen.has(key)) return false;
    return playCue(cue, { key, scale: id === 'voice' ? VOICE_SCALE : 1 });
  }

  /** The settings preview: an explicit user action, so it bypasses unlock,
   * mute, toggles and the throttle, at the master volume. */
  function preview(pack) {
    if (!PACKS.includes(pack)) return false;
    try {
      const el = makeAudio(cueUrl(pack, EVENT_CUES.turnDone));
      if (!el) return false;
      el.volume = clampVolume(prefs.volume);
      el.play()?.catch?.(() => {});
      return true;
    } catch {
      return false;
    }
  }

  function setPreference(next) {
    const events = {};
    for (const e of SOUND_EVENTS) {
      events[e.id] = typeof next.events?.[e.id] === 'boolean' ? next.events[e.id] : (prefs.events[e.id] ?? true);
    }
    const pack = PACKS.includes(next.pack) ? next.pack : prefs.pack;
    prefs = {
      pack,
      volume: clampVolume(next.volume ?? prefs.volume),
      muted: typeof next.muted === 'boolean' ? next.muted : prefs.muted,
      events,
    };
    writePrefs(store, prefs);
    if (unlocked) warm(pack);
  }

  // The unlock listeners belong to this player: dispose drops them, so a
  // discarded player (a test makes one per case) stops listening.
  function dispose() {
    if (typeof document !== 'undefined') {
      document.removeEventListener('pointerdown', unlock, { capture: true });
      document.removeEventListener('keydown', unlock, { capture: true });
    }
  }

  if (typeof document !== 'undefined') {
    // Capture, so a cue played by the unlocking click itself still counts:
    // target-phase handlers run before document bubble listeners.
    document.addEventListener('pointerdown', unlock, { once: true, capture: true });
    document.addEventListener('keydown', unlock, { once: true, capture: true });
  }

  return {
    playEvent,
    playCue,
    preview,
    preference: () => JSON.parse(JSON.stringify(prefs)),
    setPreference,
    dispose,
  };
}

let shared = null;

/** The one player this app plays through, or `null` where there is no
 * document to unlock (SSR, a test without a DOM). Two importers get the
 * same one. */
export function soundPlayer() {
  if (shared) return shared;
  if (typeof window === 'undefined' || typeof document === 'undefined') return null;
  shared = createSoundPlayer();
  return shared;
}

// One Run snapshot for transition detection: status plus per-ticket state.
// The Runs view seeds it on open and diffs every snapshot after.
export function seedRun(run) {
  if (!run) return null;
  const tickets = {};
  for (const t of run.tickets ?? []) tickets[t.id] = { state: t.state, needsYou: t.needsYou ?? '' };
  return { status: run.status, tickets };
}

/** The sounding transitions between two Run snapshots, oldest first. Empty
 * on the first snapshot (opening a Run replays history; history is not
 * news). A ticket first seen already in needs-you sounds, and a changed
 * needs-you reason sounds again under its new key. Keys name the event,
 * so a push arriving over both the Run SSE and the service worker message
 * sounds once. */
export function runTransitions(prev, next) {
  if (!prev || !next) return [];
  const out = [];
  for (const t of next.tickets ?? []) {
    const was = prev.tickets?.[t.id];
    if (t.state === 'needs-you' && (was?.state !== 'needs-you' || (t.needsYou ?? '') !== (was.needsYou ?? ''))) {
      out.push({ event: 'needsYou', runId: next.id, ticketId: t.id, num: t.num, title: t.title, key: `needsyou:${next.id}:${t.id}:${reasonHash(t.needsYou ?? '')}` });
    } else if (was && t.state === 'merged' && was.state !== 'merged') {
      out.push({ event: 'complete', runId: next.id, ticketId: t.id, num: t.num, title: t.title, key: `merged:${next.id}:${t.id}` });
    }
  }
  if (next.status === 'done' && prev.status !== 'done') {
    out.push({ event: 'complete', runId: next.id, ticketId: '', key: `run:${next.id}:done` });
  }
  return out;
}
