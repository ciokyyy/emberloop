// API client. The base URL is configurable without server rendering:
// a build-time Vite variable or a runtime global on the served page.
export function apiBase() {
  const runtime = typeof window !== 'undefined' ? window.__EL_API_BASE__ : '';
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

// opts: { mode, images } where images are { mime, data (base64) }.
export const postPrompt = (id, text, opts = {}) =>
  fetch(url(`/api/sessions/${id}/prompt`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text, ...opts }),
  }).then(json);

export const postSteer = (id, text) =>
  fetch(url(`/api/sessions/${id}/steer`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  }).then(json);

export const postFollowup = (id, text) =>
  fetch(url(`/api/sessions/${id}/followup`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  }).then(json);

export const interruptSession = (id) =>
  fetch(url(`/api/sessions/${id}/interrupt`), { method: 'POST' }).then(json);

export const getQueue = (id) => fetch(url(`/api/sessions/${id}/queue`)).then(json);

export const clearQueue = (id) => fetch(url(`/api/sessions/${id}/queue`), { method: 'DELETE' }).then(json);

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

export const getModels = (harness, current = '') =>
  fetch(url(`/api/models?harness=${encodeURIComponent(harness)}${current ? `&current=${encodeURIComponent(current)}` : ''}`)).then(json);

export const getSettings = () => fetch(url('/api/settings')).then(json);

export const putSettings = (body) =>
  fetch(url('/api/settings'), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);
export const listSpecs = (repo) => fetch(url(`/api/specs?repo=${encodeURIComponent(repo)}`)).then(json);

export const previewSpec = (spec) => fetch(url(`/api/specs/preview?spec=${encodeURIComponent(spec)}`)).then(json);

export const listRuns = () => fetch(url('/api/runs')).then(json);

export const startRun = (body) =>
  fetch(url('/api/runs'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

export const getRun = (id) => fetch(url(`/api/runs/${id}`)).then(json);

export function openRunStream(id, onRun) {
  const source = new EventSource(url(`/api/runs/${id}/events`));
  source.onmessage = (ev) => {
    try {
      onRun(JSON.parse(ev.data));
    } catch {
      // Malformed line: skip, keep the stream alive.
    }
  };
  return source;
}

export const pauseRun = (id) =>
  fetch(url(`/api/runs/${id}/pause`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  }).then(json);

export const resumeRun = (id) =>
  fetch(url(`/api/runs/${id}/resume`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  }).then(json);

export const listProjects = () => fetch(url('/api/projects')).then(json);

export const addProject = (body) =>
  fetch(url('/api/projects'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(async (res) => {
    if (!res.ok) throw new Error((await res.text()).trim() || `request failed: ${res.status}`);
    return res.json();
  });

export const renameProject = (id, name) =>
  fetch(url(`/api/projects/${id}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name }),
  }).then(json);

export const removeProject = (id) => fetch(url(`/api/projects/${id}`), { method: 'DELETE' }).then(json);

export const getGit = (id) => fetch(url(`/api/projects/${id}/git`)).then(json);

export const getGitDiff = (id, path, staged = false) =>
  fetch(url(`/api/projects/${id}/git/diff?path=${encodeURIComponent(path)}${staged ? '&staged=1' : ''}`)).then(json);

export const compressSession = (id, instructions = '') =>
  fetch(url(`/api/sessions/${id}/compress`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ instructions }),
  }).then(async (res) => {
    const body = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(body.error || `request failed: ${res.status}`);
    return body;
  });

// Approve or deny a sudo request. The password is optional and travels once;
// the Daemon keeps it in memory for the run only.
export const decideSudo = (sessionId, requestId, action, password = '') =>
  fetch(url(`/api/sessions/${sessionId}/sudo/${requestId}`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ action, password }),
  }).then(async (res) => {
    if (res.status === 404) throw new Error('This request expired (the Daemon restarted). Ask the agent to run it again.');
    if (res.status === 409) throw new Error('Already decided.');
    return json(res);
  });
