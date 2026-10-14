// Quota presentation helpers. Percent values and reset times come from the
// Daemon contract; these helpers only format them for display.

const WINDOW_LABELS = new Map([
  ['5h', '5-hour'],
  ['5 hour', '5-hour'],
  ['five hour', '5-hour'],
  ['week', 'Weekly'],
  ['wk', 'Weekly'],
  ['weekly', 'Weekly'],
  ['seven day', 'Weekly'],
  ['month', 'Monthly'],
  ['mo', 'Monthly'],
  ['monthly', 'Monthly'],
  ['spend week', 'Spend this week'],
  ['spend this week', 'Spend this week'],
]);

const DEFAULT_PLANS = {
  claude: 'Pro',
  chatgpt: 'Plus',
  'openai-codex': 'Plus',
  'opencode-go': 'Go',
  openrouter: 'Credits',
  'agy-gemini': 'Gemini + 3P',
  'agy-3p': 'Gemini + 3P',
};

const DEFAULT_SOURCES = {
  claude: 'Host login',
  chatgpt: 'pig login',
  'openai-codex': 'pig login',
  'opencode-go': 'API key',
  openrouter: 'API key',
  'agy-gemini': 'pig login',
  'agy-3p': 'pig login',
};

function epochSeconds(value) {
  if (value === null || value === undefined || value === '') return Number.NaN;
  const numeric = Number(value);
  if (Number.isFinite(numeric)) return numeric;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed / 1000 : Number.NaN;
}

// The header strip uses the same cuts: calm below 60%, warm below 80%, hot
// from 80% up. A missing/invalid value is not a percent and stays calm.
export function thresholdClass(pct) {
  const n = Number(pct);
  if (!Number.isFinite(n) || n < 60) return 'calm';
  if (n < 80) return 'warm';
  return 'hot';
}

export function hasPercent(pct) {
  return pct !== null && pct !== undefined && pct !== '' && Number.isFinite(Number(pct));
}

export function wordLabel(label) {
  const raw = String(label ?? '').trim();
  const key = raw.toLowerCase().replace(/[_-]+/g, ' ');
  return WINDOW_LABELS.get(key) ?? raw;
}

// nowMs is epoch milliseconds; resetsAt is epoch seconds (or an ISO time).
export function resetCountdown(resetsAt, nowMs = Date.now()) {
  const reset = epochSeconds(resetsAt);
  if (!Number.isFinite(reset) || reset <= 0) return '';
  const secs = Math.floor(reset - nowMs / 1000);
  if (secs <= 0) return 'now';
  if (secs >= 86400) {
    const days = Math.floor(secs / 86400);
    const hours = Math.floor((secs % 86400) / 3600);
    return `${days}d${hours}h`;
  }
  if (secs >= 3600) {
    const hours = Math.floor(secs / 3600);
    const mins = Math.floor((secs % 3600) / 60);
    return `${hours}h${String(mins).padStart(2, '0')}m`;
  }
  return `${Math.ceil(secs / 60)}m`;
}

// updatedAt is epoch seconds; 0 means that no value has been read yet.
export function staleLabel(updatedAt, nowMs = Date.now()) {
  const updated = epochSeconds(updatedAt);
  if (!Number.isFinite(updated) || updated <= 0) return 'never updated';
  const secs = Math.max(0, Math.floor(nowMs / 1000) - updated);
  if (secs === 0) return 'updated just now';
  if (secs < 60) return `updated ${secs}s ago`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `updated ${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `updated ${hours}h ago`;
  return `updated ${Math.floor(hours / 24)}d ago`;
}

export function quotaState(provider) {
  const state = provider?.state || 'ok';
  if (state === 'not_configured' || state === 'not_connected') return 'not_connected';
  if (state === 'error' && (provider?.windows ?? []).length > 0) return 'stale';
  return state;
}

export function quotaPlan(provider) {
  return provider?.plan || DEFAULT_PLANS[provider?.key] || 'Plan';
}

export function quotaSource(provider) {
  return provider?.source || DEFAULT_SOURCES[provider?.key] || 'Account';
}

export function quotaHint(provider) {
  if (provider?.hint) return provider.hint;
  if (provider?.key === 'agy-gemini' || provider?.key === 'agy-3p') {
    return 'Needs ~/.el/agy-oauth.json from the pig-antigravity-auth login. Shows the Gemini and Claude + GPT-OSS pools, each with a 5-hour and a weekly bar.';
  }
  return 'Connect this account in Providers to see its quota.';
}

export function quotaUpdatedLine(provider, nowMs = Date.now()) {
  const state = quotaState(provider);
  const age = staleLabel(provider?.updated_at, nowMs);
  if (state === 'not_connected') return quotaSource(provider);
  if (state === 'expired') return `last read ${age.replace(/^updated /, '')}`;
  if (state === 'stale') return `${age} · retrying`;
  return `${age} · ${quotaSource(provider)}`;
}
