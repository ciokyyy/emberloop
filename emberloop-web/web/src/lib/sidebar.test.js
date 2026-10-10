import { describe, it, expect, beforeEach } from 'vitest';
import {
  tierOf,
  splitTiers,
  rootTiers,
  buildTree,
  groupAllDone,
  sessionLabel,
  matchSession,
  loadIdSet,
  storeIdSet,
  isCollapsed,
  togglePref,
  readJSON,
  writeJSON,
} from './sidebar.js';

beforeEach(() => localStorage.clear());

describe('tierOf', () => {
  it('tiers failed sessions as needs-you', () => {
    expect(tierOf({ status: 'failed' })).toBe('needs');
  });
  it('tiers an explicit needs-you status or flag as needs-you', () => {
    expect(tierOf({ status: 'needs-you' })).toBe('needs');
    expect(tierOf({ status: 'idle', needsYou: 'same refusal' })).toBe('needs');
  });
  it('tiers working sessions as working', () => {
    expect(tierOf({ status: 'working' })).toBe('working');
  });
  it('tiers idle and ended sessions as recent', () => {
    expect(tierOf({ status: 'idle' })).toBe('recent');
    expect(tierOf({ status: 'ended' })).toBe('recent');
    expect(tierOf({})).toBe('recent');
  });
});

describe('splitTiers', () => {
  it('buckets sessions keeping their order', () => {
    const sessions = [
      { id: 'a', status: 'idle' },
      { id: 'b', status: 'failed' },
      { id: 'c', status: 'working' },
      { id: 'd', status: 'idle' },
    ];
    const tiers = splitTiers(sessions);
    expect(tiers.needs.map((s) => s.id)).toEqual(['b']);
    expect(tiers.working.map((s) => s.id)).toEqual(['c']);
    expect(tiers.recent.map((s) => s.id)).toEqual(['a', 'd']);
  });
});

describe('buildTree', () => {
  it('nests children under their parent and keeps orphans as roots', () => {
    const sessions = [
      { id: 'p' },
      { id: 'k1', parentId: 'p' },
      { id: 'k2', parentId: 'p' },
      { id: 'o', parentId: 'missing' },
    ];
    const { roots, kids } = buildTree(sessions);
    expect(roots.map((s) => s.id)).toEqual(['p', 'o']);
    expect(kids.get('p').map((s) => s.id)).toEqual(['k1', 'k2']);
  });
});

describe('groupAllDone', () => {
  it('is true only when every child settled', () => {
    expect(groupAllDone([])).toBe(false);
    expect(groupAllDone([{ status: 'ended' }, { status: 'failed' }])).toBe(true);
    expect(groupAllDone([{ status: 'ended' }, { status: 'idle' }])).toBe(false);
    expect(groupAllDone([{ status: 'working' }])).toBe(false);
  });
});

describe('sessionLabel', () => {
  it('prefers the spawn name, then the title, then the harness label', () => {
    expect(sessionLabel({ name: 'kid', title: 'T', harness: 'claude', id: 'abcdef123' })).toBe('kid');
    expect(sessionLabel({ title: 'T', harness: 'claude', id: 'abcdef123' })).toBe('T');
    expect(sessionLabel({ harness: 'claude', id: 'abcdef123' })).toBe('claude · abcdef12');
  });
});

describe('matchSession', () => {
  const s = { title: 'Fix sidebar', harness: 'claude', id: 'x' };
  it('matches labels, harnesses and project names case-insensitively', () => {
    expect(matchSession(s, 'emberloop', '')).toBe(true);
    expect(matchSession(s, 'emberloop', 'sidebar')).toBe(true);
    expect(matchSession(s, 'emberloop', 'CLAUDE')).toBe(true);
    expect(matchSession(s, 'emberloop', 'ember')).toBe(true);
    expect(matchSession(s, 'emberloop', 'wick')).toBe(false);
  });
});

describe('persisted collapse prefs', () => {
  it('round-trips id sets through localStorage', () => {
    storeIdSet('k', new Set(['a', 'b']));
    expect(loadIdSet('k')).toEqual(new Set(['a', 'b']));
    expect(loadIdSet('missing')).toEqual(new Set());
  });
  it('prefers explicit overrides over the dynamic default', () => {
    expect(isCollapsed({}, 'p', true)).toBe(true);
    expect(isCollapsed({ p: false }, 'p', true)).toBe(false);
    expect(isCollapsed({ p: true }, 'p', false)).toBe(true);
    expect(togglePref({}, 'p', true)).toEqual({ p: false });
    expect(togglePref({ p: false }, 'p', true)).toEqual({ p: true });
  });
  it('reads and writes sidebar collapse JSON safely', () => {
    expect(readJSON('nope', false)).toBe(false);
    writeJSON('k', true);
    expect(readJSON('k', false)).toBe(true);
  });
});

describe('rootTiers', () => {
  it('shows a Subagent only under its parent, and lifts the parent to the busiest tier', () => {
    const sessions = [
      { id: 'p', status: 'idle' },
      { id: 'c1', status: 'working', parentId: 'p' },
      { id: 'c2', status: 'idle', parentId: 'p' },
      { id: 'o', status: 'idle' },
    ];
    const t = rootTiers(sessions);
    expect(t.working.map((s) => s.id)).toEqual(['p']);
    expect(t.recent.map((s) => s.id)).toEqual(['o']);
    expect(t.needs).toEqual([]);
  });

  it('keeps an orphan as a root in its own tier', () => {
    const t = rootTiers([{ id: 'x', status: 'working', parentId: 'gone' }]);
    expect(t.working.map((s) => s.id)).toEqual(['x']);
  });
});
