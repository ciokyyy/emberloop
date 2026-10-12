import { describe, it, expect, afterEach } from 'vitest';
import { render, fireEvent } from '@testing-library/svelte';
import ToolStrip from './ToolStrip.svelte';
import Card from './Card.svelte';
import { groupTranscript } from './events.js';

const toolEv = (seq, kind, { id = `t${seq}`, name = 'read', input = '', output = '', status = '' } = {}, extra = {}) => ({
  seq,
  kind,
  text: JSON.stringify({ id, name, input, output, status }),
  status: '',
  ...extra,
});
const stripOf = (events) => groupTranscript(events).find((b) => b.card === 'toolstrip');

const liveBlock = () =>
  stripOf([
    toolEv(1, 'tool_call_started', { id: 't1', name: 'read', input: '{"path":"a.js"}', status: 'running' }),
    toolEv(2, 'tool_call_updated', { id: 't1', name: 'read', output: 'line1\nline2', status: 'completed' }),
    toolEv(3, 'tool_call_started', { id: 't2', name: 'bash', input: 'vitest', output: 'ok', status: 'running' }),
  ]);

afterEach(() => {
  document.body.innerHTML = '';
});

describe('ToolStrip header', () => {
  it('shows the current tool with its server, target and timer', () => {
    const { container } = render(ToolStrip, { props: { block: liveBlock() } });
    const strip = container.querySelector('[data-card="toolstrip"]');
    expect(strip?.textContent).toContain('bash');
    expect(strip?.textContent).toContain('vitest');
    expect(container.querySelector('[data-testid="toolstrip-elapsed"]')?.textContent).toMatch(/\d+\.\d+s/);
    expect(container.querySelector('[data-testid="toolstrip-progress"]')).not.toBeNull();
  });
  it('shows a shimmer placeholder while running with no output', () => {
    const block = stripOf([toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', status: 'running' })]);
    const { container } = render(ToolStrip, { props: { block } });
    expect(container.querySelector('[data-testid="toolstrip-shimmer"]')).not.toBeNull();
  });
});

describe('ToolStrip settled summary', () => {
  it('settles into a one-line summary still expandable to history', async () => {
    const block = stripOf([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'read', status: 'completed' }, { at: '2026-01-01T00:00:00.000Z' }),
      toolEv(2, 'tool_call_started', { id: 't2', name: 'bash', status: 'completed' }, { at: '2026-01-01T00:00:04.300Z' }),
      { seq: 3, kind: 'assistant_chunk', text: 'done' },
    ]);
    // groupTranscript marks the strip settled (it is not the last block);
    // rebuild the visible shape the way Card receives it.
    const settled = { ...block, settled: true, elapsedMs: 4300 };
    const { container } = render(ToolStrip, { props: { block: settled } });
    expect(container.querySelector('[data-testid="toolstrip-summary"]')?.textContent).toBe('2 tools · 4.3s');
    expect(container.querySelector('[data-testid="toolstrip-stage"]')).toBeNull();
    expect(container.querySelector('[data-testid="toolstrip-count"]')?.textContent).toBe('2');
    await fireEvent.click(container.querySelector('[data-testid="toolstrip-history-toggle"]'));
    expect(container.querySelector('[data-testid="toolstrip-history"]')?.getAttribute('data-open')).toBe('true');
    expect(container.querySelectorAll('[data-testid="toolstrip-history-row"]')).toHaveLength(2);
  });
});

describe('ToolStrip failure', () => {
  it('holds the red state with the error visible', () => {
    const block = stripOf([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'bash', output: 'boom: no such file', status: 'failed' }),
    ]);
    const { container } = render(ToolStrip, { props: { block } });
    const strip = container.querySelector('[data-card="toolstrip"]');
    expect(strip?.getAttribute('data-status')).toBe('failed');
    expect(strip?.className).toContain('border-destructive/40');
    const err = container.querySelector('[data-testid="toolstrip-stage"]');
    expect(err?.getAttribute('role')).toBe('alert');
    expect(err?.textContent).toContain('boom: no such file');
  });
});

describe('ToolStrip history', () => {
  it('counts previous tools and expands a row to its saved output', async () => {
    const { container } = render(ToolStrip, { props: { block: liveBlock() } });
    expect(container.querySelector('[data-testid="toolstrip-count"]')?.textContent).toBe('1');
    await fireEvent.click(container.querySelector('[data-testid="toolstrip-history-toggle"]'));
    const rows = container.querySelectorAll('[data-testid="toolstrip-history-row"]');
    expect(rows).toHaveLength(1);
    expect(rows[0].textContent).toContain('read');
    // Saved output stays hidden until its row opens.
    expect(rows[0].textContent).not.toContain('line1');
    await fireEvent.click(rows[0].querySelector('button'));
    expect(rows[0].textContent).toContain('line1');
    expect(rows[0].textContent).toContain('line2');
  });
  it('renders diff runs red and green', () => {
    const block = stripOf([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'edit', input: 'a.js', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'edit', output: '-old\n+new', status: 'completed' }),
    ]);
    const { container } = render(ToolStrip, { props: { block } });
    const stage = container.querySelector('[data-testid="toolstrip-stage"]');
    expect(stage?.querySelector('.diff-del')?.textContent).toContain('-old');
    expect(stage?.querySelector('.diff-add')?.textContent).toContain('+new');
  });
});

describe('ToolStrip motion contract', () => {
  it('caps the stage height with the mockup max', () => {
    const { container } = render(ToolStrip, { props: { block: liveBlock() } });
    expect(container.querySelector('[data-testid="toolstrip-stage"]')?.style.maxHeight).toBe('232px');
  });
  it('takes the reduced-motion path with no slide classes', () => {
    globalThis.matchMedia = () => ({ matches: true, media: '(prefers-reduced-motion: reduce)' });
    try {
      const { container } = render(ToolStrip, { props: { block: liveBlock() } });
      expect(container.querySelector('[data-card="toolstrip"]')?.getAttribute('data-motion')).toBe('off');
      expect(container.querySelector('.ts-cur')).toBeNull();
      expect(container.querySelector('.ts-prev')).toBeNull();
    } finally {
      delete globalThis.matchMedia;
    }
  });
});

describe('ToolStrip streaming', () => {
  it('appends lines as output streams in', async () => {
    const first = stripOf([toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', output: 'line1', status: 'running' })]);
    const { container, rerender } = render(ToolStrip, { props: { block: first } });
    expect(container.querySelectorAll('[data-testid="toolstrip-stage"] .ts-line')).toHaveLength(1);
    const second = stripOf([
      toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', output: 'line1', status: 'running' }),
      toolEv(2, 'tool_call_updated', { id: 't1', name: 'bash', output: 'line1\nline2', status: 'running' }),
    ]);
    await rerender({ block: second });
    const lines = container.querySelectorAll('[data-testid="toolstrip-stage"] .ts-line');
    expect(lines).toHaveLength(2);
    expect(lines[1].textContent).toContain('line2');
  });
});

describe('Card toolstrip wiring', () => {
  it('renders a strip block as the strip card', () => {
    const { container } = render(Card, { props: { block: liveBlock() } });
    expect(container.querySelector('[data-card="toolstrip"]')).not.toBeNull();
  });
});

describe('ToolStrip live gating', () => {
  it('marks the live stage so per-line entrances stay off while streaming', () => {
    const { container } = render(ToolStrip, { props: { block: liveBlock() } });
    expect(container.querySelector('[data-testid="toolstrip-stage"]')?.parentElement?.getAttribute('data-live')).toBe('true');
  });
  it('windows a long live output to the tail with an earlier-lines note', () => {
    const output = Array.from({ length: 250 }, (_, i) => `line${i}`).join('\n');
    const block = stripOf([toolEv(1, 'tool_call_started', { id: 't1', name: 'bash', input: '', output, status: 'running' })]);
    const { container } = render(ToolStrip, { props: { block } });
    expect(container.querySelectorAll('[data-testid="toolstrip-stage"] .ts-line')).toHaveLength(200);
    expect(container.querySelector('[data-testid="toolstrip-stage"]')?.textContent).toContain('+50 earlier lines');
  });
});
