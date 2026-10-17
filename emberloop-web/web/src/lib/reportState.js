const emptyState = () => ({ reports: [], dismissed: [], readSeq: 0, latestSeq: 0 });

export function reportStateKey(userId, sessionId) {
  return `eb.reports:${encodeURIComponent(String(userId ?? ''))}:${encodeURIComponent(String(sessionId ?? ''))}`;
}

// Report state is private to this browser, User and Session. The report body is
// retained as well as its sequence so the tray can restore older tail Events.
export function readReportState(userId, sessionId) {
  if (!userId || !sessionId || typeof localStorage === 'undefined') return emptyState();
  try {
    const value = JSON.parse(localStorage.getItem(reportStateKey(userId, sessionId)) ?? '{}');
    return {
      reports: Array.isArray(value.reports) ? value.reports.filter((r) => r && Number.isFinite(Number(r.seq))) : [],
      dismissed: Array.isArray(value.dismissed) ? value.dismissed.map(Number).filter(Number.isFinite) : [],
      readSeq: Math.max(0, Number(value.readSeq) || 0),
      latestSeq: Math.max(0, Number(value.latestSeq) || 0),
    };
  } catch {
    return emptyState();
}
}

export function writeReportState(userId, sessionId, state) {
  if (!userId || !sessionId || typeof localStorage === 'undefined') return;
  try {
    localStorage.setItem(reportStateKey(userId, sessionId), JSON.stringify(state));
  } catch {
    // Private browsing or a full storage quota keeps the current view usable.
  }
}

export function rememberReports(state, reports = []) {
  const bySeq = new Map(state.reports.map((r) => [Number(r.seq), r]));
  for (const report of reports) {
    if (!report || !Number.isFinite(Number(report.seq))) continue;
    bySeq.set(Number(report.seq), { ...report, seq: Number(report.seq) });
  }
  return { ...state, reports: [...bySeq.values()].sort((a, b) => a.seq - b.seq), latestSeq: Math.max(state.latestSeq ?? 0, ...bySeq.keys()) };
}

export function setReportDismissed(state, seq, dismissed) {
  const id = Number(seq);
  if (!Number.isFinite(id)) return state;
  const values = new Set(state.dismissed);
  if (dismissed) values.add(id);
  else values.delete(id);
  return { ...state, dismissed: [...values].sort((a, b) => a - b) };
}

export function markReportsRead(state, seq) {
  return { ...state, readSeq: Math.max(state.readSeq, Number(seq) || 0) };
}

export function isReportUnread(userId, sessionId, reportSeq = 0) {
  const state = readReportState(userId, sessionId);
  return Math.max(Number(reportSeq) || 0, state.latestSeq) > state.readSeq;
}
