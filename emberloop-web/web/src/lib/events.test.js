import { describe, it, expect } from 'vitest';
import { cardForEvent, groupTranscript, compressBlockReason } from './events.js';
const ev = (seq, kind, text = '', status = '', mode = '') => ({ seq, kind, text, status, mode });

describe('cardForEvent maps every Event kind to a card', () => {
  const cases = [
    ['prompt', 'user'],
    ['assistant_chunk', 'assistant'],
    ['thinking_chunk', 'thinking'],
    ['tool_call_started', 'tool'],
    ['tool_call_updated', 'tool'],
    ['tool_call_finished', 'tool'],
    ['permission', 'permission'],
    ['status', 'status'],
    ['subagent_spawned', 'subagent'],
    ['subagent_finished', 'subagent'],
    ['error', 'error'],
    ['injection', 'divider'],
  ];
  for (const [kind, card] of cases) {
    it(`${kind} renders as ${card} card`, () => {
      expect(cardForEvent(ev(1, kind, 'x')).card).toBe(card);
    });
  }
  it('unknown future kind renders as generic card, never nothing', () => {
    expect(cardForEvent(ev(1, 'quantum_leap', 'x')).card).toBe('unknown');
  });
  it('tool card is collapsed by default', () => {
    expect(cardForEvent(ev(1, 'tool_call_started', 'read\nfoo')).collapsed).toBe(true);
  });
  it('thinking card is collapsed by default', () => {
    expect(cardForEvent(ev(1, 'thinking_chunk', 'hmm')).collapsed).toBe(true);
  });
  it('diff card is expanded by default', () => {
    expect(cardForEvent(ev(1, 'diff', '--- a\n+++ b')).collapsed).toBe(false);
  });
  it('labels steer prompts as Steer', () => {
    const c = cardForEvent(ev(1, 'prompt', 'go left', '', 'steer'));
    expect(c.card).toBe('user');
    expect(c.title).toBe('Steer');
  });
  it('labels follow-up prompts as Follow-up', () => {
    const c = cardForEvent(ev(1, 'prompt', 'after that', '', 'followup'));
    expect(c.card).toBe('user');
    expect(c.title).toBe('Follow-up');
  });
  it('renders a Handoff prompt as a collapsed block, not a user message', () => {
    const c = cardForEvent({ seq: 1, kind: 'prompt', text: '# Handoff\nbody', handoff: true, sender: 'system' });
    expect(c.card).toBe('report');
    expect(c.title).toBe('Handoff');
    expect(c.collapsed).toBe(true);
  });
  it('renders a compressed Event as a divider', () => {
    const c = cardForEvent({ seq: 1, kind: 'compressed', text: '{"number":2,"tokensBefore":5000,"model":"none","fallback":"deterministic"}' });
    expect(c.card).toBe('divider');
    expect(c.body).toBe('#2 · 5000 tokens before · deterministic');
    const m = cardForEvent({ seq: 2, kind: 'compressed', text: '{"number":3,"model":"gpt","fallback":"summary"}' });
    expect(m.body).toBe('#3 · gpt · summary');
    expect(c.body).toContain('#2');
    expect(c.body).toContain('5000 tokens before');
    expect(c.body).toContain('deterministic');
  });
  it('names retries and the summarizer error in the divider', () => {
    const c = cardForEvent({ seq: 4, kind: 'compressed', text: '{"number":4,"tokensBefore":900,"model":"gpt","fallback":"deterministic","retries":2,"error":"credit exhausted"}' });
    expect(c.body).toBe('#4 · 900 tokens before · gpt · deterministic · 2 retries · error: credit exhausted');
    const r = cardForEvent({ seq: 5, kind: 'compressed', text: '{"number":5,"model":"gpt","fallback":"repair","retries":1}' });
    expect(r.body).toBe('#5 · gpt · repair · 1 retry');
  });
  it('labels plain prompts as You', () => {
    expect(cardForEvent(ev(1, 'prompt', 'hi')).title).toBe('You');
  });
  it('renders an injection Event as a Memory injected divider, never raw JSON', () => {
    const c = cardForEvent(ev(1, 'injection', JSON.stringify({ profileVersion: 4, rules: [{ id: 'r1', version: 1 }, { id: 'r2', version: 3 }] })));
    expect(c.card).toBe('divider');
    expect(c.title).toBe('Memory injected');
    expect(c.body).toBe('profile v4 · 2 rules');
    const one = cardForEvent(ev(2, 'injection', JSON.stringify({ rules: [{ id: 'r', version: 2 }] })));
    expect(one.body).toBe('1 rule');
  });
  it('carries the injected Rule IDs on the divider for the chat promote action', () => {
    const c = cardForEvent(ev(1, 'injection', JSON.stringify({ rules: [{ id: 'r1', version: 1 }, { id: 'r2', version: 3 }] })));
    expect(c.rules).toEqual([{ id: 'r1', version: 1 }, { id: 'r2', version: 3 }]);
    const blocks = groupTranscript([ev(1, 'injection', JSON.stringify({ rules: [{ id: 'r1', version: 1 }] }))]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].rules).toEqual([{ id: 'r1', version: 1 }]);
  });
  it('renders a malformed injection Event as an empty divider, never nothing', () => {
    const c = cardForEvent(ev(1, 'injection', 'not json'));
    expect(c.card).toBe('divider');
    expect(c.body).toBe('');
    expect(c.rules).toEqual([]);
  });
});

describe('groupTranscript', () => {
  it('merges consecutive assistant chunks into one block', () => {
    const blocks = groupTranscript([ev(1, 'assistant_chunk', 'hel'), ev(2, 'assistant_chunk', 'lo')]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('assistant');
    expect(blocks[0].body).toBe('hello');
  });
  it('keeps user messages as separate blocks', () => {
    const blocks = groupTranscript([ev(1, 'prompt', 'a'), ev(2, 'prompt', 'b')]);
    expect(blocks).toHaveLength(2);
  });
  it('merges tool started + updates with same name into one strip run', () => {
    const blocks = groupTranscript([
      ev(1, 'tool_call_started', 'read\n{"path":"a"}'),
      ev(2, 'tool_call_updated', 'read\nline1\nline2'),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('toolstrip');
    expect(blocks[0].runs).toHaveLength(1);
    expect(blocks[0].runs[0].output).toContain('line2');
  });
});

describe('groupTranscript tool runs', () => {
  it('keeps two sequential calls to the same tool as separate runs in one strip', () => {
    const blocks = groupTranscript([
      ev(1, 'tool_call_started', 'read\na'),
      ev(2, 'tool_call_updated', 'read\nout-a'),
      ev(3, 'tool_call_started', 'read\nb'),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('toolstrip');
    expect(blocks[0].runs).toHaveLength(2);
    expect(blocks[0].runs[0].output).toContain('out-a');
    expect(blocks[0].runs[1].output).toContain('b');
  });
});

describe('compaction cards', () => {
  const started = JSON.stringify({ reason: 'manual' });
  const done = JSON.stringify({ reason: 'threshold', tokensBefore: 180000, tokensAfter: 24000, summary: '## Task\nship it' });
  const failed = JSON.stringify({ reason: 'manual', aborted: true, error: 'model unavailable' });

  it('a started compaction renders as a running card', () => {
    const c = cardForEvent(ev(1, 'compaction_started', started));
    expect(c.card).toBe('compaction');
    expect(c.compaction.state).toBe('running');
  });
  it('the end event replaces the running card in place', () => {
    const blocks = groupTranscript([ev(1, 'prompt', 'hi'), ev(2, 'compaction_started', started), ev(3, 'compaction', done)]);
    const cards = blocks.filter((b) => b.card === 'compaction');
    expect(cards).toHaveLength(1);
    expect(cards[0].compaction).toMatchObject({ state: 'done', tokensBefore: 180000, tokensAfter: 24000 });
    expect(cards[0].body).toContain('ship it');
  });
  it('an aborted or errored compaction is a failed card', () => {
    const blocks = groupTranscript([ev(1, 'compaction_started', started), ev(2, 'compaction', failed)]);
    expect(blocks[0].compaction.state).toBe('failed');
    expect(blocks[0].compaction.error).toBe('model unavailable');
  });
  it('an end event with no started event still renders one card', () => {
    expect(groupTranscript([ev(1, 'compaction', done)])).toHaveLength(1);
  });
  it('malformed payload still renders a card with no numbers', () => {
    const blocks = groupTranscript([ev(1, 'compaction', 'not json')]);
    expect(blocks[0].card).toBe('compaction');
  });
  it('usage stays hidden from the transcript', () => {
    expect(groupTranscript([ev(1, 'usage', '{"used":1,"size":2}')])).toHaveLength(0);
  });
});

import { parseSudoResult, groupTranscript as groupSudo } from './events.js';

describe('sudo cards', () => {
  it('parses the TOON result message', () => {
    const r = parseSudoResult('sudo_result:\n  request: sudo-1\n  status: approved\n  exit: 0\n  command: "sudo ls"\n  output: "a\\nb"');
    expect(r).toEqual({ id: 'sudo-1', status: 'approved', exit: 0, command: 'sudo ls', output: 'a\nb' });
    expect(parseSudoResult('hello')).toBeNull();
  });
  it('updates the request card in place when resolved', () => {
    const req = { seq: 1, kind: 'sudo_request', text: JSON.stringify({ id: 's1', command: 'sudo ls', state: 'pending' }) };
    const res = { seq: 2, kind: 'sudo_resolved', text: JSON.stringify({ id: 's1', command: 'sudo ls', state: 'running' }) };
    const blocks = groupSudo([req, res]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('sudo');
    expect(blocks[0].sudo.state).toBe('running');
  });
  it('shows a sudo_result prompt as a result card', () => {
    const blocks = groupSudo([{ seq: 3, kind: 'prompt', text: 'sudo_result:\n  request: s1\n  status: denied\n  exit: -1\n  command: "sudo x"\n  output: "no"' }]);
    expect(blocks[0].card).toBe('sudoresult');
  });
});

describe('Profile patch cards', () => {
  it('renders proposals and updates the inline card after approval or rejection', async () => {
    const { groupTranscript } = await import('./events.js');
    const blocks = groupTranscript([
      ev(1, 'profile_patch', JSON.stringify({ id: 7, ops: [{ op: 'add', text: 'Prefer tabs' }], reason: 'Repeated preference', state: 'pending' })),
      ev(2, 'profile_patch_resolved', JSON.stringify({ id: 7, status: 'approved' })),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0]).toMatchObject({ card: 'profilePatch', profilePatch: { id: 7, state: 'approved' } });
  });
});

describe('subagent status and grouping', () => {
  it('drops idle and working status pills, keeps failed and ended', async () => {
    const { cardForEvent } = await import('./events.js');
    expect(cardForEvent(ev(1, 'status', '', 'idle')).card).toBe('none');
    expect(cardForEvent(ev(1, 'status', '', 'working')).card).toBe('none');
    expect(cardForEvent(ev(1, 'status', '', 'failed')).card).toBe('status');
    expect(cardForEvent(ev(1, 'status', '', 'ended')).card).toBe('status');
  });
  it('names a spawned card from its payload, never a bare id', async () => {
    const { cardForEvent } = await import('./events.js');
    const c = cardForEvent(ev(1, 'subagent_spawned', JSON.stringify({ id: 'abc123', group: 'g1', name: 'Alpha' })));
    expect(c.card).toBe('subagent');
    expect(c.title).toBe('Alpha');
    expect(c.childId).toBe('abc123');
    expect(c.groupId).toBe('g1');
  });
  it('merges one spawn call into one Group header block', async () => {
    const { groupTranscript } = await import('./events.js');
    const blocks = groupTranscript([
      ev(1, 'subagent_spawned', JSON.stringify({ id: 'a', group: 'g', name: 'Alpha' })),
      ev(2, 'subagent_spawned', JSON.stringify({ id: 'b', group: 'g', name: 'Beta' })),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('group');
    expect(blocks[0].members.map((m) => m.name)).toEqual(['Alpha', 'Beta']);
  });
  it('settles the Group block on its finished marker instead of a new card', async () => {
    const { groupTranscript } = await import('./events.js');
    const blocks = groupTranscript([
      ev(1, 'subagent_spawned', JSON.stringify({ id: 'a', group: 'g', name: 'Alpha' })),
      ev(2, 'subagent_finished', 'g'),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('group');
    expect(blocks[0].finished).toBe(true);
  });
  it('carries stored image ids on user blocks', async () => {
    const { groupTranscript } = await import('./events.js');
    const blocks = groupTranscript([{ seq: 1, kind: 'prompt', text: 'see this', imageIds: ['img1'] }]);
    expect(blocks[0].imageIds).toEqual(['img1']);
  });
});

describe('spawn tool call', () => {
  it('is replaced by its Group card unless it failed', () => {
    const spawn = (seq, status) => ({ seq, kind: 'tool_call_started', text: JSON.stringify({ id: 's', name: 'spawn', input: '{}', status }) });
    expect(groupTranscript([spawn(1, 'running')]).length).toBe(0);
    expect(groupTranscript([spawn(1, 'failed')])[0].card).toBe('tool');
  });
});

describe('thinking inside a tool burst', () => {
  it('joins the open strip as a thinking run between tools', () => {
    const ev = (seq, kind, extra) => ({ seq, kind, at: new Date(seq * 1000).toISOString(), ...extra });
    const blocks = groupTranscript([
      ev(1, 'tool_call_started', { text: 'bash\n{"command":"ls"}' }),
      ev(2, 'thinking_chunk', { text: 'plan **next**' }),
      ev(3, 'tool_call_started', { text: 'read\n{"path":"a"}' }),
    ]);
    const strips = blocks.filter((b) => b.card === 'toolstrip');
    expect(strips).toHaveLength(1);
    expect(strips[0].runs.map((r) => r.name)).toEqual(['bash', 'thinking', 'read']);
    expect(strips[0].runs[1].thinking).toBe(true);
  });
});

describe('compress blocked and compressed Events', () => {
  it('a blocked compress renders the banner card with its error', () => {
    const card = cardForEvent(ev(1, 'compress_blocked', JSON.stringify({ number: 2, error: '402 Payment Required', action: 'retry' })));
    expect(card.card).toBe('compressBlocked');
    expect(card.title).toBe('compress needs pig credits');
    expect(card.body).toBe('402 Payment Required');
  });
  it('the compressed divider names the transient retries', () => {
    const card = cardForEvent(ev(1, 'compressed', JSON.stringify({ number: 3, model: 'test-faux/faux-1', fallback: 'summary', retries: 0, transientRetries: 2, startRetries: 1 })));
    expect(card.body).toContain('3 transient retries');
  });
  it('compressBlockReason reads the error and falls back on bad JSON', () => {
    expect(compressBlockReason(JSON.stringify({ error: 'boom' }))).toBe('boom');
    expect(compressBlockReason('not json')).toBe('compress blocked');
  });
});

describe('prompt senders on user cards', () => {
  it('carries the sender and the sending User id on a user prompt', () => {
    expect(cardForEvent({ ...ev(1, 'prompt', 'hi'), sender: 'user', user: 'u-mira' })).toMatchObject({
      card: 'user',
      sender: 'user',
      senderId: 'u-mira',
    });
  });

  it('types a system prompt with no sender id, so it shows no badge', () => {
    expect(cardForEvent({ ...ev(2, 'prompt', 'go'), sender: 'system' })).toMatchObject({ card: 'user', sender: 'system', senderId: '' });
  });
});
