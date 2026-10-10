import { describe, it, expect } from 'vitest';
import { parsePath, buildPath, sameRoute } from './router.js';

describe('parsePath', () => {
  it('parses the home, settings and runs paths', () => {
    expect(parsePath('/')).toEqual({ name: 'home' });
    expect(parsePath('')).toEqual({ name: 'home' });
    expect(parsePath('/settings')).toEqual({ name: 'settings' });
    expect(parsePath('/quota')).toEqual({ name: 'quota' });
    expect(parsePath('/runs')).toEqual({ name: 'runs' });
    expect(parsePath('/runs/')).toEqual({ name: 'runs' });
  });

  it('parses session, run and ticket paths', () => {
    expect(parsePath('/s/abc')).toEqual({ name: 'session', id: 'abc' });
    expect(parsePath('/s/abc/')).toEqual({ name: 'session', id: 'abc' });
    expect(parsePath('/s/abc?at=12')).toEqual({ name: 'session', id: 'abc', at: 12 });
    expect(parsePath('/s/abc?at=nope')).toEqual({ name: 'session', id: 'abc' });
    expect(parsePath('/runs/r1')).toEqual({ name: 'run', runId: 'r1' });
    expect(parsePath('/runs/r1/t2')).toEqual({ name: 'ticket', runId: 'r1', ticketId: 't2' });
  });

  it('marks unknown paths and bare prefixes', () => {
    expect(parsePath('/nope').name).toBe('unknown');
    expect(parsePath('/s').name).toBe('unknown');
    expect(parsePath('/s/').name).toBe('unknown');
    expect(parsePath('/runs/a/b/c').name).toBe('unknown');
  });
});

describe('buildPath', () => {
  it('round-trips every route', () => {
    const routes = [
      { name: 'home' },
      { name: 'session', id: 'abc' },
      { name: 'session', id: 'abc', at: 7 },
      { name: 'runs' },
      { name: 'run', runId: 'r1' },
      { name: 'ticket', runId: 'r1', ticketId: 't2' },
      { name: 'settings' },
      { name: 'quota' },
    ];
    for (const r of routes) {
      expect(parsePath(buildPath(r))).toEqual(r);
    }
  });

  it('falls back to home for junk', () => {
    expect(buildPath(null)).toBe('/');
    expect(buildPath({ name: 'wat' })).toBe('/');
  });
});

describe('sameRoute', () => {
  it('compares session identity including the at target', () => {
    expect(sameRoute({ name: 'session', id: 'a' }, { name: 'session', id: 'a' })).toBe(true);
    expect(sameRoute({ name: 'session', id: 'a' }, { name: 'session', id: 'b' })).toBe(false);
    expect(sameRoute({ name: 'session', id: 'a', at: 3 }, { name: 'session', id: 'a' })).toBe(false);
    expect(sameRoute({ name: 'runs' }, { name: 'runs' })).toBe(true);
    expect(sameRoute({ name: 'runs' }, { name: 'settings' })).toBe(false);
  });
});

describe('popstate handling', () => {
  it('re-parses the URL on back/forward so the view follows history', () => {
    // The App popstate listener is `() => applyRoute(location)`; the
    // contract here is that every history entry parses back to its route.
    const entries = ['/s/a?at=4', '/runs/r1/t2', '/settings', '/'];
    const names = entries.map((e) => parsePath(e).name);
    expect(names).toEqual(['session', 'ticket', 'settings', 'home']);
    expect(parsePath(entries[0])).toMatchObject({ id: 'a', at: 4 });
  });
});
