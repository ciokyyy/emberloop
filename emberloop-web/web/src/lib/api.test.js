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

describe('promotions client', () => {
  it('proposes a Rule against a Session worktree with its target', async () => {
    const seen = {};
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        seen.url = String(u);
        seen.opts = opts;
        return reply(201, '{"id":7,"status":"pending","target":"agents_md","target_path":"AGENTS.md","diff":"--- a/AGENTS.md\\n"}');
      }),
    );
    const item = await api.proposePromotion('r1', 's1', 'agents_md');
    expect(item.id).toBe(7);
    expect(item.diff).toContain('AGENTS.md');
    expect(seen.url).toBe('/api/promotions');
    expect(seen.opts.method).toBe('POST');
    expect(JSON.parse(seen.opts.body)).toEqual({ rule_id: 'r1', session_id: 's1', target: 'agents_md' });
  });

  it('lists the promotion queue and decides with an optional edit', async () => {
    const urls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        urls.push(String(u));
        if (String(u).endsWith('/decide')) {
          expect(opts.method).toBe('POST');
          return reply(200, '{"status":"approved","rule_id":"r1"}');
        }
        return reply(200, '[{"id":7,"status":"pending","target":"agents_md","target_path":"AGENTS.md","diff":"+x"}]');
      }),
    );
    const queue = await api.listPromotions();
    expect(queue).toHaveLength(1);
    expect(queue[0].diff).toBe('+x');
    const out = await api.decidePromotion(7, 'approved', { text: 'edited' });
    expect(out.status).toBe('approved');
    expect(urls).toEqual(['/api/promotions/queue', '/api/promotions/queue/7/decide']);
  });

  it('promotes a Rule from its own Session without leaving the chat', async () => {
    const seen = {};
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        seen.url = String(u);
        seen.opts = opts;
        return reply(201, '{"id":9,"status":"pending"}');
      }),
    );
    const item = await api.promoteSessionRule('s1', 'r1');
    expect(item.id).toBe(9);
    expect(seen.url).toBe('/api/sessions/s1/promote');
    expect(JSON.parse(seen.opts.body)).toEqual({ rule_id: 'r1', target: '' });
  });
});

describe('Claude login client', () => {
  it('starts, reads, submits a code to, and cancels the host PTY session', async () => {
    const calls = [];
    vi.stubGlobal('fetch', vi.fn(async (url, options = {}) => {
      calls.push({ url: String(url), method: options.method ?? 'GET', body: options.body ? JSON.parse(options.body) : null });
      return reply(200, '{"id":"login-1","state":"running","output":"Visit https://claude.ai"}');
    }));

    expect((await api.startClaudeLogin()).id).toBe('login-1');
    expect((await api.getClaudeLogin('login-1')).state).toBe('running');
    await api.sendClaudeLoginCode('login-1', 'pasted-code');
    await api.cancelClaudeLogin('login-1');

    expect(calls).toEqual([
      { url: '/api/providers/claude/login', method: 'POST', body: null },
      { url: '/api/providers/claude/login/login-1', method: 'GET', body: null },
      { url: '/api/providers/claude/login/login-1/code', method: 'POST', body: { code: 'pasted-code' } },
      { url: '/api/providers/claude/login/login-1', method: 'DELETE', body: null },
    ]);
  });
});
