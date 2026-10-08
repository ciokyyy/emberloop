import { describe, it, expect } from 'vitest';
import { cardForEvent, groupTranscript } from './events.js';
const ev = (seq, kind, text = '', status = '', mode = '') => ({ seq, kind, text, status, mode });

describe('cardForEvent maps every Event kind to a card', () => {
  const cases = [
    ['prompt', 'user'],
    ['assistant_chunk', 'assistant'],
    ['thinking_chunk', 'thinking'],
    ['tool_call_started', 'tool'],
    ['tool_call_updated', 'tool'],
    ['diff', 'diff'],
    ['permission', 'permission'],
    ['status', 'status'],
    ['subagent_spawned', 'subagent'],
    ['subagent_finished', 'subagent'],
    ['error', 'error'],
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
  it('labels plain prompts as You', () => {
    expect(cardForEvent(ev(1, 'prompt', 'hi')).title).toBe('You');
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
  it('merges tool started + updates with same name into one collapsed block', () => {
    const blocks = groupTranscript([
      ev(1, 'tool_call_started', 'read\n{"path":"a"}'),
      ev(2, 'tool_call_updated', 'read\nline1\nline2'),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('tool');
    expect(blocks[0].collapsed).toBe(true);
    expect(blocks[0].body).toContain('line2');
  });
});

describe('groupTranscript tool runs', () => {
  it('keeps two sequential calls to the same tool as separate blocks', () => {
    const blocks = groupTranscript([
      ev(1, 'tool_call_started', 'read\na'),
      ev(2, 'tool_call_updated', 'read\nout-a'),
      ev(3, 'tool_call_started', 'read\nb'),
    ]);
    expect(blocks).toHaveLength(2);
    expect(blocks[0].body).toContain('out-a');
    expect(blocks[1].body).toContain('b');
  });
});
