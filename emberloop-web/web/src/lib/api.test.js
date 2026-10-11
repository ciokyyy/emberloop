import { describe, it, expect, vi, afterEach } from 'vitest';
import * as api from './api.js';

afterEach(() => {
  vi.unstubAllGlobals();
});

// reply makes a fetch answer with a status and a plain body.
const reply = (status, text = '') => ({
  ok: status >= 200 && status < 300,
  status,
  text: async () => text,
  json: async () => JSON.parse(text || 'null'),
});

describe('auth client', () => {
  it('signs in with the credentials and returns the User', async () => {
    const seen = {};
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        seen.url = String(u);
        seen.opts = opts;
        return reply(200, '{"id":"u1","username":"alice","admin":true}');
      }),
    );
    const who = await api.login('alice', 'correct horse battery');
    expect(who.username).toBe('alice');
    expect(seen.url).toBe('/api/auth/login');
    expect(seen.opts.method).toBe('POST');
    expect(JSON.parse(seen.opts.body)).toEqual({ username: 'alice', password: 'correct horse battery' });
  });

  it('rejects with the status and the Daemon reason', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => reply(401, 'invalid username or password\n')));
    await expect(api.login('alice', 'nope')).rejects.toMatchObject({ status: 401, message: 'invalid username or password' });
  });

  it('reads the auth state and the signed-in User', async () => {
    const urls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        urls.push(String(u));
        return String(u).endsWith('/state') ? reply(200, '{"empty":true,"signup_enabled":true}') : reply(200, '{"username":"alice"}');
      }),
    );
    expect(await api.getAuthState()).toEqual({ empty: true, signup_enabled: true });
    expect(await api.getMe()).toEqual({ username: 'alice' });
    expect(urls).toEqual(['/api/auth/state', '/api/auth/me']);
  });

  it('logs out with a POST and accepts the empty 204', async () => {
    const seen = {};
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        seen.url = String(u);
        seen.method = opts.method;
        return reply(204);
      }),
    );
    await expect(api.logout()).resolves.toBeUndefined();
    expect(seen).toEqual({ url: '/api/auth/logout', method: 'POST' });
  });
});

describe('unauthorized signal', () => {
  it('fires when a Daemon route outside the auth routes answers 401', async () => {
    const onUnauthorized = vi.fn();
    window.addEventListener(api.UNAUTHORIZED, onUnauthorized);
    vi.stubGlobal('fetch', vi.fn(async () => reply(401, 'unauthorized\n')));
    await expect(api.listSessions()).rejects.toMatchObject({ status: 401 });
    expect(onUnauthorized).toHaveBeenCalledTimes(1);
    window.removeEventListener(api.UNAUTHORIZED, onUnauthorized);
  });

  it('stays quiet for the auth routes, which answer 401 on a wrong password or no session', async () => {
    const onUnauthorized = vi.fn();
    window.addEventListener(api.UNAUTHORIZED, onUnauthorized);
    vi.stubGlobal('fetch', vi.fn(async () => reply(401, 'unauthorized\n')));
    await expect(api.getMe()).rejects.toMatchObject({ status: 401 });
    await expect(api.login('alice', 'nope')).rejects.toMatchObject({ status: 401 });
    expect(onUnauthorized).not.toHaveBeenCalled();
    window.removeEventListener(api.UNAUTHORIZED, onUnauthorized);
  });
});
