import { describe, it, expect } from 'vitest';
import {
  STATE_STEPS,
  shortReviewerType,
  shortTip,
  findTicket,
  roundNumber,
  currentRound,
  stateTrack,
  reviewCards,
  gateSummary,
  boxProgress,
  agentsOf,
  agentRoleLabel,
  sinceLabel,
  dropPendingEcho,
  ticketFinished,
  runFinished,
} from './ticketPage.js';

const round1 = {
  n: 1,
  tip: 'bad73e8aaa01',
  reviews: {
    'reviewer-standards': {
      verdict: 'changes',
      findings: [{ file: 'events.js', line: 78, severity: 'major', problem: 'tool blocks all open' }],
      at: 'bad73e8aaa01',
      durationMs: 68000,
    },
    'reviewer-spec': { verdict: 'pass', findings: [], at: 'bad73e8aaa01', durationMs: 65000 },
  },
  declined: ['Full shadcn registry port: same tokens.'],
  sentToWorkerAt: '2026-10-09T10:01:00Z',
};

const round2 = {
  n: 2,
  tip: '4444333fff02',
  reviews: {
    'reviewer-spec': { verdict: 'pass', findings: [], heldFromRound: 1, at: 'bad73e8aaa01' },
  },
};

const ticket = (over = {}) => ({
  id: '07-ui-design',
  num: 7,
  title: 'UI design',
  state: 'in-review',
  rounds: [round1, round2],
  ...over,
});

describe('ticketPage state mapping', () => {
  it('maps every track state to its current step', () => {
    const labels = { queued: 'queued', running: 'running', merging: 'merging', merged: 'merged' };
    for (const [state, label] of Object.entries(labels)) {
      const { steps, alert, currentLabel } = stateTrack(ticket({ state, rounds: [] }));
      const idx = STATE_STEPS.findIndex((s) => s.id.replace('-', '-') === state || s.id === state);
      expect(steps[idx].state).toBe(state === 'merged' ? 'done' : 'current');
      expect(alert).toBeNull();
      expect(currentLabel).toBe(label);
    }
    // Merged marks every step done.
    expect(stateTrack(ticket({ state: 'merged' })).steps.every((s) => s.state === 'done')).toBe(true);
  });

  it('labels in-review with its round number', () => {
    expect(stateTrack(ticket()).currentLabel).toBe('in review · round 2');
    expect(stateTrack(ticket({ rounds: [] })).currentLabel).toBe('in review');
    const { steps } = stateTrack(ticket());
    expect(steps.find((s) => s.id === 'in-review').state).toBe('current');
    expect(steps.find((s) => s.id === 'queued').state).toBe('done');
    expect(steps.find((s) => s.id === 'merging').state).toBe('todo');
  });

  it('surfaces off-track states as alerts', () => {
    const needs = stateTrack(ticket({ state: 'needs-you' }));
    expect(needs.alert).toEqual({ kind: 'needs-you', label: 'needs you' });
    expect(needs.steps.find((s) => s.id === 'in-review').state).toBe('current');
    for (const state of ['parked', 'discarded', 'paused']) {
      const t = stateTrack(ticket({ state }));
      expect(t.alert).toEqual({ kind: state, label: state });
      expect(t.steps.every((s) => s.state === 'todo')).toBe(true);
    }
  });
});

describe('ticketPage rounds rendering', () => {
  it('renders two cards per round in gate order', () => {
    const cards = reviewCards(round1);
    expect(cards.map((c) => c.short)).toEqual(['standards', 'spec']);
    expect(cards[0].verdict).toBe('changes');
    expect(cards[0].findings).toHaveLength(1);
    expect(cards[1].verdict).toBe('pass');
  });

  it('marks a carried pass as held and a missing verdict as pending', () => {
    const cards = reviewCards(round2);
    expect(cards[0].verdict).toBe('pending');
    expect(cards[0].held).toBe(false);
    expect(cards[1]).toMatchObject({ verdict: 'pass', held: true, heldFromRound: 1 });
  });

  it('finds tickets and round counts', () => {
    const run = { tickets: [ticket(), ticket({ id: 'other' })] };
    expect(findTicket(run, '07-ui-design').num).toBe(7);
    expect(findTicket(run, 'missing')).toBeNull();
    expect(roundNumber(ticket())).toBe(2);
    expect(currentRound(ticket()).n).toBe(2);
    expect(currentRound(ticket({ rounds: [] }))).toBeNull();
  });
});

describe('ticketPage panel helpers', () => {
  const gateTicket = ticket({
    tip: '4444333fff02',
    gate: {
      required: ['reviewer-standards', 'reviewer-spec'],
      verdicts: {
        'reviewer-spec': { verdict: 'pass', at: '4444333fff02', fresh: true },
        'reviewer-standards': { verdict: 'changes', at: 'bad73e8aaa01', fresh: false },
      },
    },
  });

  it('summarizes the gate with freshness', () => {
    expect(gateSummary(gateTicket)).toEqual([
      { type: 'reviewer-standards', short: 'standards', verdict: 'changes', fresh: false, at: 'bad73e8aaa01' },
      { type: 'reviewer-spec', short: 'spec', verdict: 'pass', fresh: true, at: '4444333fff02' },
    ]);
    const missing = gateSummary(ticket({ gate: { required: ['reviewer-spec'], verdicts: {} } }));
    expect(missing).toEqual([{ type: 'reviewer-spec', short: 'spec', verdict: 'pending', fresh: false, at: '' }]);
  });

  it('counts acceptance boxes', () => {
    expect(boxProgress([{ text: 'a', checked: true }, { text: 'b', checked: false }])).toEqual({ done: 1, total: 2 });
    expect(boxProgress([])).toEqual({ done: 0, total: 0 });
  });

  it('lists agents with a session-id fallback', () => {
    const agents = [
      { sessionId: 'sess-1', role: 'worker', harness: 'claude', status: 'working', name: '07-ui-design' },
      { sessionId: 'sess-2', role: 'reviewer-spec', harness: 'claude', status: 'idle', name: '' },
    ];
    expect(agentsOf({ agents })).toBe(agents);
    expect(agentsOf({ workerSession: 'sess-1', reviewerSessions: { 'reviewer-spec': 'sess-2' } })).toEqual([
      { sessionId: 'sess-1', role: 'worker', harness: '', status: '', name: '' },
      { sessionId: 'sess-2', role: 'reviewer-spec', harness: '', status: '', name: '' },
    ]);
    expect(agentRoleLabel('reviewer-standards')).toBe('reviewer');
    expect(agentRoleLabel('scout')).toBe('scout');
    expect(agentRoleLabel('worker')).toBe('worker');
  });

  it('shortens reviewer types, tips and elapsed labels', () => {
    expect(shortReviewerType('reviewer-spec')).toBe('spec');
    expect(shortReviewerType('reviewer-standards')).toBe('standards');
    expect(shortTip('4444333fff02')).toBe('4444333');
    expect(shortTip('')).toBe('—');
    const now = new Date('2026-10-09T12:00:00Z').getTime();
    expect(sinceLabel('2026-10-09T11:49:00Z', now)).toBe('11m');
    expect(sinceLabel('2026-10-09T11:59:30Z', now)).toBe('30s');
    expect(sinceLabel('not-a-date', now)).toBe('');
    expect(sinceLabel('2026-10-09T09:00:00Z', now)).toBe('3h');
  });
});

describe('dropPendingEcho', () => {
  const pend = (over = {}) => ({ key: 1, text: 'steer the worker', mode: 'steer', images: 0, imageIds: [], state: 'sent', ...over });

  it('drops exactly one pending per exact echo', () => {
    const list = [pend({ key: 1 }), pend({ key: 2 })];
    const once = dropPendingEcho(list, { text: 'steer the worker', mode: 'steer' });
    expect(once).toHaveLength(1);
    expect(once[0].key).toBe(2);
    expect(dropPendingEcho(once, { text: 'steer the worker', mode: 'steer' })).toHaveLength(0);
  });

  it('ignores partial overlaps instead of clearing the queue', () => {
    const list = [pend({ key: 1, text: 'fix the parser' }), pend({ key: 2, text: 'parser' })];
    // The echo carries the merged text of queued messages; a pending whose
    // text is only contained in it is not confirmed.
    expect(dropPendingEcho(list, { text: 'fix the parser plus docs' })).toHaveLength(2);
    expect(dropPendingEcho(list, { text: 'fix the parse' })).toHaveLength(2);
  });

  it('matches image-only pendings instead of leaving ghosts', () => {
    const list = [pend({ key: 9, text: '', images: 2, imageIds: [] })];
    expect(dropPendingEcho(list, { text: '', images: 2 })).toHaveLength(0);
    // A text echo does not confirm an image-only pending.
    expect(dropPendingEcho(list, { text: 'look', images: 0 })).toHaveLength(1);
  });

  it('keeps pendings with different attachments or modes', () => {
    const list = [pend({ key: 1, images: 1, imageIds: ['img-a'] })];
    expect(dropPendingEcho(list, { text: 'steer the worker', images: 0 })).toHaveLength(1);
    expect(dropPendingEcho(list, { text: 'steer the worker', images: 1, imageIds: ['img-b'] })).toHaveLength(1);
    expect(dropPendingEcho(list, { text: 'steer the worker', mode: 'followup', images: 1, imageIds: ['img-a'] })).toHaveLength(1);
  });
});

describe('finished helpers', () => {
  it('closes the stream only for terminal states', () => {
    expect(ticketFinished({ state: 'merged' })).toBe(true);
    expect(ticketFinished({ state: 'discarded' })).toBe(true);
    expect(ticketFinished({ state: 'in-review' })).toBe(false);
    expect(ticketFinished({ state: 'needs-you' })).toBe(false);
    expect(ticketFinished(null)).toBe(false);
    expect(runFinished({ status: 'done' })).toBe(true);
    expect(runFinished({ status: 'failed' })).toBe(true);
    expect(runFinished({ status: 'running' })).toBe(false);
    expect(runFinished({ status: 'needs-you' })).toBe(false);
    expect(runFinished({ status: 'abandoned' })).toBe(true);
    expect(runFinished({ status: 'paused' })).toBe(false);
  });
});
