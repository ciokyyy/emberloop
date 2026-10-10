// Quota page helpers: threshold tone, reset countdown and the "updated"
// stamp. Pure functions so vitest covers them without the DOM.
//
// The thresholds mirror el.ThresholdClass in emberloop/header.go: calm below
// 60%, warm below 80%, hot at 80% and up. The header strip paints its bars
// from the same cut, so the two read the same.

export function thresholdClass(pct) {
  const n = Number(pct) || 0;
  if (n < 60) return 'calm';
  if (n < 80) return 'warm';
  return 'hot';
}

// resetCountdown mirrors the header strip's usageResetIn: "4d2h", "1h40m",
// "25m", "now" once the reset has passed, "" when there is no reset time.
// nowMs is epoch milliseconds; resetsAt is epoch seconds.
export function resetCountdown(resetsAt, nowMs = Date.now()) {
  if (!resetsAt || resetsAt <= 0) return '';
  const secs = Math.floor(resetsAt - nowMs / 1000);
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

// staleLabel is the Claude card's age stamp: "updated 12m ago". Claude is
// push-only, so this warns the value may already be old. updatedAt is epoch
// seconds; 0 means the file was never written.
export function staleLabel(updatedAt, nowMs = Date.now()) {
  if (!updatedAt || updatedAt <= 0) return 'never updated';
  const secs = Math.max(0, Math.floor(nowMs / 1000) - updatedAt);
  if (secs < 60) return 'updated just now';
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `updated ${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `updated ${hours}h ago`;
  return `updated ${Math.floor(hours / 24)}d ago`;
}
