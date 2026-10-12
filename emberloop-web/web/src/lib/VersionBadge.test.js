import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import VersionBadge from './VersionBadge.svelte';
import { uiHash } from './version.js';

// The badge reads /api/version through the global fetch; each test stubs it
// with the Daemon answer it needs.
function stubVersion(body) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: true,
      json: async () => body,
    })),
  );
}

describe('VersionBadge', () => {
  it('shows ui and be hashes once the Daemon answers', async () => {
    stubVersion({ version: 'dev', commit: 'abc1234', uiHash: uiHash() });
    const { container, unmount } = render(VersionBadge);
    const badge = await within(container).findByTestId('version-badge');
    expect(badge.textContent).toContain('ui ');
    expect(badge.textContent).toContain('be abc1234');
    expect(within(container).queryByText(/update available/)).toBeNull();
    unmount();
    vi.unstubAllGlobals();
  });

  it('offers a reload when the Daemon serves a newer UI build', async () => {
    stubVersion({ version: 'dev', commit: 'abc1234', uiHash: 'newer-build-1' });
    const reload = vi.fn();
    Object.defineProperty(window, 'location', { value: { ...window.location, reload }, writable: true });
    const { container, unmount } = render(VersionBadge);
    const pill = await within(container).findByText(/update available — reload/);
    await fireEvent.click(pill);
    expect(reload).toHaveBeenCalled();
    unmount();
    vi.unstubAllGlobals();
  });

  it('copies the badge line on click and flashes copied', async () => {
    stubVersion({ version: 'dev', commit: 'abc1234', uiHash: uiHash() });
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    const { container, unmount } = render(VersionBadge);
    const badge = await within(container).findByTestId('version-badge');
    await fireEvent.click(within(badge).getByText(/ui /));
    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('be abc1234'));
    expect(await within(badge).findByText('copied')).toBeTruthy();
    unmount();
    vi.unstubAllGlobals();
  });

  it('renders nothing while the Daemon is unreachable', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({
        ok: false,
        json: async () => ({}),
      })),
    );
    const { container, unmount } = render(VersionBadge);
    await new Promise((r) => setTimeout(r, 50));
    expect(within(container).queryByTestId('version-badge')).toBeNull();
    unmount();
    vi.unstubAllGlobals();
  });

  it('renders a static stamp for the phone drawer when inline', async () => {
    stubVersion({ version: 'dev', commit: 'abc1234', uiHash: uiHash() });
    const { container, unmount } = render(VersionBadge, { props: { inline: true } });
    const line = await within(container).findByTestId('version-inline');
    expect(line.textContent).toContain('be abc1234');
    expect(within(container).queryByTestId('version-badge')).toBeNull();
    unmount();
    vi.unstubAllGlobals();
  });

  it('offers a phone update pill when the build is stale', async () => {
    stubVersion({ version: 'dev', commit: 'abc1234', uiHash: 'newer-build-2' });
    const { container, unmount } = render(VersionBadge);
    expect(await within(container).findByTestId('update-pill')).toBeTruthy();
    unmount();
    vi.unstubAllGlobals();
  });
});
