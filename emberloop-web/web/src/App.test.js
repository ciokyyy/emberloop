import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, within, screen, fireEvent, cleanup } from '@testing-library/svelte';
import App from './App.svelte';

// jsdom has no ResizeObserver; App measures the floating composer with one.
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
});

describe('Runs navigation', () => {
  it('opens the Runs view from the sidebar', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => {
          if (String(u).includes('/api/specs')) return { specs: [] };
          return [];
        },
      })),
    );
    const { container } = render(App);
    await fireEvent.click(await within(container).findByLabelText('Specs and Runs'));
    expect(await within(container).findByText(/repo directory/i)).toBeTruthy();
  });
});

describe('Header', () => {
  it('shows a compact Live dot, and Reconnecting text only when disconnected', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => [] })));
    const { container } = render(App);
    expect(await within(container).findByTestId('live')).toBeTruthy();
    expect(within(container).queryByText('Live')).toBeNull();
    expect(within(container).getByTestId('header-title').textContent).toContain('No Session');
  });
});

describe('Sidebar collapse', () => {
  const sessionsBody = [{ id: 's1', harness: 'fake', model: 'fake-model', title: 'Fix sidebar', status: 'idle', projectId: 'p1', owner: 'u1' }];
  const usersBody = [{ id: 'u1', username: 'cioky', admin: true, online: true }];
  const me = { id: 'u1', username: 'cioky', admin: true };
  const projectsBody = [{ id: 'p1', name: 'demo', path: '/tmp/el-e2e/repo' }];
  afterEach(() => cleanup());
  function stubApp() {
    // Each render boots from the URL: reset it, or a leaked /runs from an
    // earlier test opens the Runs view instead of the chat sidebar.
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => {
          if (String(u).includes('/queue')) return { queue: [] };
          if (String(u).includes('/api/sessions/s1')) return { session: sessionsBody[0], events: [], resumable: true, hasMore: false, firstSeq: 0 };
          if (String(u).includes('/api/sessions')) return sessionsBody;
          if (String(u).includes('/api/projects')) return projectsBody;
          if (String(u).includes('/api/users')) return usersBody;
          if (String(u).includes('/api/specs')) return { specs: [] };
          return [];
        },
      })),
    );
  }

  it('toggles from the rail and persists across reloads', async () => {
    stubApp();
    const { container, unmount } = render(App);
    const sidebar = await within(container).findByTestId('sidebar');
    expect(sidebar.className).toContain('w-64');
    await fireEvent.click(await within(container).findByRole('button', { name: 'Collapse sidebar' }));
    expect(sidebar.className).toContain('w-0');
    expect(localStorage.getItem('eb.sidebar-collapsed')).toBe('true');
    unmount();
    vi.unstubAllGlobals();
    vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => [] })));
    const second = render(App);
    expect(second.container.querySelector('[data-testid="sidebar"]').className).toContain('w-0');
  });

  it('toggles collapse on Ctrl+B and ignores typing', async () => {
    stubApp();
    const { container } = render(App);
    const sidebar = await within(container).findByTestId('sidebar');
    await fireEvent.keyDown(window, { key: 'b', ctrlKey: true });
    expect(sidebar.className).toContain('w-0');
    await fireEvent.keyDown(window, { key: 'b', ctrlKey: true });
    expect(sidebar.className).toContain('w-64');
    await fireEvent.keyDown(within(container).getByLabelText('Filter chats'), { key: 'b', ctrlKey: true });
    expect(sidebar.className).toContain('w-64');
  });

  it('opens the drawer from the top bar and closes it on navigate', async () => {
    stubApp();
    const { container } = render(App, { props: { user: me } });
    await within(container).findByTestId('sidebar');
    await fireEvent.click(within(container).getByRole('button', { name: 'Sessions' }));
    expect(await screen.findAllByLabelText('Filter chats')).toHaveLength(2);
    await fireEvent.click(screen.getAllByText('Fix sidebar')[0]);
    expect(await screen.findAllByLabelText('Filter chats')).toHaveLength(1);
  });

  it('shows the creator in the Session header', async () => {
    stubApp();
    history.replaceState(null, '', '/s/s1');
    const { container } = render(App, { props: { user: me } });
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('created by cioky'));
  });
});

describe('Quota navigation', () => {
  it('opens the Quota view from the rail and fetches it once', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    const calls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        calls.push(String(u));
        if (String(u).includes('/api/quota')) return { ok: true, json: async () => ({ providers: [] }) };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App);
    await fireEvent.click(await within(container).findByLabelText('Quota'));
    expect(await within(container).findByTestId('quota-page')).toBeTruthy();
    expect(calls.filter((u) => u.includes('/api/quota')).length).toBe(1);
  });
});

describe('Account footer', () => {
  afterEach(() => cleanup());

  it('shows the signed-in User and signs out through the callback', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => (String(u).includes('/api/specs') ? { specs: [] } : []),
      })),
    );
    const onSignOut = vi.fn();
    const { container } = render(App, { props: { user: { id: 'u1', username: 'alice', admin: true }, onSignOut } });
    const account = await within(container).findByTestId('account');
    expect(account.textContent).toContain('alice');
    expect(account.textContent).toContain('Admin');
    await fireEvent.click(within(container).getByLabelText('Sign out'));
    expect(onSignOut).toHaveBeenCalledTimes(1);
  });
});
