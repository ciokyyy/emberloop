import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, within, screen, fireEvent, cleanup } from '@testing-library/svelte';
import App from './App.svelte';
import { resetDiscovery } from './lib/runsDiscovery.svelte.js';
import { stopLive } from './lib/live.js';
import { readReportState } from './lib/reportState.js';

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

describe('Providers route', () => {
  it('opens /providers inside the Quota sidebar area and navigates back to Quota', async () => {
    history.replaceState(null, '', '/providers');
    vi.stubGlobal('EventSource', class {
      constructor() {}
      close() {}
    });
    vi.stubGlobal('fetch', vi.fn(async (input) => {
      const path = String(input);
      if (path.endsWith('/api/providers')) return { ok: true, json: async () => ({ providers: [] }) };
      if (path.endsWith('/api/quota')) return { ok: true, json: async () => ({ providers: [] }) };
      if (path.endsWith('/api/users')) return { ok: true, json: async () => [] };
      return { ok: true, json: async () => [] };
    }));
    const { container } = render(App, { props: { user: { username: 'admin', admin: true } } });
    try {
      expect(await within(container).findByRole('heading', { name: 'Providers' })).toBeTruthy();
      const nav = within(container).getByRole('navigation', { name: 'Quota and Providers navigation' });
      const providerLink = within(nav).getByRole('link', { name: 'Providers' });
      expect(providerLink.getAttribute('aria-current')).toBe('page');
      await fireEvent.click(within(nav).getByRole('link', { name: 'Quota' }));
      expect(await within(container).findByRole('heading', { name: 'Quota' })).toBeTruthy();
      expect(window.location.pathname).toBe('/quota');
    } finally {
      cleanup();
      stopLive();
      vi.unstubAllGlobals();
      history.replaceState(null, '', '/');
    }
  });
});
describe('Project scope routes', () => {
  afterEach(() => {
    cleanup();
    stopLive();
    vi.unstubAllGlobals();
    localStorage.clear();
    history.replaceState(null, '', '/');
    resetDiscovery();
  });

  const response = (body) => ({ ok: true, json: async () => body });
  class FakeEventSource {
    constructor() {}
    close() {}
  }

  it('derives Project scope from the canonical Session URL', async () => {
    history.replaceState(null, '', '/s/session-one');
    vi.stubGlobal('EventSource', FakeEventSource);
    const sessions = [
      { id: 'session-one', title: 'Project Session', projectId: 'project-one', owner: 'u1', status: 'idle' },
      { id: 'session-two', title: 'Other Project Session', projectId: 'project-two', owner: 'u1', status: 'idle' },
    ];
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      if (path.endsWith('/api/projects')) return response([
        { id: 'project-one', name: 'Renamed Project', path: '/one' },
        { id: 'project-two', name: 'Other Project', path: '/two' },
      ]);
      if (path.endsWith('/api/sessions')) return response(sessions);
      if (path.startsWith('/api/sessions/session-one?')) return response({
        session: sessions[0], events: [], resumable: true, hasMore: false, firstSeq: 0,
      });
      if (path.endsWith('/api/runs')) return response({ runs: [] });
      if (path.endsWith('/api/users')) return response([{ id: 'u1', username: 'alice' }]);
      if (path.endsWith('/queue')) return response({ queue: [] });
      return response({});
    }));

    const { container } = render(App, { props: { user: { id: 'u1', username: 'alice' } } });
    expect(await within(container).findAllByText('Project Session')).toHaveLength(2);
    expect(within(container).queryByText('Other Project Session')).toBeNull();
    expect(within(container).getByRole('button', { name: 'Project scope' }).textContent).toContain('Renamed Project');
    expect(window.location.pathname).toBe('/s/session-one');
  });

  it('shows the Dashboard and its Project switcher in the phone drawer', async () => {
    history.replaceState(null, '', '/');
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      if (path.endsWith('/api/dashboard')) return response({ groups: {} });
      if (path.endsWith('/api/projects')) return response([{ id: 'project-one', name: 'Demo', path: '/repo' }]);
      return response([]);
    }));

    const { container } = render(App);
    expect(await within(container).findByTestId('dashboard')).toBeTruthy();
    await fireEvent.click(within(container).getByRole('button', { name: 'Project switcher' }));
    const drawer = await screen.findByTestId('drawer-scroll');
    expect(within(drawer).getByTestId('project-switcher')).toBeTruthy();
    expect(within(drawer).getByRole('button', { name: 'Project scope' }).textContent).toContain('All projects');

    await fireEvent.click(within(drawer).getByRole('button', { name: 'Project scope' }));
    await fireEvent.click(await screen.findByRole('option', { name: 'Demo' }));
    await vi.waitFor(() => expect(window.location.pathname).toBe('/p/project-one'));
    expect(screen.queryByTestId('drawer-scroll')).toBeNull();
  });

  it('keeps home-directory Sessions in the built-in No project scope', async () => {
    history.replaceState(null, '', '/p/none');
    vi.stubGlobal('EventSource', FakeEventSource);
    const sessions = [
      { id: 'home-session', title: 'Home Session', owner: 'u1', status: 'idle' },
      { id: 'project-session', title: 'Project Session', projectId: 'project-one', owner: 'u1', status: 'idle' },
    ];
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      if (path.endsWith('/api/projects')) return response([{ id: 'project-one', name: 'Demo', path: '/repo' }]);
      if (path.endsWith('/api/sessions')) return response(sessions);
      if (path.endsWith('/api/runs')) return response({ runs: [] });
      if (path.endsWith('/api/users')) return response([{ id: 'u1', username: 'alice' }]);
      return response({});
    }));

    const { container } = render(App, { props: { user: { id: 'u1', username: 'alice' } } });
    expect(await within(container).findByText('Home Session')).toBeTruthy();
    expect(within(container).queryByText('Project Session')).toBeNull();
    expect(within(container).getByRole('button', { name: 'Project scope' }).textContent).toContain('No project');
  });
  it('filters Project Runs, discovers specs in its folder, and starts without a folder input', async () => {
    history.replaceState(null, '', '/p/project-one/runs');
    vi.stubGlobal('EventSource', FakeEventSource);
    const starts = [];
    const startedRun = {
      id: 'run-started', spec: '/one/.scratch/one/spec.md', repo: '/one',
      projectId: 'project-one', status: 'running', tickets: [],
    };
    vi.stubGlobal('fetch', vi.fn(async (url, opts = {}) => {
      const path = String(url);
      if (path.endsWith('/api/projects')) return response([
        { id: 'project-one', name: 'Renamed Project', path: '/one' },
        { id: 'project-two', name: 'Other Project', path: '/two' },
      ]);
      if (path.endsWith('/api/runs') && opts.method === 'POST') {
        starts.push(JSON.parse(opts.body));
        return response({ run: startedRun });
      }
      if (path.endsWith('/api/runs/run-started')) return response({ run: startedRun });
      if (path.endsWith('/api/runs')) return response({ runs: [
        { id: 'run-one', spec: '/one/.scratch/one/spec.md', projectId: 'project-one', updatedAt: '2026-10-14T10:00:00Z', counts: { total: 1 } },
        { id: 'run-two', spec: '/two/.scratch/two/spec.md', projectId: 'project-two', updatedAt: '2026-10-14T09:00:00Z', counts: { total: 1 } },
      ] });
      if (path.includes('/api/specs/preview')) return response({
        spec: '/one/.scratch/one/spec.md', title: 'One spec',
        tickets: [{ id: 'one', num: 1, title: 'First ticket', blockers: [], done: 0, total: 1 }],
      });
      if (path.startsWith('/api/specs?')) return response({ specs: [
        { spec: '/one/.scratch/one/spec.md', title: 'One spec', tickets: 1 },
      ] });
      return response([]);
    }));

    const { container } = render(App);
    const sidebar = await within(container).findByTestId('runs-sidebar');
    expect(await within(sidebar).findByRole('button', { name: /one.*0\/1/ })).toBeTruthy();
    expect(within(sidebar).queryByRole('button', { name: /two.*0\/1/ })).toBeNull();
    expect(within(sidebar).queryByLabelText(/repo directory/i)).toBeNull();
    expect(await within(sidebar).findByText('Specs in /one')).toBeTruthy();
    await fireEvent.click(await within(sidebar).findByRole('button', { name: /one spec/i }));
    await fireEvent.click(await within(container).findByRole('button', { name: 'Start Run' }));
    expect(starts).toEqual([{ spec: '/one/.scratch/one/spec.md', repo: '/one' }]);
    await vi.waitFor(() => expect(window.location.pathname).toBe('/p/project-one/runs/run-started'));
  });

  it('edits and removes a Project from its settings route', async () => {
    history.replaceState(null, '', '/p/project-one/settings');
    vi.stubGlobal('EventSource', FakeEventSource);
    const calls = [];
    let project = { id: 'project-one', name: 'Demo', path: '/repo', gist: 'Original gist.' };
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    vi.stubGlobal('fetch', vi.fn(async (url, opts = {}) => {
      const path = String(url);
      calls.push({ path, method: opts.method ?? 'GET', body: opts.body ? JSON.parse(opts.body) : null });
      if (path.endsWith('/api/projects') && opts.method !== 'PATCH') return response(project ? [project] : []);
      if (path.endsWith('/api/projects/project-one') && opts.method === 'PATCH') {
        project = { ...project, ...JSON.parse(opts.body) };
        return response(project);
      }
      if (path.endsWith('/api/projects/project-one') && opts.method === 'DELETE') {
        project = null;
        return response({ ok: true });
      }
      if (path.endsWith('/api/dashboard')) return response({ groups: {} });
      return response([]);
    }));

    try {
      const { container } = render(App);
      expect(await within(container).findByRole('heading', { level: 1, name: 'Project settings' })).toBeTruthy();
      await fireEvent.input(await within(container).findByRole('textbox', { name: 'Project name' }), { target: { value: 'Renamed' } });
      await fireEvent.click(within(container).getByRole('button', { name: 'Save name' }));
      await vi.waitFor(() => expect(calls).toContainEqual({ path: '/api/projects/project-one', method: 'PATCH', body: { name: 'Renamed' } }));

      await fireEvent.input(within(container).getByRole('textbox', { name: 'Project gist' }), { target: { value: 'Edited gist.' } });
      await fireEvent.click(within(container).getByRole('button', { name: 'Save gist' }));
      await vi.waitFor(() => expect(calls).toContainEqual({ path: '/api/projects/project-one', method: 'PATCH', body: { gist: 'Edited gist.' } }));
      expect(within(container).queryByText(/override/i)).toBeNull();

      await fireEvent.click(within(container).getByRole('button', { name: 'Remove project' }));
      await vi.waitFor(() => expect(calls).toContainEqual({ path: '/api/projects/project-one', method: 'DELETE', body: null }));
      await vi.waitFor(() => expect(window.location.pathname).toBe('/'));
      expect(confirm).toHaveBeenCalledOnce();
    } finally {
      confirm.mockRestore();
    }
  });
});

describe('Draft Project pick', () => {
  afterEach(() => {
    cleanup();
    stopLive();
    vi.unstubAllGlobals();
    localStorage.clear();
    history.replaceState(null, '', '/');
  });

  const response = (body) => ({ ok: true, json: async () => body });
  const project = { id: 'project-1', name: 'Demo', path: '/repo' };
  const session = { id: 'draft-session', harness: 'claude', model: 'sonnet', projectId: 'project-1', workDir: '/repo', status: 'idle' };

  function setup(suggestedProject, pendingSuggestion = null, failFirstPrompt = false) {
    history.replaceState(null, '', '/p/none');
    localStorage.clear();
    vi.stubGlobal('EventSource', class { close() {} });
    const createBodies = [];
    const promptBodies = [];
    let createdSession = session;
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = String(url);
      const method = options.method ?? 'GET';
      if (path.endsWith('/api/projects/suggest')) return pendingSuggestion ?? response({ suggestedProject });
      if (path.endsWith('/api/projects')) return response([project]);
      if (path === '/api/sessions' && method === 'POST') {
        const body = JSON.parse(options.body);
        createBodies.push(body);
        createdSession = { ...session, projectId: body.projectId ?? '', workDir: body.projectId ? '/repo' : '/home/alice' };
        return response(createdSession);
      }
      if (path === '/api/sessions') return response([]);
      if (path.startsWith('/api/sessions/draft-session?')) return response({ session: createdSession, events: [], resumable: true, hasMore: false, firstSeq: 0 });
      if (path.endsWith('/api/sessions/draft-session/prompt')) {
        promptBodies.push(JSON.parse(options.body));
        if (failFirstPrompt) {
          failFirstPrompt = false;
          return { ok: false, status: 500, json: async () => ({}) };
        }
        return response({ seq: 1 });
      }
      if (path.endsWith('/api/sessions/draft-session/queue')) return response({ queue: [] });
      if (path.endsWith('/api/runs')) return response({ runs: [] });
      if (path.endsWith('/api/users')) return response([]);
      if (path.endsWith('/api/projects/project-1/git')) return response({});
      return response([]);
    });
    vi.stubGlobal('fetch', fetchMock);
    return { ...render(App), createBodies, promptBodies, fetchMock };
  }

  async function sendFirstPrompt(container) {
    const message = await within(container).findByRole('textbox', { name: 'message' });
    await fireEvent.input(message, { target: { value: 'Fix the Demo startup flow.' } });
    await fireEvent.click(within(container).getByRole('button', { name: 'Send' }));
  }

  it('sends the held prompt if navigation closes its draft during the Jev request', async () => {
    let finishSuggestion;
    const pendingSuggestion = new Promise((resolve) => { finishSuggestion = resolve; });
    const { container, createBodies, promptBodies, fetchMock } = setup(project, pendingSuggestion);
    await sendFirstPrompt(container);
    await vi.waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/projects/suggest'))).toBe(true));

    history.replaceState(null, '', '/s/other-session');
    window.dispatchEvent(new PopStateEvent('popstate'));
    finishSuggestion(response({ suggestedProject: project }));

    await vi.waitFor(() => expect(promptBodies).toHaveLength(1));
    expect(createBodies[0].projectId).toBeNull();
    expect(promptBodies[0]).toEqual({ text: 'Fix the Demo startup flow.' });
  });

  it('shows the Daemon suggestion and Yes starts the Session and prompt in that Project', async () => {
    const { container, createBodies, promptBodies, fetchMock } = setup(project);
    await sendFirstPrompt(container);

    const banner = await within(container).findByTestId('project-pick-banner');
    expect(banner.textContent).toContain('Run this in Demo?');
    expect(createBodies).toHaveLength(0);
    expect(promptBodies).toHaveLength(0);
    const suggestCall = fetchMock.mock.calls.find(([url]) => String(url).endsWith('/api/projects/suggest'));
    expect(JSON.parse(suggestCall[1].body)).toEqual({ prompt: 'Fix the Demo startup flow.' });

    await fireEvent.click(within(banner).getByRole('button', { name: 'Yes' }));
    await vi.waitFor(() => expect(promptBodies).toHaveLength(1));
    expect(createBodies[0].projectId).toBe('project-1');
    expect(promptBodies[0]).toEqual({ text: 'Fix the Demo startup flow.' });
  });

  it('keeps a held prompt available to retry if the confirmed Session rejects it', async () => {
    const { container, createBodies, promptBodies } = setup(project, null, true);
    await sendFirstPrompt(container);
    const banner = await within(container).findByTestId('project-pick-banner');
    await fireEvent.click(within(banner).getByRole('button', { name: 'Yes' }));

    const retry = await within(container).findByTestId('failed-draft-prompt');
    expect(createBodies).toHaveLength(1);
    expect(promptBodies).toHaveLength(1);
    await fireEvent.click(within(retry).getByRole('button', { name: 'Retry first prompt' }));
    await vi.waitFor(() => expect(promptBodies).toHaveLength(2));
    expect(createBodies).toHaveLength(1);
    expect(promptBodies[0]).toEqual(promptBodies[1]);
  });

  it('No keeps the existing unprojected start behavior, while no suggestion shows no banner', async () => {
    const selected = setup(project);
    await sendFirstPrompt(selected.container);
    const banner = await within(selected.container).findByTestId('project-pick-banner');
    await fireEvent.click(within(banner).getByRole('button', { name: 'No' }));
    await vi.waitFor(() => expect(selected.promptBodies).toHaveLength(1));
    expect(selected.createBodies[0].projectId).toBeNull();

    cleanup();
    const ordinary = setup(null);
    await sendFirstPrompt(ordinary.container);
    await vi.waitFor(() => expect(ordinary.promptBodies).toHaveLength(1));
    expect(within(ordinary.container).queryByTestId('project-pick-banner')).toBeNull();
    expect(ordinary.createBodies[0].projectId).toBeNull();
  });
  it('starts an image-only draft without requesting a Project suggestion', async () => {
    const { container, createBodies, promptBodies, fetchMock } = setup(project);
    await within(container).findByRole('textbox', { name: 'message' });
    const image = new File([Uint8Array.of(1, 2, 3)], 'diagram.png', { type: 'image/png' });
    await fireEvent.change(container.querySelector('input[name="attachments"]'), { target: { files: [image] } });
    await within(container).findByTestId('composer-tray');
    await fireEvent.click(within(container).getByRole('button', { name: 'Send' }));

    await vi.waitFor(() => expect(promptBodies).toHaveLength(1));
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/projects/suggest'))).toBe(false);
    expect(createBodies[0].projectId).toBeNull();
    expect(promptBodies[0]).toMatchObject({ text: '', mode: 'prompt', images: [{ mime: 'image/png' }] });
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

  it('renders the Dashboard while its payload loads without waiting for Session or Run lists', async () => {
    history.replaceState(null, '', '/');
    localStorage.clear();
    const dashboard = deferred();
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path.endsWith('/api/dashboard')) return dashboard.promise.then(response);
      return Promise.resolve(response([]));
    });
    vi.stubGlobal('EventSource', fakeEvents());
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    expect(await within(container).findByTestId('dashboard')).toBeTruthy();
    expect(within(container).getByRole('status', { name: 'Loading Dashboard' })).toBeTruthy();
    await vi.waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/dashboard'))).toBe(true));
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/sessions'))).toBe(false);
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/runs'))).toBe(false);

    dashboard.resolve({ groups: {} });
    expect(await within(container).findByText(/Nothing needs attention/)).toBeTruthy();
  });
  it('refreshes Sessions changed while away before resolving the Project scope', async () => {
    history.replaceState(null, '', '/p/none');
    localStorage.clear();
    const refreshedSessions = deferred();
    let sessionReads = 0;
    const priorSession = { id: 's1', harness: 'fake', model: 'fake-model', title: 'Prior Session', status: 'idle' };
    const newSession = { id: 's2', harness: 'fake', model: 'fake-model', title: 'Fresh Session', status: 'idle' };
    const fetchMock = vi.fn((url) => {
      const path = String(url);
      if (path === '/api/sessions') {
        sessionReads += 1;
        if (sessionReads === 1) return Promise.resolve(response([priorSession]));
        if (sessionReads === 2) return refreshedSessions.promise.then(response);
        return Promise.resolve(response([newSession]));
      }
      if (path.startsWith('/api/sessions/s1')) {
        return Promise.resolve(response({ session: priorSession, events: [], resumable: true, hasMore: false, firstSeq: 0 }));
      }
      if (path.startsWith('/api/sessions/s2')) {
        return Promise.resolve(response({ session: newSession, events: [], resumable: true, hasMore: false, firstSeq: 0 }));
      }
      if (path.endsWith('/api/runs')) return Promise.resolve(response({ runs: [] }));
      if (path.endsWith('/api/specs')) return Promise.resolve(response({ specs: [] }));
      if (path.includes('/queue')) return Promise.resolve(response({ queue: [] }));
      return Promise.resolve(response([]));
    });
    const EventSource = fakeEvents();
    vi.stubGlobal('EventSource', EventSource);
    vi.stubGlobal('fetch', fetchMock);

    const { container } = render(App);
    await vi.waitFor(() => expect(sessionReads).toBe(1));
    await vi.waitFor(() => expect(within(container).queryByTestId('sessions-loading')).toBeNull());
    await fireEvent.click(within(container).getByLabelText('Specs and Runs'));
    const normalStream = EventSource.instances.find((stream) => !stream.url.includes('topic=quota'));
    normalStream.onmessage({ data: 'sessions' });
    expect(sessionReads).toBe(1);

    await fireEvent.click(within(container).getByLabelText('Chat'));
    await vi.waitFor(() => expect(sessionReads).toBe(2));
    expect(within(container).getByTestId('header-title').textContent).toContain('Prior Session');
    expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith('/api/sessions/s2'))).toBe(false);

    refreshedSessions.resolve([newSession]);
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('Fresh Session'));
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
    await vi.waitFor(() => expect(within(container).getByTestId('sessions-loading').textContent).toContain('Loading Project scope'));
    detail.resolve({
      session: { id: 's1', harness: 'fake', model: 'fake-model', title: 'Loaded Session' },
      events: [], resumable: true, hasMore: false, firstSeq: 0,
    });
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('Loaded Session'));
    await vi.waitFor(() => expect(within(container).queryByTestId('sessions-loading')).toBeNull());
    expect(within(container).queryByTestId('session-missing')).toBeNull();
  });

  it('marks a live report read when it arrives in the open Session', async () => {
    history.replaceState(null, '', '/s/s1');
    localStorage.clear();
    const session = { id: 's1', harness: 'fake', model: 'fake-model', title: 'Session A', projectId: 'p1', owner: 'u1', status: 'idle' };
    const project = { id: 'p1', name: 'Demo', path: '/repo' };
    const EventSource = fakeEvents();
    vi.stubGlobal('EventSource', EventSource);
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      if (path === '/api/sessions') return response([session]);
      if (path === '/api/projects') return response([project]);
      if (path.startsWith('/api/sessions/s1?')) return response({ session, events: [], reports: [], resumable: true, hasMore: false, firstSeq: 0 });
      if (path === '/api/sessions/s1/queue') return response({ queue: [] });
      if (path.endsWith('/api/runs')) return response({ runs: [] });
      if (path.endsWith('/api/specs')) return response({ specs: [] });
      return response([]);
    }));

    const { container } = render(App, { props: { user: { id: 'u1', username: 'Alice' } } });
    await within(container).findByRole('button', { name: /Session A/ });
    const stream = EventSource.instances.find((item) => item.url.includes('/api/sessions/s1/events'));
    expect(stream).toBeTruthy();
    stream.onmessage({ data: JSON.stringify({ seq: 1, kind: 'prompt', mode: 'steer', text: 'Subagents group-1 finished:\n- child-1 (worker): completed' }) });

    await vi.waitFor(() => expect(readReportState('u1', 's1').readSeq).toBe(1));
    expect(readReportState('u1', 's1').reports).toHaveLength(1);
    expect(container.querySelector('[data-testid="report-unread"]')).toBeNull();
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
    expect(within(container).getByRole('heading', { level: 1, name: 'Runs' })).toBeTruthy();
    const sidebar = await within(container).findByTestId('runs-sidebar');
    expect(within(sidebar).getByRole('status', { name: 'Loading Runs' })).toBeTruthy();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/runs'));
    expect(fetchMock.mock.calls.some(([url]) => String(url).endsWith('/api/sessions'))).toBe(false);

    runs.resolve({ runs: [] });
    expect(await within(sidebar).findByText('No Runs yet')).toBeTruthy();
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
    history.replaceState(null, '', '/p/none');
    const { container } = render(App);
    expect(await within(container).findByTestId('live')).toBeTruthy();
    expect(within(container).queryByText('Live')).toBeNull();
    await vi.waitFor(() => expect(within(container).getByTestId('header-title').textContent).toContain('New chat'));
  });
});

describe('Sidebar collapse', () => {
  const sessionsBody = [{ id: 's1', harness: 'fake', model: 'fake-model', title: 'Fix sidebar', status: 'idle', projectId: 'p1', owner: 'u1' }];
  const usersBody = [{ id: 'u1', username: 'cioky', admin: true, online: true }];
  const me = { id: 'u1', username: 'cioky', admin: true };
  const projectsBody = [{ id: 'p1', name: 'demo', path: '/tmp/el-e2e/repo' }];
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });
  function stubApp() {
    // Each render boots from the URL: reset it, or a leaked /runs from an
    // earlier test opens the Runs view instead of the chat sidebar.
    history.replaceState(null, '', '/p/p1');
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
    history.replaceState(null, '', '/p/p1');
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
    history.replaceState(null, '', '/p/p1');
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
  it('keeps the chat header to icon controls: status glyph, context ring, Git and the Session menu', async () => {
    stubApp();
    history.replaceState(null, '', '/s/s1');
    vi.stubGlobal('EventSource', class { close() {} });
    const usage = { seq: 9, kind: 'usage', text: JSON.stringify({ used: 150000, size: 1000000 }) };
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        if (String(u).includes('/git')) return { ok: true, json: async () => ({ isRepo: true, branch: 'main', files: ['a', 'b'], log: [] }) };
        if (String(u).includes('/api/sessions/s1')) return { ok: true, json: async () => ({ session: sessionsBody[0], events: [usage], resumable: true, hasMore: false, firstSeq: 0 }) };
        if (String(u).includes('/api/sessions')) return { ok: true, json: async () => sessionsBody };
        if (String(u).includes('/api/projects')) return { ok: true, json: async () => projectsBody };
        if (String(u).includes('/api/users')) return { ok: true, json: async () => usersBody };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App, { props: { user: me } });
    const header = await vi.waitFor(() => {
      const el = container.querySelector('[data-page-header]');
      expect(el?.querySelector('[data-testid="context-meter"]')).toBeTruthy();
      return el;
    });
    expect(within(header).getByRole('status', { name: 'Live' })).toBeTruthy();
    expect(await within(header).findByRole('button', { name: 'Context 15% used' })).toBeTruthy();
    // New Session lives in the phone Session menu, which stays icon-only too.
    expect(within(header).getByRole('button', { name: 'Session menu' })).toBeTruthy();
    const git = await within(header).findByRole('button', { name: 'Git: main, 2 changed' });
    expect(git.textContent.trim()).toBe('');
    expect(header.textContent).not.toMatch(/% ctx/);
    for (const b of header.querySelectorAll('button')) expect(b.textContent.trim(), b.getAttribute('aria-label')).toBe('');
  });
  it('opens the phone Session menu from the chat header and closes it with system Back', async () => {
    // The phone layout is what puts the sheet on the back stack.
    vi.stubGlobal('matchMedia', (query) => ({ matches: query === '(max-width: 767px)', media: query, addEventListener() {}, removeEventListener() {} }));
    stubApp();
    vi.stubGlobal('EventSource', class { close() {} });
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        if (String(u).includes('/queue')) return { ok: true, json: async () => ({ queue: [] }) };
        if (String(u).includes('/git')) return { ok: true, json: async () => ({ isRepo: true, branch: 'main', files: [] }) };
        if (String(u).includes('/api/sessions/s1')) return { ok: true, json: async () => ({ session: sessionsBody[0], events: [], resumable: true, hasMore: false, firstSeq: 0 }) };
        if (String(u).includes('/api/sessions')) return { ok: true, json: async () => sessionsBody };
        if (String(u).includes('/api/projects')) return { ok: true, json: async () => projectsBody };
        if (String(u).includes('/api/users')) return { ok: true, json: async () => usersBody };
        return { ok: true, json: async () => [] };
      }),
    );
    const { container } = render(App, { props: { user: me } });
    const header = await vi.waitFor(() => {
      const el = container.querySelector('[data-page-header]');
      expect(el?.querySelector('[data-testid="chat-menu"]')).toBeTruthy();
      return el;
    });
    await fireEvent.click(within(header).getByRole('button', { name: 'Session menu' }));
    const sheet = await screen.findByRole('dialog', { name: 'Session' });
    expect(sheet.textContent).toContain('New Session');
    await vi.waitFor(() => expect(sheet.textContent).toContain('demo'));

    await fireEvent.click(within(sheet).getByRole('button', { name: /Switch Project/ }));
    expect(await screen.findByRole('dialog', { name: 'Project' })).toBeTruthy();
    await fireEvent.click(screen.getByRole('button', { name: 'Back to Session' }));
    expect(await screen.findByRole('dialog', { name: 'Session' })).toBeTruthy();

    window.dispatchEvent(new PopStateEvent('popstate'));
    await vi.waitFor(() => expect(screen.queryByRole('dialog', { name: 'Session' })).toBeNull());
  });
  it('keeps the composer dock above the keyboard and the home indicator', async () => {
    stubApp();
    history.replaceState(null, '', '/s/s1');
    const { container } = render(App, { props: { user: me } });
    const dock = await vi.waitFor(() => {
      const el = container.querySelector('[data-testid="composer-dock"]');
      expect(el).toBeTruthy();
      return el;
    });
    expect(container.querySelector('.app-shell')).toBeTruthy();
    expect(dock.className).toContain('var(--safe-bottom)');
    expect(within(dock).getByLabelText('composer')).toBeTruthy();
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
    expect(within(drawerNav).getByRole('link', { name: 'Quota' }).className).toContain('max-md:min-h-14');
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
    history.replaceState(null, '', '/p/project-1');
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
    expect(runRow.className).toContain('border-status-needs-you/30');
    expect(runRow.closest('section[aria-label="alice"]')).toBeTruthy();
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
