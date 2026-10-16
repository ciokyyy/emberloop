// API client. The base URL is configurable without server rendering:
// a build-time Vite variable or a runtime global on the served page.
export function apiBase() {
  const runtime = typeof window !== 'undefined' ? window.__EL_API_BASE__ : '';
  const build = import.meta.env?.VITE_API_BASE ?? '';
  return String(runtime || build || '').replace(/\/$/, '');
}

const url = (path) => apiBase() + path;

// Fired on window when a Daemon route outside the auth routes answers 401: the
// session has ended, so the app returns to the sign-in screen (Root.svelte).
export const UNAUTHORIZED = 'el:unauthorized';

async function json(res) {
  if (!res.ok) {
    if (res.status === 401 && typeof window !== 'undefined') window.dispatchEvent(new Event(UNAUTHORIZED));
    const err = new Error(`request failed: ${res.status}`);
    err.status = res.status;
    throw err;
  }
  return res.json();
}

export const listSessions = () => fetch(url('/api/sessions')).then(json);

// Active Session and Run summaries grouped by stable Project id for the home Dashboard.
export const getDashboard = () => fetch(url('/api/dashboard')).then(json);

export const createSession = (body) =>
  fetch(url('/api/sessions'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

// params: { tail, before, turns, at } for snapshot paging. No params means
// the full log, as before.
export const getSession = (id, params = null) => {
  let qs = '';
  if (params) {
    const q = new URLSearchParams();
    for (const k of ['tail', 'before', 'turns', 'at']) {
      if (params[k] != null && params[k] !== '') q.set(k, String(params[k]));
    }
    const s = q.toString();
    if (s) qs = `?${s}`;
  }
  return fetch(url(`/api/sessions/${id}${qs}`)).then(json);
};

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

// Session image attachments live under the Daemon state dir; the list
// feeds the composer gallery, and imageSrc renders chips and thumbnails
// from the same bytes the agent reads.
export const listImages = (id) => fetch(url(`/api/sessions/${id}/images`)).then(json);

export const uploadImage = (id, { name, mediaType, data }) =>
  fetch(url(`/api/sessions/${id}/images`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name, mediaType, data }),
  }).then(json);

// Uploads one voice recording to the Session store. Resolves its meta
// ({ id, ... }); the id rides the next prompt as an imageId.
export async function uploadAudio(id, { blob, durationMs, peaks }) {
  const data = await new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onload = () => resolve(String(r.result).split(',')[1] ?? '');
    r.onerror = () => reject(r.error);
    r.readAsDataURL(blob);
  });
  const body = await fetch(url(`/api/sessions/${id}/images`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name: 'voice', mediaType: blob.type || 'audio/webm', data, durationMs, peaks }),
  }).then(json);
  return body.image;
}

export const imageSrc = (sessionId, imageId) => url(`/api/sessions/${sessionId}/images/${imageId}`);

export const interruptSession = (id) =>
  fetch(url(`/api/sessions/${id}/interrupt`), { method: 'POST' }).then(json);

export const getQueue = (id) => fetch(url(`/api/sessions/${id}/queue`)).then(json);

export const clearQueue = (id) => fetch(url(`/api/sessions/${id}/queue`), { method: 'DELETE' }).then(json);

export function openStream(id, since, onEvent, opts = {}) {
  const source = new EventSource(url(`/api/sessions/${id}/events?since=${since}`));
  source.onmessage = (ev) => {
    try {
      onEvent(JSON.parse(ev.data));
    } catch {
      // Malformed line: skip, keep the stream alive.
    }
    try {
      opts.onMessage?.();
    } catch {
      // Hook must never break the stream.
    }
  };
  source.onerror = (ev) => {
    try {
      opts.onError?.(ev);
    } catch {
      // Hook must never break the stream.
    }
  };
  source.onopen = () => {
    try {
      opts.onOpen?.();
    } catch {
      // Hook must never break the stream.
    }
  };
  return source;
}

export const getModels = (harness, current = '') =>
  fetch(url(`/api/models?harness=${encodeURIComponent(harness)}${current ? `&current=${encodeURIComponent(current)}` : ''}`)).then(json);

export const getSettings = () => fetch(url('/api/settings')).then(json);

export const getDisk = () => fetch(url('/api/disk')).then(json);

export const putSettings = (body) =>
  fetch(url('/api/settings'), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

// Profile: per-User standing instructions. GET reads the latest version with
// its rendered text; PUT appends a version from line-ID'd ops (add, edit,
// retire); versions lists every version; restore copies one as a new
// version; queue lists profile_patch proposals and decide approves (with an
// optional edit) or rejects them. Every approval writes a profile_updated
// Event to the owner's Sessions.
export const getProfile = () => fetch(url('/api/profile')).then(json);

export const putProfile = (ops) =>
  fetch(url('/api/profile'), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ops }),
  }).then(json);

export const getProfileVersions = () => fetch(url('/api/profile/versions')).then(json);

export const restoreProfile = (version) =>
  fetch(url('/api/profile/restore'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ version }),
  }).then(json);

export const getMemoryDigestPreference = () => fetch(url('/api/push/digest')).then(json);
export const setMemoryDigestPreference = (enabled) =>
  fetch(url('/api/push/digest'), { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ enabled }) }).then(json);

export const getProfileQueue = () => fetch(url('/api/profile/queue')).then(json);

export const decideProfileQueue = (id, status, ops = null) =>
  fetch(url(`/api/profile/queue/${id}/decide`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(ops ? { status, ops } : { status }),
  }).then(json);

// Memory page: the User's own bank. Rules and observations list newest first
// with an empty query and search through FTS5 with one; the queue lists rule
// proposals with their quotes and source Sessions; status carries the engine
// chip plus queue and ledger stats; purge deletes the chosen records after
// the typed PURGE confirmation and reports what it removed.
export const getRules = (q = '', limit = 50) => {
  const qs = q ? `?q=${encodeURIComponent(q)}&limit=${limit}` : `?limit=${limit}`;
  return fetch(url(`/api/rules${qs}`)).then(json);
};

export const getObservations = (q = '', limit = 50) => {
  const qs = q ? `?q=${encodeURIComponent(q)}&limit=${limit}` : `?limit=${limit}`;
  return fetch(url(`/api/observations${qs}`)).then(json);
};

export const getRulesQueue = () => fetch(url('/api/rules/queue')).then(json);

export const decideRuleQueue = (id, status, edit = null) =>
  fetch(url(`/api/rules/queue/${id}/decide`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(edit ? { status, ...edit } : { status }),
  }).then(json);

export const batchRuleQueue = (body) =>
  fetch(url('/api/rules/queue/batch'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

export const getRuleQueueStats = () => fetch(url('/api/rules/queue/stats')).then(json);

export const getMemoryLedger = () => fetch(url('/api/memory/ledger')).then(json);

export const getMemoryStatus = () => fetch(url('/api/memory/status')).then(json);

export const purgeMemory = (selection, confirm) =>
  fetch(url('/api/memory/purge'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ ...selection, confirm }),
  }).then(json);

// Memory page controls. Actions are separate requests so every mutation is
// explicit and promotion only creates a pending Rule proposal.
export const getMemories = ({ q = '', kind = '', status = '' } = {}) => {
	const params = new URLSearchParams({ limit: '100' });
	if (q) params.set('q', q);
	if (kind) params.set('kind', kind);
	if (status) params.set('status', status);
	return fetch(url(`/api/memory/memories?${params}`)).then(json);
};
export const getNewMemoryCount = (since = 0) => fetch(url(`/api/memory/memories/count?since=${encodeURIComponent(since)}`)).then(json);
const memoryAction = (id, action, body = {}) => fetch(url(`/api/memory/memories/${id}/${action}`), { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }).then(json);
export const forgetMemory = (id) => memoryAction(id, 'forget');
export const parkMemory = (id) => memoryAction(id, 'park');
export const pinMemory = (id, pinned) => memoryAction(id, 'pin', { pinned });
export const promoteMemoryRule = (id) => memoryAction(id, 'promote');
export const editMemory = (id, text) => memoryAction(id, 'edit', { text });
export const promoteObservationMemory = (id) => fetch(url(`/api/memory/observations/${id}/promote`), { method: 'POST' }).then(json);
export const getEpisodes = () => fetch(url('/api/memory/episodes')).then(json);
// AGENTS.md or a new note file in the Session's worktree). Propose answers
// the queue item with its diff preview for approval; list reads the
// promotion queue; decide approves (writing the worktree file, never
// committing) or rejects. The Memory page proposes from the rules list; the
// chat action proposes from its own Session.
export const proposePromotion = (ruleId, sessionId, target = '') =>
  fetch(url('/api/promotions'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rule_id: ruleId, session_id: sessionId, target }),
  }).then(json);

export const listPromotions = () => fetch(url('/api/promotions/queue')).then(json);

export const decidePromotion = (id, status, opts = {}) =>
  fetch(url(`/api/promotions/queue/${id}/decide`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ status, ...opts }),
  }).then(json);

// Chat action: propose the Rule against that Session's own worktree, so the
// composer never leaves the chat to file a promotion.
export const promoteSessionRule = (sessionId, ruleId, target = '') =>
  fetch(url(`/api/sessions/${sessionId}/promote`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ rule_id: ruleId, target }),
  }).then(json);

export const listSpecs = (repo) => fetch(url(`/api/specs?repo=${encodeURIComponent(repo)}`)).then(json);

export const previewSpec = (spec) => fetch(url(`/api/specs/preview?spec=${encodeURIComponent(spec)}`)).then(json);

export const listRuns = () => fetch(url('/api/runs')).then(json);
export const listUsers = () => fetch(url('/api/users')).then(json);

export const startRun = (body) =>
  fetch(url('/api/runs'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(json);

export const getRun = (id) => fetch(url(`/api/runs/${id}`)).then(json);

export function openRunStream(id, onRun, opts = {}) {
  const source = new EventSource(url(`/api/runs/${id}/events`));
  source.onmessage = (ev) => {
    try {
      onRun(JSON.parse(ev.data));
    } catch {
      // Malformed line: skip, keep the stream alive.
    }
    try {
      opts.onMessage?.();
    } catch {
      // Hook must never break the stream.
    }
  };
  source.onerror = (ev) => {
    try {
      opts.onError?.(ev);
    } catch {
      // Hook must never break the stream.
    }
  };
  source.onopen = () => {
    try {
      opts.onOpen?.();
    } catch {
      // Hook must never break the stream.
    }
  };
  return source;
}

// Checkpoint actions on the Run's spec branch. Rebase reports conflicts in
// the returned Run snapshot; abandon is exposed only behind UI confirmation.
export const rebaseRun = (id) =>
  fetch(url(`/api/runs/${id}/rebase`), { method: 'POST' }).then(json);

export const abandonRun = async (id) => {
  const res = await fetch(url(`/api/runs/${id}/abandon`), { method: 'POST' });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 && typeof window !== 'undefined') window.dispatchEvent(new Event(UNAUTHORIZED));
    const err = new Error(body.error || `request failed: ${res.status}`);
    err.status = res.status;
    err.run = body.run;
    throw err;
  }
  return body;
};

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
export const switchRun = (id, { backend, model }) =>
  fetch(url(`/api/runs/${id}/switch`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ backend, model }),
  }).then(json);

export const pauseTicket = (runId, ticketId) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/pause`), { method: 'POST' }).then(json);

export const resumeTicket = (runId, ticketId) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/resume`), { method: 'POST' }).then(json);

export const switchTicket = (runId, ticketId, { backend, model }) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/switch`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ backend, model }),
  }).then(json);


// Per-ticket actions: steer reaches the worker, retry clears the
// needs-you banner and any park and starts the ticket over, park holds
// the ticket out of the driver, discard removes its worktree and branch.
// Each answers the fresh Run snapshot. text is required for steer and
// optional for retry.
export const steerTicket = (runId, ticketId, text) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/steer`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  }).then(json);

export const retryTicket = (runId, ticketId, text = '') =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/retry`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ text }),
  }).then(json);

export const parkTicket = (runId, ticketId) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/park`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  }).then(json);

export const discardTicket = (runId, ticketId) =>
  fetch(url(`/api/runs/${runId}/tickets/${encodeURIComponent(ticketId)}/discard`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
  }).then(json);

export const listProjects = () => fetch(url('/api/projects')).then(json);
// Ask Jev which Project fits the first prompt in a projectless draft. A null
// suggestedProject means the draft should start with its existing home path.
export const suggestProject = (prompt) =>
  fetch(url('/api/projects/suggest'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt }),
  }).then(json);

export const addProject = (body) =>
  fetch(url('/api/projects'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }).then(async (res) => {
    if (!res.ok) throw new Error((await res.text()).trim() || `request failed: ${res.status}`);
    return res.json();
  });

export const updateProject = (id, changes) =>
  fetch(url(`/api/projects/${id}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(changes),
  }).then(json);

export const renameProject = (id, name) => updateProject(id, { name });

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

// Clear a compress block without a compress. The messages it held go on.
export const dismissCompress = (id) => fetch(url(`/api/sessions/${id}/compress/dismiss`), { method: 'POST' }).then(json);

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

// The Quota tab: GET reads every provider's cached windows (guarded refresh);
// POST forces a fresh fetch past the age guard for one key, or all when empty.
export const getQuota = () => fetch(url('/api/quota')).then(json);

export const refreshQuota = (key = '') =>
  fetch(url('/api/quota/refresh'), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(key ? { key } : {}),
  }).then(json);

// Provider status never returns credentials. API-key writes travel once in the
// request body; responses contain only success metadata.
export const getProviders = () => fetch(url('/api/providers')).then(json);

export const setProviderKey = (provider, key) =>
  fetch(url(`/api/providers/${encodeURIComponent(provider)}/key`), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key }),
  }).then(json);

export const removeProviderKey = (provider) =>
  fetch(url(`/api/providers/${encodeURIComponent(provider)}/key`), { method: 'DELETE' }).then(json);

// ChatGPT's device-code login is kept in a short-lived Daemon session. The
// browser polls its status and deletes it when the admin cancels.
export const startChatGPTDeviceCodeLogin = () =>
  fetch(url('/api/providers/openai-codex/device-code'), { method: 'POST' }).then(json);

export const getChatGPTDeviceCodeLogin = (id) =>
  fetch(url(`/api/providers/openai-codex/device-code/${encodeURIComponent(id)}`)).then(json);

export const cancelChatGPTDeviceCodeLogin = (id) =>
  fetch(url(`/api/providers/openai-codex/device-code/${encodeURIComponent(id)}`), { method: 'DELETE' }).then(json);

// Claude's host login runs in a short-lived PTY owned by the Daemon. The
// browser reads terminal output and sends pasted codes without using pig auth.
export const startClaudeLogin = () =>
  fetch(url('/api/providers/claude/login'), { method: 'POST' }).then(json);

export const getClaudeLogin = (id) =>
  fetch(url(`/api/providers/claude/login/${encodeURIComponent(id)}`)).then(json);

export const sendClaudeLoginCode = (id, code) =>
  fetch(url(`/api/providers/claude/login/${encodeURIComponent(id)}/code`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  }).then(json);

export const cancelClaudeLogin = (id) =>
  fetch(url(`/api/providers/claude/login/${encodeURIComponent(id)}`), { method: 'DELETE' }).then(json);

// Antigravity's provider-owned OAuth flow is hosted as a short-lived Daemon
// login. The callback is posted once and status stays secret-free.
const antigravityConnectURL = '/api/providers/google-antigravity/connect';

export const startAntigravityLogin = () =>
  fetch(url(antigravityConnectURL), { method: 'POST' }).then(json);

export const getAntigravityLogin = (id) =>
  fetch(url(`${antigravityConnectURL}/${encodeURIComponent(id)}`)).then(json);

export const submitAntigravityLogin = (id, value) =>
  fetch(url(`${antigravityConnectURL}/${encodeURIComponent(id)}`), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ value }),
  }).then(json);

export const cancelAntigravityLogin = (id) =>
  fetch(url(`${antigravityConnectURL}/${encodeURIComponent(id)}`), { method: 'DELETE' }).then(json);

// The Daemon's build identity for the version badge. version.js wraps this
// with its never-throws fetch.
export const getVersion = () => fetch(url('/api/version')).then(json);

// Auth routes answer without a cookie. Failures carry the status and the
// Daemon's plain-text reason, which the sign-in screen shows.
async function authFailure(res) {
  const err = new Error((await res.text()).trim() || `request failed: ${res.status}`);
  err.status = res.status;
  return err;
}

const authJSON = async (res) => {
  if (!res.ok) throw await authFailure(res);
  return res.json();
};

const postCredentials = (path, username, password) =>
  fetch(url(path), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  }).then(authJSON);

// { empty, signup_enabled }: which mode the sign-in screen shows.
export const getAuthState = () => fetch(url('/api/auth/state')).then(authJSON);
// The signed-in User. Rejects with status 401 when signed out.
export const getMe = () => fetch(url('/api/auth/me')).then(authJSON);
export const login = (username, password) => postCredentials('/api/auth/login', username, password);
export const signup = (username, password) => postCredentials('/api/auth/signup', username, password);
export const logout = () =>
  fetch(url('/api/auth/logout'), { method: 'POST' }).then(async (res) => {
    if (!res.ok) throw await authFailure(res);
  });
