// Shared browser-side quota state. Both the Quota page and its contextual
// sidebar read this store, so one initial request feeds both views. The source
// can also provide pushed updates; the default source is fetch-only until the
// quota live topic is connected.
import { get, writable } from 'svelte/store';
import { getQuota } from './api.js';

const noop = () => {};
const defaultSource = {
  fetch: getQuota,
  subscribe: () => noop,
};

function providerList(body) {
  if (Array.isArray(body)) return body;
  return Array.isArray(body?.providers) ? body.providers : null;
}

export function createQuotaStore(initialSource = defaultSource) {
  if (typeof initialSource?.fetch !== 'function') {
    throw new TypeError('Quota source must provide fetch().');
  }

  const state = writable([]);
  let source = initialSource;
  let listenerCount = 0;
  let stopSource = null;
  let pending = null;
  let loaded = false;
  let revision = 0;

  function publish(body, expectedSource = source) {
    if (expectedSource !== source) return false;
    const providers = providerList(body);
    if (!providers) return false;
    revision++;
    state.set(providers);
    loaded = true;
    return true;
  }

  function startSource() {
    if (stopSource) return;
    const activeSource = source;
    try {
      const stop = activeSource.subscribe?.((body) => publish(body, activeSource));
      stopSource = typeof stop === 'function' ? stop : noop;
    } catch {
      stopSource = noop;
    }
  }

  function unsubscribeSource() {
    stopSource?.();
    stopSource = null;
  }

  function subscribe(run) {
    const unsubscribeState = state.subscribe(run);
    if (listenerCount === 0) loaded = false;
    listenerCount++;
    if (listenerCount === 1) startSource();

    let active = true;
    return () => {
      if (!active) return;
      active = false;
      unsubscribeState();
      listenerCount--;
      if (listenerCount === 0) {
        unsubscribeSource();
        loaded = false;
      }
    };
  }

  function load() {
    if (pending) return pending;
    if (loaded) return Promise.resolve({ providers: get(state) });
    const activeSource = source;
    const startedAtRevision = revision;
    const request = Promise.resolve()
      .then(() => activeSource.fetch())
      .then((body) => {
        // A newer pushed/manual value wins over a fetch that started earlier.
        if (activeSource === source && revision === startedAtRevision) publish(body, activeSource);
        return body;
      });
    pending = request;
    request.then(
      () => { if (pending === request) pending = null; },
      () => { if (pending === request) pending = null; },
    );
    return request;
  }

  function replaceSource(nextSource) {
    if (typeof nextSource?.fetch !== 'function') {
      throw new TypeError('Quota source must provide fetch().');
    }
    unsubscribeSource();
    source = nextSource;
    revision++;
    pending = null;
    loaded = false;
    state.set([]);
    if (listenerCount > 0) startSource();
  }

  return {
    subscribe,
    load,
    set: (body) => publish(body),
    replaceSource,
  };
}

export const quotaStore = createQuotaStore();
