import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
import Root from './Root.svelte';
import { UNAUTHORIZED } from './lib/api.js';

const alice = { id: 'u1', username: 'alice', admin: true, created: '2026-10-11T00:00:00Z' };

// stubDaemon fakes the auth routes with one session flag. The rest of the API
// answers empty, which is all the shell needs to boot.
function stubDaemon({ signedIn = false } = {}) {
  const daemon = { signedIn, meFails: false, calls: [] };
  const reply = (status, body = '') => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => (typeof body === 'string' ? JSON.parse(body || 'null') : body),
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
  });
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts = {}) => {
      const path = new URL(String(u), 'http://daemon.test').pathname;
      daemon.calls.push(`${opts.method ?? 'GET'} ${path}`);
      if (path === '/api/auth/me') {
        if (daemon.meFails) throw new TypeError('fetch failed');
        return daemon.signedIn ? reply(200, alice) : reply(401, 'unauthorized\n');
      }
      if (path === '/api/auth/state') return reply(200, { empty: false, signup_enabled: true });
      if (path === '/api/auth/login') {
        daemon.signedIn = true;
        return reply(200, alice);
      }
      if (path === '/api/auth/logout') {
        daemon.signedIn = false;
        return reply(204, '');
      }
      if (path === '/api/specs') return reply(200, { specs: [] });
      return reply(200, []);
    }),
  );
  return daemon;
}

beforeEach(() => {
  // Each render boots from the URL and local storage: reset both.
  history.replaceState(null, '', '/');
  localStorage.clear();
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
  vi.stubGlobal('EventSource', class { close() {} });
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('Root gate', () => {
  it('shows only the sign-in screen while signed out', async () => {
    stubDaemon({ signedIn: false });
    const { container } = render(Root);
    expect(await within(container).findByTestId('auth-screen')).toBeTruthy();
    expect(within(container).queryByTestId('sidebar')).toBeNull();
  });

  it('opens the shell with the signed-in User in the footer', async () => {
    stubDaemon({ signedIn: true });
    const { container } = render(Root);
    const account = await within(container).findByTestId('account');
    expect(account.textContent).toContain('alice');
    expect(within(container).getByTestId('sidebar')).toBeTruthy();
    expect(within(container).queryByTestId('auth-screen')).toBeNull();
  });

  it('signs out to the sign-in screen', async () => {
    const daemon = stubDaemon({ signedIn: true });
    const { container } = render(Root);
    await fireEvent.click(await within(container).findByLabelText('Sign out'));
    expect(await within(container).findByTestId('auth-screen')).toBeTruthy();
    expect(daemon.calls).toContain('POST /api/auth/logout');
  });

  it('returns to sign-in when a route answers 401 mid-session', async () => {
    const daemon = stubDaemon({ signedIn: true });
    const { container } = render(Root);
    await within(container).findByTestId('account');
    daemon.signedIn = false;
    window.dispatchEvent(new Event(UNAUTHORIZED));
    expect(await within(container).findByTestId('auth-screen')).toBeTruthy();
  });

  it('offers a retry when the Daemon does not answer, then opens the shell', async () => {
    const daemon = stubDaemon({ signedIn: true });
    daemon.meFails = true;
    const { container } = render(Root);
    const retry = await within(container).findByRole('button', { name: 'Try again' });
    daemon.meFails = false;
    await fireEvent.click(retry);
    expect(await within(container).findByTestId('account')).toBeTruthy();
  });
});
