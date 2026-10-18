import { describe, it, expect } from 'vitest';
import { fallbackName, displayName, parseSpawned, parseFinished, latestTool, childState, groupCounts, reportCountsText } from './subagents.js';

describe('fallbackName', () => {
  it('takes the first four words of the prompt', () => {
    expect(fallbackName('run the full test suite now')).toBe('run the full test');
  });
  it('falls back to a label, never an empty string', () => {
    expect(fallbackName('')).toBe('Subagent');
  });
  it('prefers an explicit name', () => {
    expect(displayName({ name: 'Alpha', prompt: 'job A' })).toBe('Alpha');
  });
});

describe('spawned payloads', () => {
  it('parses the JSON payload', () => {
    expect(parseSpawned(JSON.stringify({ id: 'a', group: 'g', name: 'N' }))).toEqual({ id: 'a', group: 'g', name: 'N' });
  });
  it('accepts legacy bare ids', () => {
    expect(parseSpawned('abc123')).toEqual({ id: 'abc123', group: '', name: '' });
  });
  it('parses group and per-child finished markers', () => {
    expect(parseFinished('g1')).toEqual({ group: 'g1', id: '' });
    expect(parseFinished(JSON.stringify({ id: 'a', group: 'g' }))).toEqual({ group: 'g', id: 'a' });
  });
});

describe('latestTool', () => {
  it('summarizes the newest tool call with its file', () => {
    const evs = [
      { kind: 'tool_call_started', text: 'Read {"path":"a.js"}' },
      { kind: 'tool_call_started', text: 'Edit stt.js' },
    ];
    expect(latestTool(evs)).toBe('Edit stt.js');
  });
  it('reads the JSON tool payload, not the raw text', () => {
    const evs = [{ kind: 'tool_call_started', text: JSON.stringify({ id: 'call_1', name: 'read', input: '{"path":"src/a.js"}' }) }];
    expect(latestTool(evs)).toBe('read src/a.js');
  });
  it('is empty with no tool calls', () => {
    expect(latestTool([{ kind: 'assistant_chunk', text: 'hi' }])).toBe('');
  });
});

describe('childState', () => {
  it('treats an idle child with no reply as still running', () => {
    expect(childState('idle', [{ kind: 'prompt', text: 'go' }])).toBe('running');
    expect(childState('idle', [{ kind: 'prompt', text: 'go' }, { kind: 'assistant_chunk', text: 'done' }])).toBe('done');
    expect(childState('working', [])).toBe('running');
    expect(childState('failed', [])).toBe('failed');
  });
});

describe('groupCounts', () => {
  it('tallies done, running and failed', () => {
    expect(groupCounts(['done', 'running', 'failed', 'done'])).toEqual({ total: 4, done: 2, failed: 1, running: 1, allDone: false, anyFailed: true });
    expect(groupCounts(['done', 'done']).allDone).toBe(true);
  });
});

describe('reportCountsText', () => {
  it('leads with the total and adds only the non-zero breakdown', () => {
    expect(reportCountsText({ counts: { total: 3, workers: 2, scouts: 1, failed: 0 } })).toBe('3 agents · 2 workers · 1 scout');
    expect(reportCountsText({ counts: { total: 1, workers: 1, scouts: 0, failed: 0 } })).toBe('1 agent · 1 worker');
    expect(reportCountsText({ counts: { total: 2, workers: 1, failed: 1 } })).toBe('2 agents · 1 worker · 1 failed');
  });
  it('falls back to the member list and a single agent', () => {
    expect(reportCountsText({ members: [{ kind: 'worker' }, { kind: 'worker' }] })).toBe('2 agents');
    expect(reportCountsText({})).toBe('0 agents');
  });
});
