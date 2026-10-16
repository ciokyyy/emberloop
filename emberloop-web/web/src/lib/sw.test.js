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

const source = serviceWorker;

function worker() {
  const handlers = {};
  const put = vi.fn(() => Promise.resolve());
  const fetch = vi.fn(async () => ({ ok: true, clone: () => ({}) }));
  const self = {
    location: { origin: 'https://emberloop.test' },
    addEventListener: (name, callback) => { handlers[name] = callback; },
  };
  const caches = { open: vi.fn(async () => ({ put })) };
  runInNewContext(source, { self, caches, fetch, URL, Promise });
  return { handlers, put, fetch };
}

describe('service worker fetch caching', () => {
  it('leaves navigations and API requests to the network without caching', () => {
    const { handlers, put, fetch } = worker();
    for (const [url, mode] of [
      ['https://emberloop.test/p/private-project', 'navigate'],
      ['https://emberloop.test/api/sessions', 'cors'],
    ]) {
      const event = { request: { method: 'GET', url, mode } };
      handlers.fetch(event);
      expect(event.respondWith).toBeUndefined();
    }
    expect(fetch).not.toHaveBeenCalled();
    expect(put).not.toHaveBeenCalled();
  });

  it('caches hashed assets but ignores unhashed files', async () => {
    const { handlers, put, fetch } = worker();
    const assetEvent = {
      request: { method: 'GET', url: 'https://emberloop.test/assets/app-a1b2c3d4.js', mode: 'cors' },
      respondWith: (promise) => { assetEvent.response = promise; },
    };
    handlers.fetch(assetEvent);
    await assetEvent.response;
    expect(fetch).toHaveBeenCalledOnce();
    expect(put).toHaveBeenCalledOnce();

    const plainEvent = { request: { method: 'GET', url: 'https://emberloop.test/manifest.webmanifest', mode: 'cors' } };
    handlers.fetch(plainEvent);
    expect(plainEvent.respondWith).toBeUndefined();
  });
});
