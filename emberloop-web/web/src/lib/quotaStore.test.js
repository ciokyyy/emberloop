import { describe, expect, it, vi } from 'vitest';
import { createQuotaStore } from './quotaStore.js';

describe('quotaStore', () => {
  it('shares one fetch, accepts mocked channel updates, and replaces its source', async () => {
    let publish;
    const stopChannel = vi.fn();
    const fetch = vi.fn(async () => ({ providers: [{ key: 'claude', windows: [{ pct: 36 }] }] }));
    const store = createQuotaStore({
      fetch,
      subscribe: (onUpdate) => {
        publish = onUpdate;
        return stopChannel;
      },
    });
    const values = [];
    const unsubscribe = store.subscribe((providers) => values.push(providers));

    await Promise.all([store.load(), store.load()]);
    expect(fetch).toHaveBeenCalledOnce();
    expect(values.at(-1)[0].windows[0].pct).toBe(36);

    publish({ providers: [{ key: 'claude', windows: [{ pct: 82 }] }] });
    expect(values.at(-1)[0].windows[0].pct).toBe(82);

    const nextFetch = vi.fn(async () => ({ providers: [{ key: 'openrouter', windows: [{ pct: 94 }] }] }));
    store.replaceSource({ fetch: nextFetch, subscribe: () => () => {} });
    expect(stopChannel).toHaveBeenCalledOnce();
    await store.load();
    expect(nextFetch).toHaveBeenCalledOnce();
    expect(values.at(-1)[0].key).toBe('openrouter');

    unsubscribe();
  });

  it('keeps a pushed value when an older fetch resolves afterward', async () => {
    let publish;
    let resolveFetch;
    const store = createQuotaStore({
      fetch: () => new Promise((resolve) => { resolveFetch = resolve; }),
      subscribe: (onUpdate) => { publish = onUpdate; return () => {}; },
    });
    const values = [];
    const unsubscribe = store.subscribe((providers) => values.push(providers));
    const pending = store.load();
    await Promise.resolve();
    publish({ providers: [{ key: 'claude', windows: [{ pct: 82 }] }] });
    resolveFetch({ providers: [{ key: 'claude', windows: [{ pct: 36 }] }] });
    await pending;

    expect(values.at(-1)[0].windows[0].pct).toBe(82);
    unsubscribe();
  });
});
