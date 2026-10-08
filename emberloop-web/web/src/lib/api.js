// API client. The base URL is configurable without server rendering:
// a build-time Vite variable or a runtime global on the served page.
export function apiBase() {
  const runtime = typeof window !== 'undefined' ? window.__EMBERLOOP_API_BASE__ : '';
  const build = import.meta.env?.VITE_API_BASE ?? '';
  return String(runtime || build || '').replace(/\/$/, '');
}

const url = (path) => apiBase() + path;

async function json(res) {
  if (!res.ok) throw new Error(`request failed: ${res.status}`);
  return res.json();
}

export const listSessions = () => fetch(url('/api/sessions')).then(json);

export const createSession = (body) =>
  fetch(url('/api/sessions'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

export const getSession = (id) => fetch(url(`/api/sessions/${id}`)).then(json);

// mode travels along for forward compatibility; the current Daemon stores
// the text and ignores the extra field.
export const postPrompt = (id, text, mode = 'prompt') =>
  fetch(url(`/api/sessions/${id}/prompt`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, mode }),
  }).then(json);

export function openStream(id, since, onEvent) {
  const source = new EventSource(url(`/api/sessions/${id}/events?since=${since}`));
  source.onmessage = (ev) => {
    try {
      onEvent(JSON.parse(ev.data));
    } catch {
      // Malformed line: skip, keep the stream alive.
    }
  };
  return source;
}
