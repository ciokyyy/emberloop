// One realtime channel for the whole UI. The Daemon publishes the name of
// every topic that changed over a single EventSource; a component asks
// `watch(topic, refetch)` and runs its normal GET again on each change.
// Components never open their own state streams or poll. On (re)connect the
// Daemon sends "hello" and every watcher refetches, so a dropped connection
// loses nothing.
//
// Topics: sessions, session:<id>, runs, run:<id>, settings, users.
import { apiBase } from './api.js';

const watchers = new Map(); // topic -> Set<fn>
let source = null;

function dispatch(topic) {
  const fns = topic === 'hello' ? [...watchers.values()].flatMap((s) => [...s]) : [...(watchers.get(topic) ?? [])];
  // One call per function per message, even when it watches several topics.
  for (const fn of new Set(fns)) {
    try {
      fn(topic);
    } catch {
      // A failing watcher must not stop the others.
    }
  }
}

function ensure() {
  if (source || typeof EventSource === 'undefined') return;
  source = new EventSource(apiBase() + '/api/live');
  source.onmessage = (ev) => dispatch(String(ev.data));
  // EventSource reconnects by itself and the Daemon answers with hello.
}

// watch runs fn whenever topic changes and once per (re)connect. It returns
// the stop function. Calls are coalesced: fn never runs twice at once, and
// changes that arrive while it runs trigger exactly one more run.
export function watch(topic, fn) {
  ensure();
  let running = false;
  let again = false;
  const run = async (t) => {
    if (running) {
      again = true;
      return;
    }
    running = true;
    try {
      await fn(t);
    } catch {
      // Refetch errors keep the last good value.
    } finally {
      running = false;
      if (again) {
        again = false;
        run(t);
      }
    }
  };
  if (!watchers.has(topic)) watchers.set(topic, new Set());
  watchers.get(topic).add(run);
  return () => {
    watchers.get(topic)?.delete(run);
    if (!watchers.get(topic)?.size) watchers.delete(topic);
  };
}

// stopLive closes the stream and drops every watcher. App calls it on unmount,
// which sign-out triggers, so the next sign-in starts from a clean channel.
export function stopLive() {
  source?.close();
  source = null;
  watchers.clear();
}
