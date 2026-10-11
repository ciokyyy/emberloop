import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
import SessionList from './SessionList.svelte';
import { buildTree } from './sidebar.js';

afterEach(() => {
  cleanup();
  localStorage.clear();
});

// Rows as buildOwnerGroups hands them over: a Run row, its Worker, and loose
// Sessions. The parent-child tree comes from buildTree, like the real list.
function mount(list, props = {}) {
  const { roots, kids } = buildTree(list);
  return render(SessionList, { props: { roots, kids, ...props } });
}

const run = { id: 'r1', kind: 'run', title: 'Web run', status: 'running', owner: 'u1' };
const worker = { id: 'w1', harness: 'claude', status: 'working', title: 'Fix rebase', parentId: 'r1', runWorker: true };
const loose = { id: 'a1', harness: 'pig', status: 'failed', title: 'Quota bug' };

describe('SessionList rows', () => {
  it('tags a Run row with the Run badge and leaves Sessions untagged', () => {
    const { getByRole, queryAllByText } = mount([run, loose]);
    expect(within(getByRole('button', { name: /Web run/ })).getByText('Run')).toBeTruthy();
    expect(queryAllByText('Run')).toHaveLength(1);
  });

  it('nests a Run Worker under its Run row, open while the Worker runs', () => {
    const { getByRole, getByText } = mount([run, worker]);
    expect(getByRole('button', { name: /Expand|Collapse subagents of Web run/ }).getAttribute('aria-expanded')).toBe('true');
    expect(getByText('Fix rebase')).toBeTruthy();
  });

  it('collapses a settled Run by default and expands it on the chevron', async () => {
    const settled = { ...worker, status: 'ended' };
    const { getByRole, queryByText } = mount([run, settled]);
    expect(queryByText('Fix rebase')).toBeNull();
    await fireEvent.click(getByRole('button', { name: 'Expand subagents of Web run' }));
    expect(queryByText('Fix rebase')).not.toBeNull();
  });

  it('shows the failed status in text and a red dot, not the needs-you treatment', () => {
    const { getByRole } = mount([loose]);
    const row = getByRole('button', { name: /Quota bug/ });
    expect(row.textContent).toContain('failed');
    expect(row.querySelector('span.bg-red-500')).not.toBeNull();
    expect(row.className).not.toContain('border-amber-500/30');
  });

  it('shows the capped state on a limit-failed Session row', () => {
    const { getByRole } = mount([{ ...loose, capped: true }]);
    const row = getByRole('button', { name: /Quota bug/ });
    expect(row.textContent).toContain('capped');
    expect(row.textContent).not.toContain('failed');
    expect(row.querySelector('span.bg-amber-500')).not.toBeNull();
    expect(row.className).not.toContain('border-amber-500/30');
  });

  it('gives a needs-you row the amber treatment', () => {
    const { getByRole } = mount([{ ...loose, status: 'idle', needsYou: true }]);
    expect(getByRole('button', { name: /Quota bug/ }).className).toContain('border-amber-500/30');
  });

  it('marks a scout row with the scout badge', () => {
    const { getByRole } = mount([{ id: 's1', harness: 'claude', status: 'idle', title: 'Find callers', scout: true }]);
    expect(within(getByRole('button', { name: /Find callers/ })).getByText('scout')).toBeTruthy();
  });

  it('passes the whole row to onSelect on click, so a Run opens its own page', async () => {
    const onSelect = vi.fn();
    const { getByRole } = mount([run, loose], { onSelect });
    await fireEvent.click(getByRole('button', { name: /Web run/ }));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: 'r1', kind: 'run' }));
    await fireEvent.click(getByRole('button', { name: /Quota bug/ }));
    expect(onSelect).toHaveBeenLastCalledWith(expect.objectContaining({ id: 'a1' }));
  });

  it('marks the open row as the current one', () => {
    const { getByRole } = mount([loose], { currentId: 'a1' });
    expect(getByRole('button', { name: /Quota bug/ }).getAttribute('aria-current')).toBe('true');
  });
});
