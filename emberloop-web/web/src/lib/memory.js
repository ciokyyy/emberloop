// Memory page helpers: pure normalizers and queue shaping so tests cover
// them without the DOM. The Daemon's older queue routes answer with Go field
// names (ID, Kind, Payload, Status) while the new Memory page routes answer
// lowercase (id, kind, payload, status); every normalizer accepts both so the
// page never blanks on a bank the settings view already reads.
export function normId(item) {
  if (item == null) return 0;
  const v = item.id ?? item.ID;
  const n = Number(v);
  return Number.isFinite(n) ? n : 0;
}

export function normStatus(item) {
  return String(item?.status ?? item?.Status ?? '');
}

export function normKind(item) {
  return String(item?.kind ?? item?.Kind ?? '');
}

export function normPayload(item) {
  return String(item?.payload ?? item?.Payload ?? '');
}

// normalizeQueueItem folds one queue row to the lowercase shape the page
// renders: { id, kind, payload, status, decidedBy, createdAt }.
export function normalizeQueueItem(item) {
  if (!item) return { id: 0, kind: '', payload: '', status: '' };
  return {
    id: normId(item),
    kind: normKind(item),
    payload: normPayload(item),
    status: normStatus(item),
    decidedBy: String(item.decidedBy ?? item.DecidedBy ?? ''),
    createdAt: String(item.createdAt ?? item.CreatedAt ?? ''),
  };
}

// parseProposal decodes one rule-queue payload into the proposal the approver
// sees: the text plus its quotes and source Sessions. Old remember payloads
// carry a single quote and sessionId; newer ones carry quotes and sessions.
// Unknown JSON gives an empty proposal, never a throw.
export function parseProposal(payload) {
  let p = {};
  try {
    p = JSON.parse(String(payload ?? '')) || {};
  } catch {
    return { text: '', quotes: [], sessions: [], reason: '', author: '', subjectKey: '', kind: '', relation: '', relatedRuleId: '' };
  }
  const text = String(p.text ?? '');
  const quotes = Array.isArray(p.quotes)
    ? p.quotes.map(String)
    : p.quote
      ? [String(p.quote)]
      : [];
  const sessions = Array.isArray(p.sessions)
    ? p.sessions.map(String)
    : p.sessionId
      ? [String(p.sessionId)]
      : [];
  return {
    text,
    quotes,
    sessions,
    reason: String(p.reason ?? ''),
    author: String(p.author ?? ''),
    subjectKey: String(p.subject_key ?? p.subjectKey ?? ''),
    kind: String(p.kind ?? ''),
    relation: String(p.relation ?? ''),
    relatedRuleId: String(p.related_rule_id ?? p.relatedRuleId ?? ''),
  };
}

// normalizeRule folds one Rule row to lowercase keys.
export function normalizeRule(r) {
  if (!r) return { ruleId: '', version: 0, text: '', kind: '', subjectKey: '', status: '' };
  return {
    ruleId: String(r.ruleId ?? r.RuleID ?? ''),
    version: Number(r.version ?? r.Version ?? 0),
    text: String(r.text ?? r.Text ?? ''),
    kind: String(r.kind ?? r.Kind ?? ''),
    subjectKey: String(r.subjectKey ?? r.SubjectKey ?? ''),
    status: String(r.status ?? r.Status ?? ''),
    anchors: r.anchors ?? r.Anchors ?? [],
    observedCommit: String(r.observedCommit ?? r.ObservedCommit ?? ''),
  };
}

// normalizeObservation folds one observation row to lowercase keys.
export function normalizeObservation(o) {
  if (!o) return { id: 0, quote: '', sessionId: '' };
  return {
    id: Number(o.id ?? o.ID ?? 0),
    sessionId: String(o.sessionId ?? o.SessionID ?? ''),
    eventId: String(o.eventId ?? o.EventID ?? ''),
    quote: String(o.quote ?? o.Quote ?? ''),
    kind: String(o.kind ?? o.Kind ?? ''),
    subjectKey: String(o.subjectKey ?? o.SubjectKey ?? ''),
    evidenceCount: Number(o.evidenceCount ?? o.EvidenceCount ?? 0),
  };
}

// normalizeLedger folds one ledger row to lowercase keys.
export function normalizeLedger(l) {
  if (!l) return { id: 0, runId: '', stage: '', status: '' };
  return {
    id: Number(l.id ?? l.ID ?? 0),
    runId: String(l.runId ?? l.RunID ?? ''),
    stage: String(l.stage ?? l.Stage ?? ''),
    status: String(l.status ?? l.Status ?? ''),
    detail: String(l.detail ?? l.Detail ?? ''),
    createdAt: String(l.createdAt ?? l.CreatedAt ?? ''),
  };
}

// groupQueueBySession buckets pending rule items by their first source
// Session, so the page offers one batch-approve button per Session. Items
// without a Session land under "" and batch by IDs only.
export function groupQueueBySession(items) {
  const groups = new Map();
  for (const raw of items ?? []) {
    const item = normalizeQueueItem(raw);
    if (item.status !== 'pending' || item.kind !== 'rule') continue;
    const proposal = parseProposal(item.payload);
    const key = proposal.sessions[0] ?? '';
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push({ item, proposal });
  }
  return [...groups.entries()].map(([sessionId, rows]) => ({ sessionId, rows }));
}

// purgeCounts summarizes a purge selection for the confirm line.
export function purgeCounts(selection) {
  const n = (v) => (Array.isArray(v) ? v.length : 0);
  return {
    memories: n(selection?.memories),
    episodes: n(selection?.episodes),
    observations: n(selection?.observations),
    rules: n(selection?.rules),
    queue: n(selection?.queue),
    ledger: n(selection?.ledger),
    total: n(selection?.observations) + n(selection?.memories) + n(selection?.episodes) + n(selection?.rules) + n(selection?.queue) + n(selection?.ledger),
  };
}

// purgeSummary renders what a purge removed: "1 rule, 2 observations".
export function purgeSummary(result) {
  const parts = [];
  const push = (n, one, many) => {
    if (n > 0) parts.push(`${n} ${n === 1 ? one : many}`);
  };
  push(Number(result?.memories ?? result?.Memories ?? 0), 'Memory', 'Memories');
  push(Number(result?.episodes ?? result?.Episodes ?? 0), 'Episode', 'Episodes');
  push(Number(result?.rules ?? result?.Rules ?? 0), 'rule', 'rules');
  push(Number(result?.observations ?? result?.Observations ?? 0), 'observation', 'observations');
  push(Number(result?.queue ?? result?.Queue ?? 0), 'queue item', 'queue items');
  push(Number(result?.ledger ?? result?.Ledger ?? 0), 'ledger row', 'ledger rows');
  if (parts.length === 0) return 'Nothing removed.';
  return `Removed ${parts.join(', ')}.`;
}

// latencyLabel renders approval latency hours as "1.5h" or "—" when none.
export function latencyLabel(hours) {
  const n = Number(hours);
  if (!Number.isFinite(n) || n <= 0) return '—';
  return `${Math.round(n * 10) / 10}h`;
}
