import { describe, it, expect } from 'vitest';
import {
  normalizeQueueItem,
  parseProposal,
  normalizeRule,
  normalizeObservation,
  normalizeLedger,
  groupQueueBySession,
  purgeCounts,
  purgeSummary,
  latencyLabel,
} from './memory.js';

describe('normalizeQueueItem', () => {
  it('accepts lowercase and Go-case keys', () => {
    expect(normalizeQueueItem({ id: 7, kind: 'rule', payload: '{}', status: 'pending' })).toMatchObject({
      id: 7,
      kind: 'rule',
      status: 'pending',
    });
    expect(normalizeQueueItem({ ID: 8, Kind: 'rule', Payload: '{}', Status: 'approved' })).toMatchObject({
      id: 8,
      kind: 'rule',
      status: 'approved',
    });
  });
});

describe('parseProposal', () => {
  it('reads the new shape with quotes and sessions', () => {
    const p = parseProposal(JSON.stringify({ text: 'use tabs', quotes: ['q1', 'q2'], sessions: ['s1', 's2'] }));
    expect(p.text).toBe('use tabs');
    expect(p.quotes).toEqual(['q1', 'q2']);
    expect(p.sessions).toEqual(['s1', 's2']);
  });

  it('upgrades the old remember shape', () => {
    const p = parseProposal(JSON.stringify({ text: 'use tabs', quote: 'q1', sessionId: 's1' }));
    expect(p.quotes).toEqual(['q1']);
    expect(p.sessions).toEqual(['s1']);
  });

  it('never throws on bad JSON', () => {
    expect(parseProposal('nope{').text).toBe('');
  });
});

describe('normalize rows', () => {
  it('folds rules, observations and ledger rows', () => {
    expect(normalizeRule({ ruleId: 'r1', text: 't' }).ruleId).toBe('r1');
    expect(normalizeRule({ RuleID: 'r2', Text: 'u' }).ruleId).toBe('r2');
    expect(normalizeObservation({ id: 3, quote: 'q' }).id).toBe(3);
    expect(normalizeObservation({ ID: 4, Quote: 'w' }).id).toBe(4);
    expect(normalizeLedger({ id: 5, stage: 'mine' }).stage).toBe('mine');
    expect(normalizeLedger({ ID: 6, Stage: 'queue_approve' }).stage).toBe('queue_approve');
  });
});

describe('groupQueueBySession', () => {
  it('buckets pending rules by first session', () => {
    const items = [
      { id: 1, kind: 'rule', status: 'pending', payload: JSON.stringify({ text: 'a', sessions: ['s1'] }) },
      { id: 2, kind: 'rule', status: 'pending', payload: JSON.stringify({ text: 'b', sessions: ['s1'] }) },
      { id: 3, kind: 'rule', status: 'approved', payload: JSON.stringify({ text: 'c', sessions: ['s1'] }) },
      { id: 4, kind: 'profile_patch', status: 'pending', payload: '{}' },
    ];
    const groups = groupQueueBySession(items);
    expect(groups).toHaveLength(1);
    expect(groups[0].sessionId).toBe('s1');
    expect(groups[0].rows).toHaveLength(2);
  });
});

describe('purge helpers', () => {
  it('counts the selection', () => {
    expect(purgeCounts({ observations: [1], rules: ['a'], queue: [], ledger: [2, 3] })).toMatchObject({
      observations: 1,
      rules: 1,
      queue: 0,
      ledger: 2,
      total: 4,
    });
  });

  it('summarizes what a purge removed', () => {
    expect(purgeSummary({ rules: 1, observations: 2, queue: 0, ledger: 0 })).toBe('Removed 1 rule, 2 observations.');
    expect(purgeSummary({})).toBe('Nothing removed.');
  });
});

describe('latencyLabel', () => {
  it('renders hours or a dash', () => {
    expect(latencyLabel(1)).toBe('1h');
    expect(latencyLabel(0)).toBe('—');
    expect(latencyLabel(null)).toBe('—');
  });
});
