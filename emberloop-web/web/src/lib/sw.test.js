import { describe, it, expect, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { runInNewContext } from 'node:vm';

const serviceWorker = readFileSync(resolve(process.cwd(), 'public/sw.js'), 'utf8');

describe('Profile patch push actions', () => {
  it.each([['approve', 'approved'], ['reject', 'rejected']])('submits %s through the authenticated queue API', async (action, status) => {
    const listeners = new Map();
    const client = { focus: vi.fn(async () => {}), postMessage: vi.fn() };
    const self = {
      addEventListener: (name, listener) => listeners.set(name, listener),
      clients: { matchAll: vi.fn(async () => [client]), openWindow: vi.fn() },
    };
    const fetch = vi.fn(async () => ({ ok: true }));
    runInNewContext(serviceWorker, { self, fetch, caches: {}, URL });

    let work;
    const close = vi.fn();
    listeners.get('notificationclick')({
      action,
      notification: { data: { path: '/', profilePatchId: 19 }, close },
      waitUntil: (promise) => (work = promise),
    });
    await work;

    expect(close).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith('/api/profile/queue/19/decide', {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ status }),
    });
    expect(client.focus).toHaveBeenCalledOnce();
  });
});
