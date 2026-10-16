import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import DashboardPage from './DashboardPage.svelte';
import { stopLive } from './live.js';

afterEach(() => {
  cleanup();
  stopLive();
  delete window.__EL_BOOT__;
  vi.unstubAllGlobals();
});

const response = (body) => ({ ok: true, json: async () => body });

class FakeEventSource {
  static instances = [];
  constructor(url) {
    this.url = String(url);
    this.constructor.instances.push(this);
  }
  close() {}
}

describe('All projects Dashboard', () => {
  it('renders active work from the grouped payload and links through stable Project ids', async () => {
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      expect(String(url)).toBe('/api/dashboard');
      return response({
        groups: {
          'stable-project-id': {
            sessions: [
              { id: 'session-active', title: 'Active Session', status: 'working' },
              { id: 'session-idle', title: 'Idle Session', status: 'idle' },
            ],
            runs: [{ id: 'run-needs-you', spec: '/repo/.scratch/demo/spec.md', status: 'needs-you' }],
          },
          none: {
            sessions: [{ id: 'home-session', title: 'Home Session', status: 'needs-you' }],
            runs: [],
          },
        },
      });
    }));
    const onNavigate = vi.fn();
    const { container } = render(DashboardPage, {
      props: {
        projects: [{ id: 'stable-project-id', name: 'Renamed Project', path: '/repo' }],
        onNavigate,
      },
    });

    expect(await screen.findByText('Renamed Project')).toBeTruthy();
    expect(screen.getByText('Active Session')).toBeTruthy();
    expect(screen.getByText('/repo/.scratch/demo/spec.md')).toBeTruthy();
    expect(screen.getByText('Home Session')).toBeTruthy();
    expect(screen.queryByText('Idle Session')).toBeNull();
    expect(container.querySelectorAll('[data-testid="dashboard-project"]')).toHaveLength(2);

    await fireEvent.click(screen.getByRole('button', { name: /Active Session Running/ }));
    expect(onNavigate).toHaveBeenLastCalledWith('/s/session-active');
    await fireEvent.click(screen.getByRole('button', { name: '/repo/.scratch/demo/spec.md Needs you' }));
    expect(onNavigate).toHaveBeenLastCalledWith('/p/stable-project-id/runs/run-needs-you');
  });

  it('shows an empty state when the active payload has no groups', async () => {
    vi.stubGlobal('EventSource', FakeEventSource);
    vi.stubGlobal('fetch', vi.fn(async () => response({ groups: {} })));
    render(DashboardPage);
    expect(await screen.findByText(/Nothing needs attention/)).toBeTruthy();
  });
  it('hands off the server Dashboard snapshot after the client Dashboard mounts', async () => {
    vi.stubGlobal('EventSource', FakeEventSource);
	window.__EL_BOOT__ = { '/api/dashboard': { groups: {} } };
	const fetchMock = vi.fn(async () => response({ groups: {} }));
	vi.stubGlobal('fetch', fetchMock);
    const snapshot = document.createElement('div');
    snapshot.id = 'el-ssr-dashboard';
    document.body.append(snapshot);

    render(DashboardPage);
	expect(snapshot.isConnected).toBe(true);
    await waitFor(() => expect(document.getElementById('el-ssr-dashboard')).toBeNull());
    expect(await screen.findByText(/Nothing needs attention/)).toBeTruthy();
	expect(fetchMock).not.toHaveBeenCalled();
  });
});
