import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import PromotionQueue from './PromotionQueue.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

const item = (over = {}) => ({
  id: 7,
  status: 'pending',
  target: 'agents_md',
  target_path: 'sub/AGENTS.md',
  diff: '--- a/sub/AGENTS.md\n+++ b/sub/AGENTS.md\n@@\n+- always wrap errors\n',
  ...over,
});

function stubQueue({ queue = [], seen = {} }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      const method = opts?.method ?? 'GET';
      if (url.includes('/api/promotions/queue/') && url.endsWith('/decide') && method === 'POST') {
        seen.decide = JSON.parse(opts.body);
        return { ok: true, json: async () => ({ status: seen.decide.status, rule_id: 'r1' }) };
      }
      if (url.includes('/api/promotions/queue') && method === 'GET') {
        return { ok: true, json: async () => queue };
      }
      return { ok: true, json: async () => ({}) };
    }),
  );
}

describe('PromotionQueue', () => {
  it('shows each proposal with its target path and diff for approval', async () => {
    stubQueue({ queue: [item()] });
    const { container } = render(PromotionQueue);
    const row = await within(container).findByTestId('promotion-queue-7');
    expect(row.textContent).toContain('sub/AGENTS.md');
    const diff = within(row).getByTestId('promotion-diff-7');
    expect(diff.textContent).toContain('+- always wrap errors');
  });

  it('shows the empty state with no promotions', async () => {
    stubQueue({ queue: [] });
    const { container } = render(PromotionQueue);
    expect(await within(container).findByText(/No promotions/)).toBeTruthy();
  });
  it('approves through the queue and reports the uncommitted write', async () => {
    const seen = {};
    stubQueue({ queue: [item()], seen });
    const { container } = render(PromotionQueue);
    await within(container).findByTestId('promotion-queue-7');
    await fireEvent.click(within(container).getByText('Approve'));
    await tick();
    expect(seen.decide).toEqual({ status: 'approved' });
    expect(container.querySelector('[role="status"]')?.textContent).toMatch(/uncommitted/);
  });

  it('rejects through the queue and keeps the label notice', async () => {
    const seen = {};
    stubQueue({ queue: [item()], seen });
    const { container } = render(PromotionQueue);
    await within(container).findByTestId('promotion-queue-7');
    await fireEvent.click(within(container).getByText('Reject'));
    await tick();
    expect(seen.decide).toEqual({ status: 'rejected' });
    expect(await within(container).findByText(/stays as a label/)).toBeTruthy();
  });

  it('hides the buttons on decided items', async () => {
    stubQueue({ queue: [item({ status: 'approved' })] });
    const { container } = render(PromotionQueue);
    const row = await within(container).findByTestId('promotion-queue-7');
    expect(within(row).queryByText('Approve')).toBeNull();
    expect(within(row).queryByText('Reject')).toBeNull();
  });
});
