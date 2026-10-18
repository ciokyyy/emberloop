import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import GitPanel from './GitPanel.svelte';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const git = {
  isRepo: true,
  branch: 'main',
  ahead: 2,
  behind: 0,
  upstream: 'origin/main',
  files: [
    { path: 'src/a.txt', index: ' ', work: 'M' },
    { path: 'b.txt', index: ' ', work: '?' },
  ],
  log: [{ hash: 'abc1234', subject: 'first commit', author: 'e2e', date: 'today' }],
};

describe('GitPanel', () => {
  it('lists changes, opens a diff and goes back to the list', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ diff: '@@ -1 +1 @@\n-one\n+two' }) })));
    render(GitPanel, { props: { projectId: 'p1', git } });
    expect(screen.getByRole('heading', { name: 'main' })).toBeTruthy();
    expect(screen.getByText('a.txt')).toBeTruthy();
    expect(screen.getByText('first commit')).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: /a\.txt/ }));
    const region = await screen.findByRole('region', { name: 'Diff of src/a.txt' });
    await waitFor(() => expect(region.textContent).toContain('+two'));
    await fireEvent.click(screen.getByRole('button', { name: 'Back to changes' }));
    expect(screen.queryByRole('region', { name: /Diff of/ })).toBeNull();
  });

  it('names Refresh and Close as icon actions', async () => {
    const onRefresh = vi.fn();
    const onClose = vi.fn();
    render(GitPanel, { props: { projectId: 'p1', git, onRefresh, onClose } });
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Close git' }));
    expect(onRefresh).toHaveBeenCalled();
    expect(onClose).toHaveBeenCalled();
  });

  it('shows an empty state outside a repository and a skeleton while loading', () => {
    const { unmount } = render(GitPanel, { props: { projectId: 'p1', git: null } });
    expect(screen.getByRole('status', { name: 'Loading git status' })).toBeTruthy();
    unmount();
    render(GitPanel, { props: { projectId: 'p1', git: { isRepo: false } } });
    expect(screen.getByText('Not a git repository')).toBeTruthy();
  });
});
