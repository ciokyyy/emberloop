import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within } from '@testing-library/svelte';

vi.mock('./api.js', () => ({
  getSession: vi.fn(async () => ({ session: { id: 'c1', name: 'Alpha', harness: 'pig', status: 'idle' }, events: [] })),
  openStream: vi.fn(() => ({ close: vi.fn() })),
}));

import GroupCard from './GroupCard.svelte';
import { getSession } from './api.js';

const group = (members) => ({ key: 1, card: 'group', title: '1 subagent', groupId: 'g', members, finished: false, events: [] });

describe('GroupCard', () => {
  beforeEach(() => vi.clearAllMocks());
  it('shows the name, harness mark, type chip and live counts', async () => {
    const { container } = render(GroupCard, {
      props: { block: group([{ id: 'c1', name: 'Alpha', harness: 'pig', scout: false, status: 'idle' }]), sessions: [], onOpenSession: () => {} },
    });
    await within(container).findByText('Alpha');
    expect(container.querySelector('[data-card="group"]').textContent).toContain('1 subagent');
    expect(container.querySelector('[data-child="c1"]').textContent).toContain('worker');
    expect(getSession).toHaveBeenCalledWith('c1');
  });
  it('shows final worker and scout counts for a completed group', async () => {
    getSession.mockImplementation(async (id) => ({
      session: { id, name: id === 'c1' ? 'Alpha' : 'Scout', harness: 'pig', status: id === 'c1' ? 'ended' : 'failed', scout: id === 'c2' },
      events: id === 'c1' ? [{ kind: 'assistant_chunk', text: 'done' }] : [],
    }));
    const { container } = render(GroupCard, {
      props: { block: group([{ id: 'c1', name: 'Alpha', harness: 'pig' }, { id: 'c2', name: 'Scout', harness: 'pig', scout: true }]) },
    });
    await within(container).findByText('Alpha');
    await vi.waitFor(() => expect(container.querySelector('[data-card="group"]')?.textContent).toMatch(/1 done · 0 running\s*· 1 failed/));
    expect(container.querySelector('[data-card="group"]')?.textContent).toContain('2 subagents');
  });
  it('tints the pig row edge teal and the claude row terracotta', async () => {
    const { container } = render(GroupCard, {
      props: {
        block: group([
          { id: 'c1', name: 'PigOne', harness: 'pig', scout: false, status: 'idle' },
          { id: 'c2', name: 'ClOne', harness: 'claude', scout: true, status: 'idle' },
        ]),
        sessions: [],
        onOpenSession: () => {},
      },
    });
    await within(container).findByText('PigOne');
    const rows = [...container.querySelectorAll('[data-child]')];
    expect(rows[0].getAttribute('style')).toContain('rgb(79, 176, 143)');
    expect(rows[1].getAttribute('style')).toContain('rgb(217, 119, 87)');
    expect(rows[1].textContent).toContain('scout');
  });
});
