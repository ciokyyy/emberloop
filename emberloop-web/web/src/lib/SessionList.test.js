import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import SessionList from './SessionList.svelte';

const sessions = [
  { id: 'a', harness: 'claude', title: 'Failed chat', status: 'failed' },
  { id: 'b', harness: 'pig', title: 'Working chat', status: 'working' },
  { id: 'c', harness: 'claude', title: 'Old chat', status: 'idle' },
  { id: 'd', harness: 'claude', name: 'kid-1', status: 'ended', parentId: 'c' },
  { id: 'e', harness: 'claude', name: 'kid-2', status: 'ended', parentId: 'c' },
  { id: 'f', harness: 'claude', title: 'Running parent', status: 'working' },
  { id: 'g', harness: 'claude', name: 'kid-3', status: 'working', parentId: 'f' },
];

beforeEach(() => localStorage.clear());

describe('attention tiers', () => {
  it('renders Needs you / Working / Recent sections with counts', async () => {
    const { container } = render(SessionList, { props: { sessions, onSelect: vi.fn() } });
    for (const [name, count] of [['Needs you', '1'], ['Working', '2'], ['Recent', '1']]) {
      const section = container.querySelector(`section[aria-label="${name}"]`);
      expect(section?.textContent).toContain(name);
      expect(section?.textContent).toContain(count);
    }
    expect(within(container).getByText('Failed chat')).toBeTruthy();
    expect(within(container).getByText('Working chat')).toBeTruthy();
    expect(within(container).getByText('Old chat')).toBeTruthy();
  });
  it('marks failed rows loud and keeps the rest quiet', async () => {
    const { container } = render(SessionList, { props: { sessions, onSelect: vi.fn() } });
    const failed = (await within(container).findByText('Failed chat')).closest('button');
    expect(failed.className).toContain('bg-amber-500/10');
    const idle = (await within(container).findByText('Old chat')).closest('button');
    expect(idle.className).not.toContain('bg-amber-500/10');
  });

  it('skips headers when a single tier is populated', async () => {
    const { container } = render(SessionList, {
      props: { sessions: [{ id: 'x', harness: 'claude', title: 'Solo', status: 'idle' }], onSelect: vi.fn() },
    });
    expect(await within(container).findByText('Solo')).toBeTruthy();
    expect(within(container).queryByRole('button', { name: /recent/i })).toBeNull();
  });
});

describe('nested subagents', () => {
  it('collapses a settled group by default and expands on chevron', async () => {
    const { container } = render(SessionList, { props: { sessions, onSelect: vi.fn() } });
    expect(within(container).queryByText('kid-1')).toBeNull();
    const chevron = await within(container).findByRole('button', { name: /expand subagents of old chat/i });
    await fireEvent.click(chevron);
    expect(await within(container).findByText('kid-1')).toBeTruthy();
    expect(await within(container).findByText('kid-2')).toBeTruthy();
    const prefs = JSON.parse(localStorage.getItem('eb.subagents-collapsed'));
    expect(prefs.c).toBe(false);
  });

  it('keeps a running group expanded by default', async () => {
    const { container } = render(SessionList, { props: { sessions, onSelect: vi.fn() } });
    expect(await within(container).findByText('kid-3')).toBeTruthy();
  });

  it('selects sessions on row click', async () => {
    const onSelect = vi.fn();
    const { container } = render(SessionList, { props: { sessions, onSelect } });
    await fireEvent.click(await within(container).findByText('Working chat'));
    expect(onSelect).toHaveBeenCalledWith('b');
  });
});
