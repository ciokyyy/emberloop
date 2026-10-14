// One shared EventSource carries ordinary topic invalidations; watched
// components refetch their own endpoints on it. Quota opens a separate
// topic=quota stream only while subscribed, carrying provider lists without
// making the shared stream reconnect as unrelated watchers change.
import { apiBase } from './api.js';

const watchers = new Map(); // topic -> Set<fn>
let source = null;
let quotaSource = null;

function dispatch(topic, payload, onlyTopic = null) {
  const fns = onlyTopic
    ? [...(watchers.get(onlyTopic) ?? [])]
    : topic === 'hello'
      ? [...watchers.values()].flatMap((s) => [...s])
      : [...(watchers.get(topic) ?? [])];
  // One call per function per message, even when it watches several topics.
  for (const fn of new Set(fns)) {
    try {
      fn(topic, payload);
    } catch {
      // A failing watcher must not stop the others.
    }
  }
}

function openSource() {
  if (source || !watchers.size || typeof EventSource === 'undefined') return;
  const next = new EventSource(`${apiBase()}/api/live`);
  source = next;
  next.onmessage = (ev) => {
    if (source === next) dispatch(String(ev.data));
  };
}

// Quota uses its own opt-in stream so adding/removing ordinary topic watchers
// never reconnects the shared stream or causes a global hello/refetch burst.
function openQuotaSource() {
  if (quotaSource || !watchers.get('quota')?.size || typeof EventSource === 'undefined') return;
  const next = new EventSource(`${apiBase()}/api/live?topic=quota`);
  quotaSource = next;
  let greeted = false;
  next.onmessage = (ev) => {
    if (quotaSource !== next) return;
    const data = String(ev.data);
    if (data === 'hello') {
      if (greeted) dispatch('hello', undefined, 'quota');
      greeted = true;
      return;
    }
    try {
      const message = JSON.parse(data);
      if (message?.topic === 'quota') dispatch('quota', message, 'quota');
    } catch {
      // Non-quota live topics are irrelevant on this opt-in stream.
    }
  };
}

// watch runs fn whenever topic changes and once per (re)connect. It returns
// the stop function. Calls are coalesced: fn never runs twice at once, and
// changes that arrive while it runs trigger exactly one more run with the
// newest payload.
export function watch(topic, fn) {
  let running = false;
  let again = false;
  let queuedTopic;
  let queuedPayload;
  const run = async (t, payload) => {
    if (running) {
      again = true;
      queuedTopic = t;
      queuedPayload = payload;
      return;
    }
    running = true;
    try {
      await fn(t, payload);
    } catch {
      // Refetch errors keep the last good value.
    } finally {
      running = false;
      if (again) {
        again = false;
        const nextTopic = queuedTopic;
        const nextPayload = queuedPayload;
        queuedTopic = undefined;
        queuedPayload = undefined;
        void run(nextTopic, nextPayload);
      }
    }
  };
  const wasWatchingTopic = Boolean(watchers.get(topic)?.size);
  if (!watchers.has(topic)) watchers.set(topic, new Set());
  watchers.get(topic).add(run);
  openSource();
  if (topic === 'quota' && !wasWatchingTopic) openQuotaSource();
  return () => {
    watchers.get(topic)?.delete(run);
    if (!watchers.get(topic)?.size) {
      watchers.delete(topic);
      if (topic === 'quota') {
        quotaSource?.close();
        quotaSource = null;
      }
    }
    if (!watchers.size) {
      source?.close();
      source = null;
    }
  };
}

// stopLive closes both streams and drops every watcher. App calls it on
// unmount/sign-out so the next sign-in starts from a clean channel.
export function stopLive() {
  source?.close();
  quotaSource?.close();
  source = null;
  quotaSource = null;
  watchers.clear();
}
