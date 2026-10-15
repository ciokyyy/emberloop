import { describe, it, expect } from 'vitest';
import { parsePath, buildPath, sameRoute } from './router.js';

describe('parsePath', () => {
  it('parses the home, settings, runs and global pages', () => {
    expect(parsePath('/')).toEqual({ name: 'home' });
    expect(parsePath('')).toEqual({ name: 'home' });
    expect(parsePath('/settings')).toEqual({ name: 'settings' });
    expect(parsePath('/quota')).toEqual({ name: 'quota' });
    expect(parsePath('/providers')).toEqual({ name: 'providers' });
    expect(parsePath('/providers#provider-google-antigravity')).toEqual({ name: 'providers' });
    expect(parsePath('/memory')).toEqual({ name: 'memory' });
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
    expect(parsePath('/providers/unknown').name).toBe('unknown');
  });
});

describe('Project-scoped routes', () => {
  it('parses every Project scope URL and keeps global pages unscoped', () => {
    expect(parsePath('/')).toEqual({ name: 'home' });
    expect(parsePath('/p/project-1')).toEqual({ name: 'project', projectId: 'project-1' });
    expect(parsePath('/p/project-1/runs')).toEqual({ name: 'project-runs', projectId: 'project-1' });
    expect(parsePath('/p/project-1/settings')).toEqual({ name: 'project-settings', projectId: 'project-1' });
    expect(parsePath('/p/none')).toEqual({ name: 'project', projectId: 'none' });
    expect(parsePath('/s/session-1')).toEqual({ name: 'session', id: 'session-1' });
    for (const path of ['/quota', '/providers', '/memory', '/settings']) {
      expect(parsePath(path).name).not.toBe('project');
      expect(parsePath(path).name).not.toBe('project-runs');
    }
  });

  it('round-trips scoped Run links and uses stable ids instead of renamed labels', () => {
    const routes = [
      { name: 'project', projectId: 'project-1' },
      { name: 'project', projectId: 'none' },
      { name: 'project-runs', projectId: 'project-1' },
      { name: 'project-settings', projectId: 'project-1' },
      { name: 'project-run', projectId: 'project-1', runId: 'run-1' },
      { name: 'project-ticket', projectId: 'project-1', runId: 'run-1', ticketId: 'ticket-1' },
    ];
    for (const route of routes) expect(parsePath(buildPath(route))).toEqual(route);

    const renamed = buildPath({ name: 'project', projectId: 'stable-id', projectName: 'New name' });
    expect(renamed).toBe('/p/stable-id');
    expect(parsePath(renamed).projectId).toBe('stable-id');
  });

  it('preserves scope when navigating between Project Runs routes', () => {
    expect(sameRoute(
      { name: 'project', projectId: 'one' },
      { name: 'project', projectId: 'two' },
    )).toBe(false);
    expect(sameRoute(
      { name: 'project-run', projectId: 'one', runId: 'r' },
      { name: 'project-run', projectId: 'two', runId: 'r' },
    )).toBe(false);
    const history = ['/p/one', '/p/one/runs', '/p/none', '/quota', '/'];
    expect(history.map((path) => parsePath(path).name)).toEqual([
      'project', 'project-runs', 'project', 'quota', 'home',
    ]);
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
      { name: 'providers' },
      { name: 'memory' },
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
    const entries = ['/s/a?at=4', '/runs/r1/t2', '/settings', '/providers', '/'];
    const names = entries.map((e) => parsePath(e).name);
    expect(names).toEqual(['session', 'ticket', 'settings', 'providers', 'home']);
    expect(parsePath(entries[0])).toMatchObject({ id: 'a', at: 4 });
  });
});
