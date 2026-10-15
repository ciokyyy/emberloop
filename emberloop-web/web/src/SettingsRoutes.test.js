import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import App from './App.svelte';
import { stopLive } from './lib/live.js';
import { SETTINGS_GROUPS, settingsPath } from './lib/settings.js';

const response = (body) => ({ ok: true, json: async () => body });
const settings = { roles: {}, max_tickets: 3, max_tickets_from_env: false };

function setup(path = '/settings', phone = true) {
  history.replaceState(null, '', path);
  vi.stubGlobal('matchMedia', vi.fn((query) => ({
    matches: phone && query.includes('max-width: 767px'),
    addListener() {},
    removeListener() {},
    addEventListener() {},
    removeEventListener() {},
    dispatchEvent() { return false; },
  })));
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  vi.stubGlobal('EventSource', class { close() {} });
  vi.stubGlobal('fetch', vi.fn(async (input, options = {}) => {
    const url = String(input);
    if (url.includes('/api/models')) return response({ models: ['auto', 'opus'] });
    if (url.endsWith('/api/settings') && options.method === 'PUT') return response(settings);
    if (url.endsWith('/api/settings')) return response(settings);
    if (url.endsWith('/api/projects')) return response([]);
    if (url.endsWith('/api/users')) return response([]);
    if (url.endsWith('/api/disk')) return response({});
    if (url.includes('/api/profile/versions') || url.includes('/api/profile/queue')) return response([]);
    if (url.endsWith('/api/profile')) return response({ lines: [], version: 0, rendered: '' });
    return response({});
  }));
  return render(App, { props: { user: { id: 'admin-id', username: 'admin', admin: true } } });
}

afterEach(() => {
  cleanup();
  stopLive();
  vi.unstubAllGlobals();
  localStorage.clear();
  history.replaceState(null, '', '/');
});

describe('Settings routes and shell', () => {
  it('shows a phone hub and navigates every page with its scope pill', async () => {
    const { container } = setup();
    const hub = await within(container).findByTestId('settings-hub');
    for (const group of SETTINGS_GROUPS) {
      expect(within(hub).getByRole('heading', { name: group.title })).toBeTruthy();
      for (const item of group.items) {
        const link = within(hub).getByRole('link', { name: new RegExp(item.label) });
        expect(link.getAttribute('href')).toBe(settingsPath(item.id));
        expect(link.className).toContain('min-h-16');
      }
    }

    let useHub = true;
    for (const group of SETTINGS_GROUPS) {
      for (const item of group.items) {
        const nav = useHub
          ? hub
          : within(container).getByRole('navigation', { name: 'Settings navigation' });
        await fireEvent.click(within(nav).getByRole('link', { name: new RegExp(item.label) }));
        useHub = false;
        await vi.waitFor(() => expect(window.location.pathname).toBe(settingsPath(item.id)));
        expect(await screen.findByRole('heading', { level: 1, name: item.label })).toBeTruthy();
        expect(within(container).getByTestId('settings-scope').textContent).toBe(item.scope);
        const back = within(container).getByTestId('settings-page').querySelector('button');
        expect(back.className).toContain('min-h-11');
      }
    }
  });

  it('redirects the desktop hub route to Agents', async () => {
    const { container } = setup('/settings', false);
    expect(await within(container).findByRole('heading', { level: 1, name: 'Agents' })).toBeTruthy();
    await vi.waitFor(() => expect(window.location.pathname).toBe('/settings/agents'));
  });
});
