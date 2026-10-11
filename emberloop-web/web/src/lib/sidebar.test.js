import { describe, it, expect, beforeEach } from 'vitest';
import {
  buildTree,
  groupAllDone,
  sessionLabel,
  matchSession,
  readJSON,
  writeJSON,
  isCollapsed,
  togglePref,
} from './sidebar.js';

describe('buildTree', () => {
  it('nests Subagents under a parent in the list and keeps orphans as roots', () => {
    const list = [
      { id: 'p', harness: 'claude' },
      { id: 'c1', parentId: 'p' },
      { id: 'c2', parentId: 'p' },
      { id: 'o', parentId: 'gone' },
    ];
    const { roots, kids } = buildTree(list);
    expect(roots.map((s) => s.id)).toEqual(['p', 'o']);
    expect(kids.get('p').map((s) => s.id)).toEqual(['c1', 'c2']);
  });
});

describe('groupAllDone', () => {
  it('reads a group as done only when every child ended or failed', () => {
    expect(groupAllDone([])).toBe(false);
    expect(groupAllDone([{ status: 'ended' }, { status: 'failed' }])).toBe(true);
    expect(groupAllDone([{ status: 'ended' }, { status: 'idle' }])).toBe(false);
  });
});

describe('sessionLabel', () => {
  it('prefers the spawn name, then the title, then harness and short id', () => {
    expect(sessionLabel({ name: 'worker: add test', title: 'T' })).toBe('worker: add test');
    expect(sessionLabel({ title: 'Fix sidebar' })).toBe('Fix sidebar');
    expect(sessionLabel({ id: 'abcdef123456', harness: 'pig' })).toBe('pig · abcdef12');
  });
});

describe('matchSession', () => {
  it('matches the label, the harness or the project name, ignoring case', () => {
    const s = { title: 'Fix Sidebar', harness: 'claude' };
    expect(matchSession(s, '', 'sidebar')).toBe(true);
    expect(matchSession(s, '', 'CLAUDE')).toBe(true);
    expect(matchSession(s, 'emberloop', 'ember')).toBe(true);
    expect(matchSession(s, 'emberloop', 'nothing')).toBe(false);
    expect(matchSession(s, '', '  ')).toBe(true);
  });
});

describe('persisted collapse prefs', () => {
  beforeEach(() => localStorage.clear());

  it('round-trips JSON through localStorage and falls back on bad data', () => {
    writeJSON('eb.test', { a: true });
    expect(readJSON('eb.test', {})).toEqual({ a: true });
    localStorage.setItem('eb.test', '{not json');
    expect(readJSON('eb.test', 'fallback')).toBe('fallback');
  });

  it('lets an explicit override win over the default, and toggles from the effective state', () => {
    expect(isCollapsed({}, 'g', true)).toBe(true);
    expect(isCollapsed({ g: false }, 'g', true)).toBe(false);
    const next = togglePref({}, 'g', true);
    expect(next.g).toBe(false);
    expect(togglePref(next, 'g', true).g).toBe(true);
  });
});
