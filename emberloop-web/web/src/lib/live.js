// One realtime channel for the whole UI. The Daemon publishes state topic
// names over one EventSource; components refetch on those messages. Quota is
// the exception: its event carries the complete provider list. Subscribed
// topic names are sent on the connection so topic-scoped Daemon work can stop
// when no component is listening. On reconnect, "hello" refetches state topics.
import { apiBase } from './api.js';

const watchers = new Map(); // topic -> Set<fn>
let source = null;
let sourceTopics = '';

function dispatch(topic, payload) {
  const fns = topic === 'hello' ? [...watchers.values()].flatMap((s) => [...s]) : [...(watchers.get(topic) ?? [])];
  // One call per function per message, even when it watches several topics.
  for (const fn of new Set(fns)) {
    try {
      fn(topic, payload);
    } catch {
      // A failing watcher must not stop the others.
    }
  }
}

function syncSource() {
  const topics = [...watchers.keys()].sort();
  const key = topics.join('\u0000');
  if (source && key === sourceTopics) return;
  source?.close();
  source = null;
  sourceTopics = key;
  if (!topics.length || typeof EventSource === 'undefined') return;

  const params = new URLSearchParams();
  for (const topic of topics) params.append('topic', topic);
  const next = new EventSource(`${apiBase()}/api/live?${params}`);
  source = next;
  next.onmessage = (ev) => {
    if (source !== next) return;
    const data = String(ev.data);
    try {
      const message = JSON.parse(data);
      if (typeof message?.topic === 'string') {
        dispatch(message.topic, message);
        return;
      }
    } catch {
      // Existing state-topic messages are plain strings.
    }
    dispatch(data);
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
  if (!watchers.has(topic)) watchers.set(topic, new Set());
  watchers.get(topic).add(run);
  syncSource();
  return () => {
    watchers.get(topic)?.delete(run);
    if (!watchers.get(topic)?.size) watchers.delete(topic);
    syncSource();
  };
}

// stopLive closes the stream and drops every watcher. App calls it on unmount,
// which sign-out triggers, so the next sign-in starts from a clean channel.
export function stopLive() {
  source?.close();
  source = null;
  sourceTopics = '';
  watchers.clear();
}
