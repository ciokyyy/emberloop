import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import RunsGraph from './RunsGraph.svelte';

const tickets = [
  { id: '01-a', num: 1, title: 'Alpha', state: 'merged', blockers: [] },
  { id: '02-b', num: 2, title: 'Beta', state: 'running', blockers: ['01-a'] },
  { id: '03-c', num: 3, title: 'Gamma', state: 'queued', blockers: ['02-b'] },
  { id: '04-d', num: 4, title: 'Delta', state: 'needs-you', blockers: [], needsYou: 'same refusal ×3', lastRefusal: '' },
];

describe('RunsGraph', () => {
  it('renders one card per ticket with status labels and a legend', async () => {
    const { container } = render(RunsGraph, { props: { tickets } });
    for (const t of ['Alpha', 'Beta', 'Gamma', 'Delta']) {
      expect(await within(container).findByText(t)).toBeTruthy();
    }
    expect(within(container).getByText('merged')).toBeTruthy();
    expect(within(container).getByText('blocked by 02')).toBeTruthy();
    expect(within(container).getByText('same refusal ×3')).toBeTruthy();
    const legend = within(container).getByTestId('runs-legend');
    expect(legend.textContent).toMatch(/1 merged/);
    expect(legend.textContent).toMatch(/1 running/);
    expect(legend.textContent).toMatch(/1 blocked/);
    expect(legend.textContent).toMatch(/1 needs-you/);
    // One SVG edge per resolved blocker (scoped: lucide icons render paths too).
    expect(within(container).getByTestId('runs-edges').querySelectorAll('path').length).toBe(2);
  });

  it('selects a ticket on card click and marks the deep-linked one', async () => {
    const onSelect = vi.fn();
    const { container } = render(RunsGraph, { props: { tickets, selectedId: '02-b', onSelect } });
    const beta = within(container).getByRole('button', { name: /ticket beta/i });
    expect(beta.getAttribute('aria-current')).toBe('true');
    await fireEvent.click(within(container).getByRole('button', { name: /ticket gamma/i }));
    expect(onSelect).toHaveBeenCalledWith(tickets[2]);
  });

  it('zooms around the canvas and resets', async () => {
    const { container } = render(RunsGraph, { props: { tickets } });
    const canvas = within(container).getByTestId('runs-canvas');
    // @panzoom/panzoom owns the transform; wait for it to mount and settle.
    await vi.waitFor(() => expect(canvas.style.transform).toMatch(/scale\(1\)/));
    await fireEvent.click(within(container).getByRole('button', { name: /zoom in/i }));
    await vi.waitFor(() => expect(canvas.style.transform).toMatch(/scale\(1\.[1-9]/));
    await fireEvent.click(within(container).getByRole('button', { name: /reset view/i }));
    await vi.waitFor(() => expect(canvas.style.transform).toMatch(/scale\(1\)/));
  });

  it('shows an empty state with no tickets', async () => {
    const { container } = render(RunsGraph, { props: { tickets: [] } });
    expect(await within(container).findByText(/no tickets yet/i)).toBeTruthy();
  });

  it('renders duplicate ticket ids once and stays keyboard reachable', async () => {
    const dupes = [
      { id: '01-a', num: 1, title: 'Alpha', state: 'running', blockers: [] },
      { id: '01-a', num: 1, title: 'Alpha copy', state: 'queued', blockers: [] },
    ];
    const { container } = render(RunsGraph, { props: { tickets: dupes } });
    // One card, not two stacked on the same id.
    expect(within(container).getAllByRole('button', { name: /ticket alpha/i })).toHaveLength(1);
    // The viewport is focusable and documents its keys; arrows pan it.
    const graph = within(container).getByTestId('runs-graph');
    expect(graph.getAttribute('tabindex')).toBe('0');
    expect(graph.getAttribute('aria-label')).toMatch(/arrow keys/i);
    const canvas = within(container).getByTestId('runs-canvas');
    await vi.waitFor(() => expect(canvas.style.transform).not.toBe(''));
    const before = canvas.style.transform;
    await fireEvent.keyDown(graph, { key: 'ArrowRight' });
    await vi.waitFor(() => expect(canvas.style.transform).not.toBe(before));
  });

  it('cleans its wheel listener up on unmount', async () => {
    const add = vi.spyOn(HTMLElement.prototype, 'addEventListener');
    const remove = vi.spyOn(HTMLElement.prototype, 'removeEventListener');
    const { container, unmount } = render(RunsGraph, { props: { tickets } });
    await within(container).findByTestId('runs-graph');
    expect(add.mock.calls.some(([type]) => type === 'wheel')).toBe(true);
    unmount();
    expect(remove.mock.calls.some(([type]) => type === 'wheel')).toBe(true);
    add.mockRestore();
    remove.mockRestore();
  });
});
