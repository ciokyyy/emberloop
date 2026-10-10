import { describe, it, expect } from 'vitest';
import {
  ticketById,
  resolveBlocker,
  nodeStatus,
  statusLabel,
  nodeSubtitle,
  graphEdges,
  structureKey,
  layoutRunGraph,
  dedupeTickets,
  edgePath,
  countByStatus,
} from './runsGraph.js';

const tickets = [
  { id: '01-a', num: 1, title: 'A', state: 'merged', blockers: [] },
  { id: '02-b', num: 2, title: 'B', state: 'running', blockers: ['01-a'] },
  { id: '03-c', num: 3, title: 'C', state: 'queued', blockers: ['02-b'] },
  { id: '04-d', num: 4, title: 'D', state: 'needs-you', blockers: [], needsYou: 'same refusal ×3', lastRefusal: '' },
  { id: '05-e', num: 5, title: 'E', state: 'parked', blockers: [] },
];

describe('runsGraph state mapping', () => {
  it('maps driver states to graph tones', () => {
    const byId = ticketById(tickets);
    expect(nodeStatus(byId['01-a'], byId)).toBe('merged');
    expect(nodeStatus(byId['02-b'], byId)).toBe('running');
    expect(nodeStatus(byId['04-d'], byId)).toBe('needs-you');
    expect(nodeStatus(byId['05-e'], byId)).toBe('parked');
    expect(nodeStatus({ ...byId['05-e'], state: 'discarded' }, byId)).toBe('parked');
    for (const s of ['running', 'in-review', 'merging']) {
      expect(nodeStatus({ id: 'x', state: s, blockers: [] }, byId)).toBe('running');
    }
  });

  it('derives blocked from unmerged blockers, by id or number', () => {
    const byId = ticketById(tickets);
    // 03-c waits on running 02-b: blocked and dim.
    expect(nodeStatus(byId['03-c'], byId)).toBe('blocked');
    // A queued ticket behind only merged blockers is plain queued.
    expect(nodeStatus({ id: 'x', state: 'queued', blockers: ['01-a'] }, byId)).toBe('queued');
    // Number refs resolve too; unknown refs count as unmerged.
    expect(nodeStatus({ id: 'x', state: 'queued', blockers: ['2'] }, byId)).toBe('blocked');
    expect(nodeStatus({ id: 'x', state: 'queued', blockers: ['nope'] }, byId)).toBe('blocked');
    expect(nodeStatus({ id: 'x', state: 'queued', blockers: [] }, byId)).toBe('queued');
  });

  it('labels blocked cards with their blockers', () => {
    expect(statusLabel({ blockers: ['02-b'] }, 'blocked')).toBe('blocked by 02');
    expect(statusLabel({ state: 'parked' }, 'parked')).toBe('parked');
    expect(statusLabel({ state: 'merged' }, 'merged')).toBe('merged');
  });

  it('subtitles stalled cards with the reason, others with verdicts', () => {
    const byId = ticketById(tickets);
    expect(nodeSubtitle(byId['04-d'])).toBe('same refusal ×3');
    expect(
      nodeSubtitle({
        verdicts: {
          'reviewer-spec': { verdict: 'pass', fresh: true },
          'reviewer-standards': { verdict: 'changes', fresh: false },
        },
      }),
    ).toBe('spec pass · standards changes (stale)');
    expect(nodeSubtitle({})).toBe('');
  });

  it('resolves blockers by id or number', () => {
    const byId = ticketById(tickets);
    expect(resolveBlocker(byId, '01-a')?.id).toBe('01-a');
    expect(resolveBlocker(byId, '2')?.id).toBe('02-b');
    expect(resolveBlocker(byId, 'nope')).toBeNull();
  });
});

describe('runsGraph edges and layout', () => {
  it('links blockers to tickets, skipping unknown refs, dupes and self loops', () => {
    expect(graphEdges(tickets)).toEqual([
      { from: '01-a', to: '02-b' },
      { from: '02-b', to: '03-c' },
    ]);
    const dupes = [
      { id: 'a', state: 'queued', blockers: ['b', 'b', 'a', 'ghost'] },
      { id: 'b', state: 'merged', blockers: [] },
    ];
    expect(graphEdges(dupes)).toEqual([{ from: 'b', to: 'a' }]);
  });

  it('keys layout identity on ids plus blockers only', () => {
    const reordered = [...tickets].reverse();
    expect(structureKey(reordered)).toBe(structureKey(tickets));
    const restated = tickets.map((t) => ({ ...t, state: 'merged' }));
    expect(structureKey(restated)).toBe(structureKey(tickets));
    const relinked = tickets.map((t) => (t.id === '03-c' ? { ...t, blockers: [] } : t));
    expect(structureKey(relinked)).not.toBe(structureKey(tickets));
  });

  it('lays chains out left to right with one edge per blocker', () => {
    const g = layoutRunGraph(tickets);
    expect(g.nodes).toHaveLength(5);
    expect(g.edges).toHaveLength(2);
    expect(g.width).toBeGreaterThan(0);
    expect(g.height).toBeGreaterThan(0);
    const at = (id) => g.nodes.find((n) => n.id === id);
    expect(at('01-a').cx).toBeLessThan(at('02-b').cx);
    expect(at('02-b').cx).toBeLessThan(at('03-c').cx);
    expect(at('03-c').status).toBe('blocked');
    expect(at('03-c').label).toBe('blocked by 02');
    for (const e of g.edges) {
      expect(e.points.length).toBeGreaterThan(1);
      expect(edgePath(e.points)).toMatch(/^M.*C /);
    }
  });

  it('handles an empty run', () => {
    expect(layoutRunGraph([])).toEqual({ nodes: [], edges: [], width: 0, height: 0 });
    expect(countByStatus([])).toEqual({});
  });

  it('builds edge paths', () => {
    expect(edgePath([])).toBe('');
    expect(edgePath([{ x: 1, y: 2 }])).toBe('M1 2');
    expect(edgePath([{ x: 0, y: 0 }, { x: 10, y: 0 }])).toMatch(/^M0 0 C /);
  });

  it('counts cards per tone for the legend', () => {
    const g = layoutRunGraph(tickets);
    expect(countByStatus(g.nodes)).toEqual({ merged: 1, running: 1, blocked: 1, 'needs-you': 1, parked: 1 });
  });

  it('lays a cyclic graph out without throwing', () => {
    const cyclic = [
      { id: 'a', num: 1, title: 'A', state: 'running', blockers: ['b'] },
      { id: 'b', num: 2, title: 'B', state: 'queued', blockers: ['a'] },
    ];
    const g = layoutRunGraph(cyclic);
    expect(g.nodes).toHaveLength(2);
    expect(g.edges).toHaveLength(2);
    for (const n of g.nodes) {
      expect(Number.isFinite(n.x)).toBe(true);
      expect(Number.isFinite(n.y)).toBe(true);
    }
  });

  it('dedupes duplicate ticket ids to one node', () => {
    const dupes = [
      { id: 'a', num: 1, title: 'First', state: 'running', blockers: [] },
      { id: 'a', num: 1, title: 'Second', state: 'queued', blockers: [] },
    ];
    expect(dedupeTickets(dupes)).toHaveLength(1);
    expect(dedupeTickets(dupes)[0].title).toBe('First');
    const g = layoutRunGraph(dupes);
    expect(g.nodes).toHaveLength(1);
    expect(g.nodes[0].title).toBe('First');
  });
});
