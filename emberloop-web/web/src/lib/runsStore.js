// One browser-side Run-summary request feeds the App sidebar and Runs page.
// Run-topic bursts are coalesced before fetching so each consumer sees the
// same list without issuing its own request.
import { get, writable } from 'svelte/store';
import { listRuns } from './api.js';
import { watch } from './live.js';

const COALESCE_MS = 50;
const noop = () => {};
const defaultSource = {
  fetch: listRuns,
  subscribe: (onChange) => watch('runs', onChange),
};

export function createRunsStore(source = defaultSource, { coalesceMs = COALESCE_MS } = {}) {
  if (typeof source?.fetch !== 'function') {
    throw new TypeError('Runs source must provide fetch().');
  }

  const state = writable([]);
  let listenerCount = 0;
  let stopSource = null;
  let pending = null;
  let refreshAfterPending = false;
  let loaded = false;
  let timer = null;
  let generation = 0;
  let sawHello = false;

  function finishRequest(request, requestGeneration) {
    if (pending !== request) return;
    pending = null;
    if (requestGeneration !== generation || listenerCount === 0) return;
    if (refreshAfterPending) {
      refreshAfterPending = false;
      void fetchRuns(true).catch(noop);
    }
  }

  function fetchRuns(force = false) {
    if (pending) {
      if (force) refreshAfterPending = true;
      return pending;
    }
    if (!force && loaded) return Promise.resolve({ runs: get(state) });

    const requestGeneration = generation;
    const request = Promise.resolve()
      .then(() => source.fetch())
      .then((body) => {
        if (requestGeneration === generation) {
          const runs = Array.isArray(body?.runs) ? body.runs : [];
          state.set(runs);
          loaded = true;
        }
        return body;
      });
    pending = request;
    request.then(
      () => finishRequest(request, requestGeneration),
      () => finishRequest(request, requestGeneration),
    );
    return request;
  }

  // Anchor the coalesce window to the first event; a burst cannot defer refresh.
  function scheduleRefresh() {
    if (timer !== null) return;
    timer = setTimeout(() => {
      timer = null;
      void fetchRuns(true).catch(noop);
    }, coalesceMs);
  }

  function startSource() {
    if (stopSource) return;
    sawHello = false;
    try {
      const stop = source.subscribe?.((topic) => {
        // The first live hello follows the initial load and is not a Run
        // change. Later hellos represent reconnects and should refresh.
        if (topic === 'hello' && !sawHello) {
          sawHello = true;
          return;
        }
        scheduleRefresh();
      });
      stopSource = typeof stop === 'function' ? stop : noop;
    } catch {
      stopSource = noop;
    }
  }

  function subscribe(run) {
    const firstListener = listenerCount === 0;
    if (firstListener) loaded = false;
    const unsubscribeState = state.subscribe(run);
    listenerCount++;
    if (firstListener) {
      startSource();
      void fetchRuns().catch(noop);
    }

    let active = true;
    return () => {
      if (!active) return;
      active = false;
      unsubscribeState();
      listenerCount--;
      if (listenerCount === 0) {
        stopSource?.();
        stopSource = null;
        if (timer !== null) clearTimeout(timer);
        timer = null;
        refreshAfterPending = false;
        loaded = false;
        generation++;
        pending = null;
      }
    };
  }

  return {
    subscribe,
    load: () => fetchRuns(),
  };
}

export const runsStore = createRunsStore();
