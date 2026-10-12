import { describe, it, expect } from 'vitest';
import { buildOwnerGroups, dotState, ownerTone, projectIdForPath, runRow, OWNER_TONES } from './owners.js';

const users = [
  { id: 'u-cioky', username: 'cioky', admin: true, online: true },
  { id: 'u-mira', username: 'mira', admin: false, online: false },
];
const me = { id: 'u-cioky', username: 'cioky', admin: true };
const projects = [
  { id: 'p-emb', name: 'emberloop', path: '/src/emberloop' },
  { id: 'p-dot', name: 'dotfiles', path: '/src/dotfiles/' },
];
const S = (id, owner, extra = {}) => ({ id, owner, harness: 'claude', status: 'idle', title: `t-${id}`, ...extra });
const sessions = [
  S('a1', 'u-cioky', { projectId: 'p-emb', name: 'lead' }),
  S('a2', 'u-cioky', { projectId: 'p-emb', parentId: 'a1', name: 'worker: add test' }),
  S('a3', 'u-cioky'),
  S('m1', 'u-mira', { projectId: 'p-dot', status: 'failed' }),
  S('m2', 'u-mira', { projectId: 'p-emb' }),
  S('x1', ''),
];
const runs = [{ id: 'r1', owner: 'u-cioky', title: 'Web run', repo: '/src/emberloop', status: 'running', tickets: [] }];
const worker = S('w1', 'u-cioky', { parentId: 'r1', runWorker: true, projectId: 'p-emb' });

const build = (over = {}) => buildOwnerGroups({ users, me, projects, sessions, runs: [], filter: 'all', ...over });
const byName = (out, name) => out.groups.find((g) => g.name === name);
const keys = (group) => group.sections.map((s) => s.key);

describe('ownerTone', () => {
  it('gives each User its palette slot by account order, wrapping, and mutes unknown Users', () => {
    expect(ownerTone(users, 'u-cioky')).toBe('var(--owner-1)');
    expect(ownerTone(users, 'u-mira')).toBe('var(--owner-2)');
    const many = Array.from({ length: OWNER_TONES + 1 }, (_, i) => ({ id: `u${i}` }));
    expect(ownerTone(many, `u${OWNER_TONES}`)).toBe('var(--owner-1)');
    expect(ownerTone(users, 'nobody')).toBe('var(--muted-foreground)');
  });
});

describe('Run rows', () => {
  it('match a repo path to its Project, ignoring a trailing slash', () => {
    expect(projectIdForPath(projects, '/src/dotfiles')).toBe('p-dot');
    expect(projectIdForPath(projects, '/elsewhere')).toBe('');
    expect(projectIdForPath(projects, '')).toBe('');
  });

  it('turn a Run into a Run-kind row, needs-you when any ticket asks', () => {
    const row = runRow(
      { id: 'r1', spec: 'specs/x/spec.md', owner: 'u-cioky', repo: '/src/emberloop', status: 'running', tickets: [{ needsYou: '' }, { needsYou: 'conflict' }] },
      projects,
    );
    expect(row).toMatchObject({ id: 'r1', kind: 'run', title: 'specs/x/spec.md', projectId: 'p-emb', needsYou: true });
  });
});

describe('dotState', () => {
  it('reads needs-you first, then capped and the Session or Run status', () => {
    expect(dotState({ status: 'failed' })).toBe('failed');
    expect(dotState({ status: 'failed', capped: true })).toBe('capped');
    expect(dotState({ status: 'failed', capped: true, needsYou: true })).toBe('needs');
    expect(dotState({ status: 'needs-you', capped: true })).toBe('needs');
    expect(dotState({ status: 'working' })).toBe('working');
    expect(dotState({ status: 'ended' })).toBe('ended');
    expect(dotState({ status: 'idle' })).toBe('idle');
    expect(dotState({ kind: 'run', status: 'running' })).toBe('working');
    expect(dotState({ kind: 'run', status: 'done' })).toBe('ended');
    expect(dotState({ kind: 'run', status: 'paused' })).toBe('idle');
  });
});

describe('buildOwnerGroups', () => {
  it('lists the own group first, then the others by name, and never an ownerless row', () => {
    const out = build();
    expect(out.groups.map((g) => g.name)).toEqual(['cioky', 'mira']);
    expect(out.groups[0].own).toBe(true);
    expect(out.groups.flatMap((g) => g.sections.flatMap((s) => s.roots.map((r) => r.id)))).not.toContain('x1');
  });

  it('counts root rows per User and nests Subagents under their parent', () => {
    const out = build();
    expect(byName(out, 'cioky').count).toBe(2);
    expect(byName(out, 'mira').count).toBe(2);
    const emb = byName(out, 'cioky').sections.find((s) => s.key === 'p-emb');
    expect(emb.roots.map((r) => r.id)).toEqual(['a1']);
    expect(out.kids.get('a1').map((r) => r.id)).toEqual(['a2']);
  });

  it('nests a Run Worker under its Run row', () => {
    const out = build({ sessions: [...sessions, worker], runs });
    const emb = byName(out, 'cioky').sections.find((s) => s.key === 'p-emb');
    expect(emb.roots.map((r) => r.id)).toEqual(['r1', 'a1']);
    expect(emb.roots[0]).toMatchObject({ kind: 'run' });
    expect(out.kids.get('r1').map((r) => r.id)).toEqual(['w1']);
  });

  it('keeps the own group on every Project in Project order, Other last', () => {
    const out = build();
    expect(keys(byName(out, 'cioky'))).toEqual(['p-emb', 'p-dot', '']);
    expect(keys(byName(out, 'mira'))).toEqual(['p-emb', 'p-dot']);
  });

  it('shows only the own group on Mine and every group on Everyone', () => {
    expect(build({ filter: 'mine' }).groups.map((g) => g.name)).toEqual(['cioky']);
    expect(build({ filter: 'all' }).groups.map((g) => g.name)).toEqual(['cioky', 'mira']);
  });

  it('keeps only the chosen Project on the Project filter', () => {
    const out = build({ filter: 'project', projectId: 'p-dot' });
    expect(byName(out, 'mira').sections.flatMap((s) => s.roots.map((r) => r.id))).toEqual(['m1']);
    expect(byName(out, 'cioky').sections.flatMap((s) => s.roots.map((r) => r.id))).toEqual([]);
    expect(keys(byName(out, 'cioky'))).toEqual(['p-dot']);
  });

  it('matches the chat query on a Subagent label and drops groups and sections with no match', () => {
    const out = build({ query: 'worker' });
    expect(out.groups.map((g) => g.name)).toEqual(['cioky']);
    expect(keys(out.groups[0])).toEqual(['p-emb']);
    expect(out.groups[0].sections[0].roots.map((r) => r.id)).toEqual(['a1']);
  });

  it('lists a row owned by a User missing from the list under a fallback group', () => {
    const out = build({ sessions: [S('g1', 'u-ghost-account-id')] });
    const ghost = out.groups.find((g) => g.id === 'u-ghost-account-id');
    expect(ghost).toMatchObject({ name: 'u-ghost-', online: false, own: false });
  });

  it('shows the signed-in User even before the list loads', () => {
    const out = buildOwnerGroups({ users: [], me, projects, sessions: [S('a9', 'u-cioky')] });
    expect(out.groups.map((g) => g.name)).toEqual(['cioky']);
    expect(out.groups[0]).toMatchObject({ own: true, online: true });
  });
});
