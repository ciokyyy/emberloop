import { describe, it, expect } from 'vitest';
import { groupTranscript, stripRunStatus } from './events.js';
import {
  STRIP_MAX_HEIGHT,
  isStripExcluded,
  stripSummary,
  formatElapsed,
  toolTarget,
  splitOutputLines,
  outputKind,
  toolIcon,
  prefersReducedMotion,
  shortTool,
  activeTool,
  kindSummary,
} from './toolstrip.js';

const toolEv = (seq, kind, { id = `t${seq}`, name = 'read', input = '', output = '', status = '' } = {}, extra = {}) => ({
  seq,
  kind,
  text: JSON.stringify({ id, name, input, output, status }),
  status: '',
  ...extra,
});
const ev = (seq, kind, text = '') => ({ seq, kind, text });

describe('tool strip grouping', () => {
  it('collapses consecutive tool calls into one strip card', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', input: '{"path":"a"}', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'read', output: 'line1', status: 'running' }),
      toolEv(3, 'tool_call_started', { id: 't2', name: 'bash', input: 'vitest', status: 'running' }),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].card).toBe('toolstrip');
    expect(blocks[0].runs.map((r) => r.name)).toEqual(['read', 'bash']);
  });
  it('joins updates into their run by tool id', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'bash', output: 'line1\nline2', status: 'completed' }),
    ]);
    expect(blocks).toHaveLength(1);
    expect(blocks[0].runs).toHaveLength(1);
    expect(blocks[0].runs[0].output).toBe('line1\nline2');
    expect(blocks[0].status).toBe('completed');
  });
  it('a finished event settles its run', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'running' }),
      toolEv(2, 'tool_call_finished', { id: 't1', name: 'read', output: 'done', status: 'completed' }),
    ]);
    expect(blocks[0].runs).toHaveLength(1);
    expect(blocks[0].status).toBe('completed');
  });
  it('starts a new strip after assistant text and settles the old one', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'completed' }),
      ev(2, 'assistant_chunk', 'on it'),
      toolEv(3, 'tool_call_started', { id: 't2', name: 'bash', status: 'running' }),
    ]);
    expect(blocks.map((b) => b.card)).toEqual(['toolstrip', 'assistant', 'toolstrip']);
    expect(blocks[0].settled).toBe(true);
    expect(blocks[2].settled).toBe(false);
  });
  it('a user message between tools starts a new strip', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'completed' }),
      ev(2, 'prompt', 'go on'),
      toolEv(3, 'tool_call_started', { id: 't2', name: 'read', status: 'running' }),
    ]);
    expect(blocks.filter((b) => b.card === 'toolstrip')).toHaveLength(2);
  });
  it('computes elapsed time from event timestamps', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read' }, { at: '2026-01-01T00:00:00.000Z' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'read' }, { at: '2026-01-01T00:00:04.300Z' }),
    ]);
    expect(blocks[0].elapsedMs).toBe(4300);
  });
  it('marks the strip failed when the current tool failed', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'bash', output: 'boom', status: 'failed' }),
    ]);
    expect(blocks[0].status).toBe('failed');
    expect(blocks[0].runs[0].output).toContain('boom');
  });
});

describe('tool strip exclusions', () => {
  for (const name of ['submit', 'sudo']) {
    it(`keeps ${name} out of the strip as its own card`, () => {
      const blocks = groupTranscript([
        toolEv(1, 'tool_call_started', { id: 't1', name, status: 'running' }),
        toolEv(2, 'tool_call_started', { id: 't2', name: 'read', status: 'running' }),
      ]);
      expect(blocks[0].card).toBe('tool');
      expect(blocks[0].title).toBe(name);
      expect(blocks[1].card).toBe('toolstrip');
    });
  }
  it('leaves sudo and approval cards untouched', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'running' }),
      ev(2, 'sudo_request', JSON.stringify({ id: 's1', command: 'sudo ls', state: 'pending' })),
      ev(3, 'permission', 'read auto-approved as allow'),
    ]);
    expect(blocks.map((b) => b.card)).toEqual(['toolstrip', 'sudo', 'permission']);
  });
  it('a running spawn tool call is hidden, its Group card stands in', () => {
    const blocks = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'running' }),
      toolEv(2, 'tool_call_started', { id: 't2', name: 'spawn', status: 'running' }),
      toolEv(3, 'tool_call_started', { id: 't3', name: 'grep', status: 'running' }),
    ]);
    expect(blocks.every((b) => b.card === 'toolstrip' && b.runs.every((r) => r.name !== 'spawn'))).toBe(true);
  });
});

describe('stripRunStatus', () => {
  it('reduces backend statuses to the three strip states', () => {
    expect(stripRunStatus('failed')).toBe('failed');
    expect(stripRunStatus('completed')).toBe('completed');
    expect(stripRunStatus('complete')).toBe('completed');
    expect(stripRunStatus('running')).toBe('running');
    expect(stripRunStatus('')).toBe('running');
  });
});

describe('tool strip formatting', () => {
  it('summarizes a settled strip as "N tools · Xs"', () => {
    expect(stripSummary(3, 4300)).toBe('3 tools · 4.3s');
    expect(stripSummary(1, 200)).toBe('1 tool · 0.2s');
  });
  it('formats elapsed seconds with one decimal', () => {
    expect(formatElapsed(0)).toBe('0.0s');
    expect(formatElapsed(1500)).toBe('1.5s');
  });
  it('prefers the file-like token for the header target', () => {
    expect(toolTarget({ input: '{"path":"src/lib/Composer.svelte"}' })).toContain('src/lib/Composer.svelte');
    expect(toolTarget({ input: '' })).toBe('');
  });
  it('splits output into lines without a phantom trailing line', () => {
    expect(splitOutputLines('a\nb\n')).toEqual(['a', 'b']);
    expect(splitOutputLines('a\n\nb')).toEqual(['a', '', 'b']);
    expect(splitOutputLines('')).toEqual(['']);
  });
  it('picks the renderer per tool family', () => {
    expect(outputKind('read')).toBe('file');
    expect(outputKind('grep')).toBe('file');
    expect(outputKind('edit')).toBe('diff');
    expect(outputKind('write')).toBe('diff');
    expect(outputKind('bash')).toBe('stream');
    expect(outputKind('mystery-tool')).toBe('json');
  });
  it('maps tool families to header glyphs', () => {
    expect(toolIcon('read')).toBe('read');
    expect(toolIcon('grep')).toBe('grep');
    expect(toolIcon('edit')).toBe('edit');
    expect(toolIcon('write')).toBe('write');
    expect(toolIcon('bash')).toBe('bash');
    expect(toolIcon('whatever')).toBe('tool');
  });
  it('excludes spawn, submit and sudo case-insensitively', () => {
    expect(isStripExcluded('spawn')).toBe(true);
    expect(isStripExcluded('Submit')).toBe(true);
    expect(isStripExcluded('read')).toBe(false);
  });
  it('caps the stage at the mockup height', () => {
    expect(STRIP_MAX_HEIGHT).toBe(232);
  });
  it('takes the reduced-motion path only under the media query', () => {
    expect(prefersReducedMotion()).toBe(false);
    const matchMedia = globalThis.matchMedia;
    globalThis.matchMedia = (q) => ({ matches: q === '(prefers-reduced-motion: reduce)', media: q });
    try {
      expect(prefersReducedMotion()).toBe(true);
    } finally {
      if (matchMedia) globalThis.matchMedia = matchMedia;
      else delete globalThis.matchMedia;
    }
  });
});

describe('idle session settles open strips', () => {
  it('closes a running tool as failed when the Session is not working', () => {
    const evs = [toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', status: 'running' })];
    expect(groupTranscript(evs)[0].settled).toBe(false);
    const idle = groupTranscript(evs, {}, { working: false })[0];
    expect(idle.settled).toBe(true);
    expect(idle.runs[0].status).toBe('failed');
    expect(idle.status).toBe('failed');
  });
});

describe('Working indicator helpers', () => {
  it('shortTool trims long names to a few characters', () => {
    expect(shortTool('bash')).toBe('bash');
    expect(shortTool('mcp__server__very_long_tool')).toBe('mcp__server__…');
    expect(shortTool(undefined)).toBe('');
  });
  it('activeTool names the current run of a trailing live strip only', () => {
    const live = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'completed' }),
      toolEv(2, 'tool_call_started', { id: 't2', name: 'bash', status: 'running' }),
    ]);
    expect(activeTool(live)).toBe('bash');
    const settled = groupTranscript([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'completed' }),
      ev(2, 'assistant_chunk', 'done'),
    ]);
    expect(activeTool(settled)).toBe('');
    expect(activeTool([])).toBe('');
  });
  it('kindSummary counts runs per tool kind in first-seen order', () => {
    const runs = [{ name: 'read' }, { name: 'bash' }, { name: 'read' }, { name: 'x', thinking: true }];
    expect(kindSummary(runs)).toEqual([
      { kind: 'read', count: 2 },
      { kind: 'bash', count: 1 },
      { kind: 'thinking', count: 1 },
    ]);
  });
});
