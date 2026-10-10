import { describe, expect, it, vi, afterEach, beforeEach } from 'vitest';
import {
  createSoundPlayer,
  runTransitions,
  seedRun,
  moreUrgent,
  readPrefs,
  reasonHash,
  DEFAULT_PREFS,
  SOUND_KEY,
  SOUND_PACK,
  SOUND_THROTTLE_MS,
  PACKS,
  EVENT_CUES,
} from './sound.js';

function memoryStore(initial = {}) {
  const data = { ...initial };
  return {
    data,
    getItem: (key) => data[key] ?? null,
    setItem: (key, value) => {
      data[key] = value;
    },
  };
}

// A stand-in for HTMLAudio. No AudioContext exists here, which is the
// point: the player has to be testable without one.
function fakeAudio(playReturns = 'ok') {
  const made = [];
  const makeAudio = (url) => {
    const el = {
      url,
      volume: 1,
      preload: '',
      played: 0,
      play: () => {
        if (playReturns === 'throw') throw new Error('no audio here');
        el.played += 1;
        if (playReturns === 'reject') return Promise.reject(new Error('missing file'));
        return Promise.resolve();
      },
      load: () => {},
    };
    made.push(el);
    return el;
  };
  return { made, makeAudio };
}

// Every player under test is disposed after its case: each one listens for
// the unlocking gesture on document, and without disposal they pile up.
const livePlayers = new Set();

function harness({ playReturns = 'ok', stored = {} } = {}) {
  const { made, makeAudio } = fakeAudio(playReturns);
  let clock = 1000;
  const store = memoryStore(stored);
  const player = createSoundPlayer({ makeAudio, store, now: () => clock });
  livePlayers.add(player);
  return {
    player,
    made,
    store,
    advance: (ms) => {
      clock += ms;
    },
  };
}

function press() {
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab' }));
}

function pointer() {
  document.dispatchEvent(new PointerEvent('pointerdown'));
}

describe('the sound catalogue', () => {
  it('plays the minimal feel by default, with all twelve uisfx packs on offer', () => {
    expect(SOUND_PACK).toBe('minimal');
    expect(DEFAULT_PREFS.pack).toBe('minimal');
    expect(PACKS).toEqual([
      'minimal',
      'soft',
      'glass',
      'arcade',
      'mechanical',
      'organic',
      'dreamy',
      'scifi',
      'rubber',
      'cinematic',
      'studio',
      'zen',
    ]);
  });

  it('maps every sounding event to a vendored cue', () => {
    expect(EVENT_CUES).toEqual({
      turnDone: 'notification',
      failed: 'error',
      sudo: 'mention',
      needsYou: 'blocked',
      complete: 'complete',
      voice: 'hover',
    });
  });

  it('orders urgency with the blocker first and the voice tick last', () => {
    expect(moreUrgent('blocked', 'error')).toBe(true);
    expect(moreUrgent('error', 'mention')).toBe(true);
    expect(moreUrgent('mention', 'notification')).toBe(true);
    expect(moreUrgent('notification', 'complete')).toBe(true);
    expect(moreUrgent('complete', 'hover')).toBe(true);
    expect(moreUrgent('notification', 'notification')).toBe(false);
    expect(moreUrgent('hover', 'blocked')).toBe(false);
  });
});

describe('the sound player', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  afterEach(() => {
    for (const player of livePlayers) {
      try {
        player.dispose();
      } catch {
        // Disposal never fails the case it cleans up after.
      }
      livePlayers.delete(player);
    }
    vi.unstubAllGlobals();
    vi.resetModules();
  });

  it('drops a cue asked for before the first gesture and never replays it', () => {
    const { player, made } = harness();

    expect(player.playEvent('turnDone'), 'a cue before the gesture is refused, not queued').toBe(false);
    expect(made, 'nothing is constructed before the user touches the page').toHaveLength(0);

    press();
    expect(player.playEvent('turnDone')).toBe(true);
    expect(made.map((el) => el.url)).toContain('/sounds/minimal/notification.mp3');
  });

  it('unlocks on the first pointer gesture as well as a key, even inside the unlocking click', () => {
    const { player, made } = harness();

    pointer();
    expect(player.playEvent('needsYou')).toBe(true);
    expect(made.map((el) => el.url)).toContain('/sounds/minimal/blocked.mp3');
  });

  it('plays the vendored file of the chosen pack at the master volume', () => {
    const { player, made } = harness();
    press();
    player.setPreference({ ...player.preference(), pack: 'glass', volume: 0.4 });
    made.length = 0;

    expect(player.playEvent('failed')).toBe(true);
    expect(made).toHaveLength(1);
    expect(made[0].url).toBe('/sounds/glass/error.mp3');
    expect(made[0].volume).toBeCloseTo(0.4);
  });

  it('plays the voice tick under the master volume', () => {
    const { player, made } = harness();
    press();
    player.setPreference({ ...player.preference(), volume: 0.8 });
    made.length = 0;

    expect(player.playEvent('voice')).toBe(true);
    expect(made[0].url).toBe('/sounds/minimal/hover.mp3');
    expect(made[0].volume).toBeLessThan(0.8);
  });

  it('says nothing while muted and reads a muted store as muted', () => {
    const { player, made } = harness({ stored: { [SOUND_KEY]: JSON.stringify({ ...DEFAULT_PREFS, muted: true }) } });
    press();
    expect(player.playEvent('sudo')).toBe(false);
    expect(made.filter((el) => el.played > 0)).toHaveLength(0);
  });

  it('honors the per-event toggles', () => {
    const { player, made } = harness();
    press();
    player.setPreference({ ...player.preference(), events: { ...player.preference().events, sudo: false } });
    made.length = 0;

    expect(player.playEvent('sudo', { key: 'sudo-1' })).toBe(false);
    expect(player.playEvent('turnDone', { key: 'turn-1' })).toBe(true);
    expect(made.filter((el) => el.played > 0).map((el) => el.url)).toEqual(['/sounds/minimal/notification.mp3']);
  });

  it('reads a missing, malformed or out-of-range preference as the default', () => {
    expect({ ...DEFAULT_PREFS, events: { ...DEFAULT_PREFS.events } }).toEqual({
      pack: 'minimal',
      volume: 0.7,
      muted: false,
      events: { turnDone: true, failed: true, sudo: true, needsYou: true, complete: true, voice: true },
    });

    for (const stored of [undefined, '', 'not json', '{}', '{"volume":"loud"}', '{"muted":"yes"}', '{"volume":9}', '{"pack":"nope"}']) {
      const { player } = harness({ stored: stored === undefined ? {} : { [SOUND_KEY]: stored } });
      expect(readPrefs(memoryStore(stored === undefined ? {} : { [SOUND_KEY]: stored })).pack, `stored ${stored}`).toBe('minimal');
      expect(player.preference().volume, `stored ${stored}`).toBe(0.7);
      expect(player.preference().muted, `stored ${stored}`).toBe(false);
    }
  });

  it('clamps the volume to 0-1, applies it and writes it back', () => {
    const { player, store } = harness();
    press();

    player.setPreference({ ...player.preference(), volume: 4 });
    expect(player.preference().volume).toBe(1);
    expect(JSON.parse(store.getItem(SOUND_KEY)).volume).toBe(1);

    player.setPreference({ ...player.preference(), volume: -3, muted: true });
    expect(player.preference().volume).toBe(0);
    expect(player.preference().muted).toBe(true);
    expect(JSON.parse(store.getItem(SOUND_KEY)).muted).toBe(true);
  });
  it('plays a second cue inside the window only when it is the more urgent one', () => {
    const { player, made, advance } = harness();
    press();
    const urls = () => made.filter((el) => el.played > 0).map((el) => el.url);

    player.playEvent('turnDone');
    advance(SOUND_THROTTLE_MS - 1);

    expect(player.playEvent('complete'), 'less urgent inside the window is refused').toBe(false);
    expect(player.playEvent('turnDone'), 'the same cue inside the window is refused').toBe(false);
    expect(urls()).toEqual(['/sounds/minimal/notification.mp3']);

    expect(player.playEvent('needsYou'), 'the blocker overtakes').toBe(true);
    expect(player.playEvent('failed'), 'nothing overtakes the blocker').toBe(false);
    expect(urls()).toEqual(['/sounds/minimal/notification.mp3', '/sounds/minimal/blocked.mp3']);

    advance(SOUND_THROTTLE_MS);
    expect(player.playEvent('failed')).toBe(true);
    expect(urls()).toHaveLength(3);
  });

  it('sounds one key once, however it arrives', () => {
    const { player, made, advance } = harness();
    press();

    expect(player.playEvent('sudo', { key: 'sudo-9' })).toBe(true);
    // The same push arriving over the service worker message sounds nothing.
    expect(player.playCue('mention', { key: 'sudo-9' })).toBe(false);
    expect(made.filter((el) => el.played > 0)).toHaveLength(1);

    // A new event still sounds.
    expect(player.playEvent('sudo', { key: 'sudo-10' })).toBe(false);
    advance(SOUND_THROTTLE_MS);
    expect(player.playEvent('sudo', { key: 'sudo-10' })).toBe(true);
  });

  it('the preview bypasses unlock, mute, toggles and the throttle', () => {
    const { player, made } = harness();
    player.setPreference({ ...player.preference(), muted: true, events: { ...player.preference().events, turnDone: false } });

    expect(player.preview('arcade')).toBe(true);
    expect(made).toHaveLength(1);
    expect(made[0].url).toBe('/sounds/arcade/notification.mp3');
    expect(player.preview('nope')).toBe(false);
  });

  it('treats a throwing factory as "did not play" and never throws at the caller', () => {
    const { player, made } = harness({ playReturns: 'throw' });
    press();
    expect(() => player.playEvent('needsYou')).not.toThrow();
    expect(made.filter((el) => el.played > 0)).toHaveLength(0);

    // The cue that never sounded does not spend the window.
    const { player: okPlayer, made: okMade } = harness();
    press();
    expect(okPlayer.playEvent('turnDone')).toBe(true);
    expect(okMade.filter((el) => el.played > 0)).toHaveLength(1);
  });

  it('survives a store that throws and a factory that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('blocked');
      },
      setItem: () => {
        throw new Error('blocked');
      },
    };
    const player = createSoundPlayer({
      makeAudio: () => {
        throw new Error('no audio here');
      },
      store: broken,
      now: () => 1,
    });
    livePlayers.add(player);
    press();
    expect(() => player.playEvent('sudo')).not.toThrow();
    expect(() => player.setPreference({ pack: 'minimal', volume: 0.5, muted: false, events: {} })).not.toThrow();
    expect(() => player.preview('minimal')).not.toThrow();
  });

  it('stops listening once disposed', () => {
    const { player, made } = harness();
    player.dispose();
    press();
    expect(player.playEvent('turnDone'), 'a disposed player never unlocks').toBe(false);
    expect(made, 'nothing is constructed after disposal').toHaveLength(0);
  });

  it('creates nothing during SSR and hands one player to every import', async () => {
    vi.stubGlobal('window', undefined);
    vi.stubGlobal('document', undefined);

    const fresh = await import('./sound.js');
    expect(fresh.soundPlayer()).toBeNull();

    vi.unstubAllGlobals();
    const { soundPlayer } = await import('./sound.js');
    expect(soundPlayer()).not.toBeNull();
    expect(soundPlayer()).toBe(soundPlayer());
  });
});

describe('runTransitions', () => {
  const run = (over = {}) => ({
    id: 'run-1',
    status: 'running',
    tickets: [
      { id: 't1', num: 1, title: 'First', state: 'running', needsYou: '' },
      { id: 't2', num: 2, title: 'Second', state: 'queued', needsYou: '' },
    ],
    ...over,
  });

  it('is silent on the first snapshot: opening a Run replays history, not news', () => {
    expect(runTransitions(null, run())).toEqual([]);
    expect(runTransitions(seedRun(run()), null)).toEqual([]);
  });

  it('names a ticket turning needs-you once', () => {
    const prev = seedRun(run());
    const next = run({ tickets: [{ id: 't1', num: 1, title: 'First', state: 'needs-you', needsYou: 'same refusal 3 times' }, run().tickets[1]] });
    expect(runTransitions(prev, next)).toEqual([
      { event: 'needsYou', runId: 'run-1', ticketId: 't1', num: 1, title: 'First', key: 'needsyou:run-1:t1:2e3b1cb8' },
    ]);
    // The same snapshot twice is not news twice.
    expect(runTransitions(seedRun(next), next)).toEqual([]);
  });

  it('names a merged ticket and a completed Run', () => {
    const prev = seedRun(run());
    const merged = run({ tickets: [{ ...run().tickets[0], state: 'merged' }, run().tickets[1]] });
    expect(runTransitions(prev, merged)).toEqual([
      { event: 'complete', runId: 'run-1', ticketId: 't1', num: 1, title: 'First', key: 'merged:run-1:t1' },
    ]);
    const done = { ...merged, status: 'done' };
    expect(runTransitions(seedRun(merged), done)).toEqual([
      { event: 'complete', runId: 'run-1', ticketId: '', key: 'run:run-1:done' },
    ]);
  });

  it('sounds a ticket first seen already in needs-you, and stays silent otherwise', () => {
    const prev = seedRun(run());
    const next = run({ tickets: [...run().tickets, { id: 't9', num: 9, title: 'New', state: 'needs-you', needsYou: 'x' }] });
    expect(runTransitions(prev, next)).toEqual([
      { event: 'needsYou', runId: 'run-1', ticketId: 't9', num: 9, title: 'New', key: 'needsyou:run-1:t9:fd0c5087' },
    ]);
    const parked = run({ tickets: [{ ...run().tickets[0], state: 'parked' }, run().tickets[1]] });
    expect(runTransitions(prev, parked)).toEqual([]);
    const merged = run({ tickets: [...run().tickets, { id: 't9', num: 9, title: 'New', state: 'merged', needsYou: '' }] });
    expect(runTransitions(prev, merged)).toEqual([]);
  });

  it('sounds a changed needs-you reason again under its new key', () => {
    const prev = seedRun(run({ tickets: [{ id: 't1', num: 1, title: 'First', state: 'needs-you', needsYou: 'same refusal 3 times' }, run().tickets[1]] }));
    const next = run({ tickets: [{ id: 't1', num: 1, title: 'First', state: 'needs-you', needsYou: 'idle 10m with no output' }, run().tickets[1]] });
    expect(runTransitions(prev, next)).toEqual([
      { event: 'needsYou', runId: 'run-1', ticketId: 't1', num: 1, title: 'First', key: 'needsyou:run-1:t1:18923055' },
    ]);
    // The same reason twice is not news twice.
    expect(runTransitions(seedRun(next), next)).toEqual([]);
  });

  it('hashes reasons the way push.go does (FNV-1a over UTF-8)', () => {
    expect(reasonHash('')).toBe('811c9dc5');
    expect(reasonHash('a')).toBe('e40c292c');
    expect(reasonHash('same refusal 3 times')).toBe('2e3b1cb8');
  });
});
