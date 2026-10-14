import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, within, screen, fireEvent, cleanup } from '@testing-library/svelte';
import App from './App.svelte';
import { stopLive } from './lib/live.js';

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
describe('Non-blocking boot', () => {
  beforeEach(() => cleanup());
  afterEach(() => {
    cleanup();
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  const response = (body) => ({ ok: true, json: async () => body });
  const deferred = () => {
    let resolve;
    const promise = new Promise((done) => { resolve = done; });
    return { promise, resolve };
  };
  const fakeEvents = () => class {
    static instances = [];
    constructor(url) {
      this.url = String(url);
      this.constructor.instances.push(this);
    }
    close() {}
  };

  it('renders the home shell while Session and Run lists are still loading', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    const sessions = deferred();
    const runs = deferred();
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path.endsWith('/api/sessions')) return sessions.promise.then(response);
      if (path.endsWith('/api/runs')) return runs.promise.then(response);
      return Promise.resolve(response(path.endsWith('/api/specs') ? { specs: [] } : []));
    });
    vi.stubGlobal('EventSource', fakeEvents());
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    expect(await within(container).findByTestId('sidebar')).toBeTruthy();
    expect(within(container).getByTestId('sessions-loading').textContent).toContain('Loading Sessions');
    await vi.waitFor(() => {
      const paths = fetchMock.mock.calls.map(([url]) => String(url));
      expect(paths.some((path) => path.endsWith('/api/sessions'))).toBe(true);
      expect(paths.some((path) => path.endsWith('/api/runs'))).toBe(true);
    });

    expect(within(container).getByTestId('header-title').textContent).toContain('No Session');

    sessions.resolve([]);
    runs.resolve({ runs: [] });
    await vi.waitFor(() => expect(within(container).queryByTestId('sessions-loading')).toBeNull());
  });
  it('keeps a direct Session open when the Session list races ahead of its detail', async () => {
    history.replaceState(null, '', '/s/s1');
    localStorage.clear();
    const detail = deferred();
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path === '/api/sessions') return Promise.resolve(response([]));
      if (path.startsWith('/api/sessions/s1')) return detail.promise.then(response);
      return Promise.resolve(response(path.endsWith('/api/specs') ? { specs: [] } : []));
    });
    vi.stubGlobal('EventSource', fakeEvents());
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    expect(await within(container).findByTestId('sessions-loading')).toBeTruthy();
    await vi.waitFor(() => expect(within(container).queryByTestId('sessions-loading')).toBeNull());
    detail.resolve({
      session: { id: 's1', harness: 'fake', model: 'fake-model', title: 'Loaded Session' },
      events: [], resumable: true, hasMore: false, firstSeq: 0,
    });
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('Loaded Session'));
    expect(within(container).queryByTestId('session-missing')).toBeNull();
  });


  it('renders Quota before its own read finishes without fetching either list', async () => {
    history.replaceState(null, '', '/quota');
    localStorage.clear();
    const quota = deferred();
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path.endsWith('/api/quota')) return quota.promise.then(response);
      return Promise.resolve(response(path.endsWith('/api/projects') ? [] : {}));
    });
    const EventSource = fakeEvents();
    vi.stubGlobal('EventSource', EventSource);
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    const page = within(container).getByTestId('quota-page');
    expect(within(page).getByRole('heading', { name: 'Quota' })).toBeTruthy();
    expect(within(page).getByLabelText('Loading quota')).toBeTruthy();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/quota'));
    const normalStream = EventSource.instances.find((stream) => !stream.url.includes('topic=quota'));
    expect(normalStream).toBeTruthy();
    normalStream.onmessage({ data: 'hello' });
    await Promise.resolve();
    const paths = fetchMock.mock.calls.map(([url]) => String(url));
    expect(paths.some((path) => path.endsWith('/api/sessions'))).toBe(false);
    expect(paths.some((path) => path.endsWith('/api/runs'))).toBe(false);

    quota.resolve({ providers: [] });
    await vi.waitFor(() => expect(within(page).queryByLabelText('Loading quota')).toBeNull());
  });

  it('renders Runs and its loading state before the Run summary arrives', async () => {
    history.replaceState(null, '', '/runs');
    localStorage.clear();
    const runs = deferred();
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path.endsWith('/api/runs')) return runs.promise.then(response);
      return Promise.resolve(response(path.endsWith('/api/specs') ? { specs: [] } : []));
    });
    vi.stubGlobal('EventSource', fakeEvents());
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    expect(within(container).getByRole('heading', { name: 'Runs' })).toBeTruthy();
    const sidebar = await within(container).findByTestId('runs-sidebar');
    expect(within(sidebar).getByText('Loading Runs…')).toBeTruthy();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/runs'));
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/sessions'))).toBe(false);

    runs.resolve({ runs: [] });
    expect(await within(sidebar).findByText('No Runs yet.')).toBeTruthy();
  });
  it('renders all 40 Run summaries after the cold Runs shell', async () => {
    history.replaceState(null, '', '/runs');
    localStorage.clear();
    const summaries = Array.from({ length: 40 }, (_, i) => ({
      id: `boot-${i}`,
      spec: `/repo/.scratch/boot-${i}/spec.md`,
      status: 'done',
      updatedAt: `2026-10-${String(i + 1).padStart(2, '0')}T10:00:00Z`,
      counts: { total: 8, merged: 8, needsYou: 0 },
    }));
    vi.stubGlobal('EventSource', fakeEvents());
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      return response(path.endsWith('/api/runs') ? { runs: summaries } : path.endsWith('/api/specs') ? { specs: [] } : []);
    }));

    const started = performance.now();
    const { container } = render(App);
    const sidebar = await within(container).findByTestId('runs-sidebar');
    const shellMs = performance.now() - started;
    expect(await within(sidebar).findByRole('button', { name: /boot-39.*8\/8/ })).toBeTruthy();
    const listMs = performance.now() - started;
    expect(sidebar.querySelectorAll('ul button')).toHaveLength(40);
    if (process.env.EL_BOOT_BENCH === '1') {
      console.info(`COLD_BOOT_BENCH shell=${shellMs.toFixed(1)}ms rows40=${listMs.toFixed(1)}ms`);
    }
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
    expect(screen.getAllByLabelText('Filter chats').every((input) => input.classList.contains('text-base'))).toBe(true);
    await fireEvent.click(screen.getAllByText('Fix sidebar')[0]);
    expect(await screen.findAllByLabelText('Filter chats')).toHaveLength(1);
  });

  it('closes the drawer on system Back and keeps the page', async () => {
    stubApp();
    history.replaceState(null, '', '/');
    const { container } = render(App, { props: { user: me } });
    await within(container).findByTestId('sidebar');
    await new Promise((r) => setTimeout(r, 50));
    const before = window.location.pathname;
    await fireEvent.click(within(container).getByRole('button', { name: 'Sessions' }));
    expect(await screen.findByTestId('drawer-nav')).toBeTruthy();
    expect(history.state?.drawer).toBe(true);
    window.dispatchEvent(new PopStateEvent('popstate', { state: null }));
    await vi.waitFor(() => expect(screen.queryByTestId('drawer-nav')).toBeNull());
    expect(window.location.pathname).toBe(before);
  });

  it('shows Close and the area tabs in the drawer', async () => {
    stubApp();
    history.replaceState(null, '', '/');
    const { container } = render(App, { props: { user: me } });
    await within(container).findByTestId('sidebar');
    await fireEvent.click(within(container).getByRole('button', { name: 'Sessions' }));
    const nav = await screen.findByTestId('drawer-nav');
    for (const name of ['Chat', 'Runs', 'Memory', 'Quota', 'Settings']) expect(within(nav).getByRole('button', { name })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Close menu' })).toBeTruthy();
  });

  it('shows the creator in the Session header', async () => {
    stubApp();
    history.replaceState(null, '', '/s/s1');
    const { container } = render(App, { props: { user: me } });
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('created by cioky'));
  });
  it('hides the composer and context meter on session-not-found', async () => {
    stubApp();
    history.replaceState(null, '', '/s/doesnotexist123');
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        if (String(u).includes('/api/sessions/doesnotexist123'))
          return { ok: false, status: 404, json: async () => ({}) };
        if (String(u).includes('/api/sessions')) return { ok: true, json: async () => sessionsBody };
        if (String(u).includes('/api/projects')) return { ok: true, json: async () => projectsBody };
        if (String(u).includes('/api/users')) return { ok: true, json: async () => usersBody };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App, { props: { user: me } });
    expect(await within(container).findByTestId('session-missing')).toBeTruthy();
    expect(within(container).queryByLabelText('composer')).toBeNull();
    expect(within(container).queryByTestId('context-meter')).toBeNull();
  });
  it('closes the Git panel on Escape and returns focus to its toggle', async () => {
    stubApp();
    history.replaceState(null, '', '/s/s1');
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        if (String(u).includes('/git')) return { ok: true, json: async () => ({ isRepo: true, branch: 'main', files: [], log: [] }) };
        if (String(u).includes('/api/sessions/s1')) return { ok: true, json: async () => ({ session: sessionsBody[0], events: [], resumable: true, hasMore: false, firstSeq: 0 }) };
        if (String(u).includes('/api/sessions')) return { ok: true, json: async () => sessionsBody };
        if (String(u).includes('/api/projects')) return { ok: true, json: async () => projectsBody };
        if (String(u).includes('/api/users')) return { ok: true, json: async () => usersBody };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App, { props: { user: me } });
    await fireEvent.click(await within(container).findByRole('button', { name: /Git|main/ }));
    expect(await within(container).findByLabelText('Git')).toBeTruthy();
    await fireEvent.keyDown(window, { key: 'Escape' });
    await vi.waitFor(() => expect(within(container).queryByLabelText('Git')).toBeNull());
    expect(document.getElementById('git-toggle')).toBe(document.activeElement);
  });
});

describe('Quota navigation', () => {
  afterEach(() => {
    cleanup();
    stopLive();
  });

  it('shares one live subscription across the page, desktop sidebar and phone drawer', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    const streams = [];
    vi.stubGlobal('EventSource', class {
      constructor(url) {
        this.url = String(url);
        this.close = vi.fn();
        streams.push(this);
      }
    });
    let liveProviders = [
      { key: 'claude', label: 'Claude', windows: [{ label: '5-hour', pct: 36 }, { label: 'Weekly', pct: 8 }] },
      { key: 'openrouter', label: 'OpenRouter', windows: [{ label: 'Credits', pct: 94, detail: '$59.16 of $63.00 · $3.84 left · today $0.91 · week $2.37 · month $5.70' }] },
      { key: 'agy-gemini', label: 'Antigravity Gemini', windows: [] },
    ];
    const calls = [];
    vi.stubGlobal('fetch', vi.fn(async (u, opts = {}) => {
      const url = String(u);
      const method = opts.method ?? 'GET';
      calls.push({ url, method, body: opts.body ? JSON.parse(opts.body) : null });
      if (url.includes('/api/quota')) return { ok: true, json: async () => ({ providers: liveProviders }) };
      return { ok: true, json: async () => [] };
    }));

    const { container } = render(App);
    await fireEvent.click(await within(container).findByLabelText('Quota and Providers'));
    expect(await within(container).findByTestId('quota-page')).toBeTruthy();
    await within(container).findAllByTestId('quota-card');
    const sidebar = within(container).getByTestId('sidebar');
    expect(sidebar.className).toContain('w-64');
    const quotaSidebar = within(sidebar).getByTestId('quota-sidebar');
    expect(within(quotaSidebar).getByRole('heading', { name: 'Quota and Providers' })).toBeTruthy();
    expect(within(quotaSidebar).getByRole('heading', { name: 'Live status' })).toBeTruthy();
    const rows = await within(quotaSidebar).findAllByTestId('quota-status-row');
    expect(rows.map((row) => row.textContent.replace(/\s+/g, ' ').trim())).toEqual([
      'Claude 36%',
      'OpenRouter 94%',
      'Antigravity Gemini —',
    ]);
    expect(within(container).getAllByText('94%')).toHaveLength(2);

    await fireEvent.click(within(container).getByRole('button', { name: 'Sessions' }));
    const drawer = await screen.findByTestId('drawer-scroll');
    const drawerQuotaSidebar = within(drawer).getByTestId('quota-sidebar');
    const drawerRows = within(drawerQuotaSidebar).getAllByTestId('quota-status-row');
    expect(drawerRows.map((row) => row.textContent.replace(/\s+/g, ' ').trim())).toEqual(rows.map((row) => row.textContent.replace(/\s+/g, ' ').trim()));
    expect(drawer.className).toContain('overflow-y-auto');
    const drawerNav = within(drawerQuotaSidebar).getByRole('navigation', { name: 'Quota and Providers navigation' });
    expect(drawerNav.className).toContain('overflow-visible');
    expect(drawerNav.className).not.toContain('overflow-auto');
    expect(within(drawerNav).getByRole('link', { name: 'Quota' }).className).toContain('max-md:min-h-11');
    const areaTabs = screen.getByTestId('drawer-nav');
    for (const tab of within(areaTabs).getAllByRole('button')) expect(tab.className).toContain('min-h-14');
    expect(screen.getByRole('button', { name: 'Close menu' }).className).toContain('max-md:size-11');

    const quotaStreams = streams.filter((stream) => stream.url.includes('topic=quota'));
    expect(quotaStreams).toHaveLength(1);
    liveProviders = liveProviders.map((provider) => (
      provider.key === 'claude'
        ? { ...provider, windows: [{ ...provider.windows[0], pct: 82 }, ...provider.windows.slice(1)] }
        : provider
    ));
    quotaStreams[0].onmessage({ data: JSON.stringify({ topic: 'quota', providers: liveProviders }) });

    const claudeCard = container.querySelector('[data-testid="quota-card"][data-key="claude"]');
    const desktopClaudeRow = rows.find((row) => row.dataset.key === 'claude');
    const drawerClaudeRow = drawerRows.find((row) => row.dataset.key === 'claude');
    await vi.waitFor(() => {
      expect(within(claudeCard).getByRole('progressbar', { name: 'Claude 5-hour' }).getAttribute('aria-valuenow')).toBe('82');
      expect(within(desktopClaudeRow).getByTestId('quota-status-percent').textContent.trim()).toBe('82%');
      expect(within(drawerClaudeRow).getByTestId('quota-status-percent').textContent.trim()).toBe('82%');
    });
    expect(calls.filter((call) => call.method === 'GET' && call.url.endsWith('/api/quota'))).toHaveLength(1);

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh Claude' }));
    await vi.waitFor(() => expect(calls.some((call) => call.method === 'POST')).toBe(true));
    expect(calls.find((call) => call.method === 'POST')).toMatchObject({ url: '/api/quota/refresh', body: { key: 'claude' } });
  });
});

describe('Memory navigation', () => {
  it('opens the Memory view from the rail', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        if (String(u).includes('/api/memory/status'))
          return { ok: true, json: async () => ({ available: true, queue: { pending: 0 }, ledgerSize: 0 }) };
        if (String(u).includes('/api/profile')) return { ok: true, json: async () => ({ version: 0, lines: [], rendered: '' }) };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App);
    await fireEvent.click(await within(container).findByLabelText('Memory'));
    expect(await within(container).findByTestId('memory-page')).toBeTruthy();
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

describe('Run summaries in the Session sidebar', () => {
  afterEach(() => cleanup());

  it('uses summary counts and Project ids without fetching Run details', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    const fetchMock = vi.fn(async (url) => ({
      ok: true,
      json: async () => {
        const path = String(url);
        if (path.endsWith('/api/runs')) {
          return {
            runs: [{
              id: 'run-summary', spec: '/repo/.scratch/demo/spec.md', status: 'needs-you',
              repo: '/repo/subdir', projectId: 'project-1', owner: 'u1', updatedAt: '2026-10-14T10:00:00Z',
              counts: { total: 2, merged: 0, needsYou: 1 },
            }],
          };
        }
        if (path.endsWith('/api/projects')) return [{ id: 'project-1', name: 'Demo', path: '/repo' }];
        if (path.endsWith('/api/users')) return [{ id: 'u1', username: 'alice', admin: true, online: true }];
        return [];
      },
    }));
    vi.stubGlobal('fetch', fetchMock);
    const { container } = render(App, { props: { user: { id: 'u1', username: 'alice', admin: true } } });
    const runRow = await within(container).findByRole('button', { name: /demo.*Run/i });
    expect(runRow.className).toContain('border-amber-500/30');
		expect(runRow.closest('section[aria-label="Demo"]')).toBeTruthy();
    expect(fetchMock.mock.calls.some(([url]) => /\/api\/runs\/[^/]+$/.test(String(url)))).toBe(false);
  });
});

describe('Coalesced Run summaries', () => {
  it('shares the sidebar list request and coalesces Run-event bursts', async () => {
    history.replaceState(null, '', '/runs');
    localStorage.clear();
    let stream;
    class FakeEventSource {
      constructor(url) {
        this.url = url;
        stream = this;
      }
      close() {}
      emit(topic) {
        this.onmessage?.({ data: topic });
      }
    }
    vi.stubGlobal('EventSource', FakeEventSource);

    const runSummary = {
      id: 'shared-run', spec: '/repo/.scratch/shared/spec.md', status: 'running',
      updatedAt: '2026-10-14T10:00:00Z', counts: { total: 1, merged: 0, needsYou: 0 },
    };
    const runListCalls = [];
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      if (path.endsWith('/api/runs')) {
        runListCalls.push(path);
        return { ok: true, json: async () => ({ runs: [runSummary] }) };
      }
      return { ok: true, json: async () => [] };
    }));

    const { container, unmount } = render(App);
    const sidebar = await within(container).findByTestId('runs-sidebar');
    expect(await within(sidebar).findByRole('button', { name: /shared.*0\/1/ })).toBeTruthy();
    expect(runListCalls).toHaveLength(1);

    stream.emit('runs');
    stream.emit('runs');
    stream.emit('runs');
    await vi.waitFor(() => expect(runListCalls).toHaveLength(2));
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(runListCalls).toHaveLength(2);
    unmount();
  });
});
