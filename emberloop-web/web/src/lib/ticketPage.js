// Ticket-page helpers for RunsView's /runs/<runId>/<ticketId> route. Pure
// functions only, so vitest covers the state mapping and round rendering
// without the DOM. Shapes follow the ticket fields GET /api/runs/{id} and
// its SSE snapshot publish (documented in emberloop-web/AGENTS.md): state,
// stateSince, boxes, body, title, branch, tip, gate, verdicts, agents,
// rounds[], followups, needsYou, lastRefusal.
export const STATE_STEPS = [
  { id: 'queued', label: 'queued' },
  { id: 'running', label: 'running' },
  { id: 'in-review', label: 'in review' },
  { id: 'merging', label: 'merging' },
  { id: 'merged', label: 'merged' },
];

// The reviewer types the Daemon gates on, in gate order.
export const REQUIRED_REVIEWERS = ['reviewer-standards', 'reviewer-spec'];

export function shortReviewerType(type) {
  if (type === 'reviewer-spec') return 'spec';
  if (type === 'reviewer-standards') return 'standards';
  return String(type ?? '');
}

export function shortTip(tip) {
  const t = String(tip ?? '');
  return t ? t.slice(0, 7) : '—';
}

export function findTicket(run, ticketId) {
  return run?.tickets?.find((t) => t.id === ticketId) ?? null;
}

export function roundNumber(ticket) {
  return ticket?.rounds?.length ?? 0;
}

export function currentRound(ticket) {
  const rounds = ticket?.rounds ?? [];
  return rounds.length ? rounds[rounds.length - 1] : null;
}

// stateTrack maps a ticket state onto the five-step track (queued, running,
// in review round N, merging, merged). States off the track surface as an
// alert pill: needs-you keeps the in-review step current, parked/discarded
// and anything unknown (e.g. paused) mark no step current.
export function stateTrack(ticket) {
  const state = ticket?.state ?? 'queued';
  const steps = STATE_STEPS.map((s) => ({ ...s, state: 'todo' }));
  const byId = Object.fromEntries(steps.map((s) => [s.id, s]));
  const doneThrough = (id) => {
    for (const s of steps) {
      if (s.id === id) break;
      s.state = 'done';
    }
  };
  let alert = null;
  let currentLabel = state;
  switch (state) {
    case 'queued':
      byId.queued.state = 'current';
      currentLabel = 'queued';
      break;
    case 'running':
      doneThrough('running');
      byId.running.state = 'current';
      currentLabel = 'running';
      break;
    case 'in-review': {
      doneThrough('in-review');
      byId['in-review'].state = 'current';
      const n = roundNumber(ticket);
      currentLabel = n > 0 ? `in review · round ${n}` : 'in review';
      break;
    }
    case 'merging':
      doneThrough('merging');
      byId.merging.state = 'current';
      currentLabel = 'merging';
      break;
    case 'merged':
      for (const s of steps) s.state = 'done';
      currentLabel = 'merged';
      break;
    case 'needs-you':
      doneThrough('in-review');
      byId['in-review'].state = 'current';
      alert = { kind: 'needs-you', label: 'needs you' };
      currentLabel = 'needs you';
      break;
    case 'parked':
      alert = { kind: 'parked', label: 'parked' };
      currentLabel = 'parked';
      break;
    case 'discarded':
      alert = { kind: 'discarded', label: 'discarded' };
      currentLabel = 'discarded';
      break;
    default:
      alert = { kind: state, label: state };
      currentLabel = state;
  }
  return { steps, alert, currentLabel };
}

// reviewCards renders one round as its reviewer cards in gate order: two
// cards per round, a held same-tip pass ("pass held", never re-run) and a
// pending card for a reviewer with no verdict yet (currently re-judging).
export function reviewCards(round, required = REQUIRED_REVIEWERS) {
  const types = required.length ? required : Object.keys(round?.reviews ?? {});
  return types.map((type) => {
    const r = round?.reviews?.[type];
    if (!r) {
      return {
        type,
        short: shortReviewerType(type),
        verdict: 'pending',
        held: false,
        heldFromRound: null,
        findings: [],
        durationMs: 0,
        at: '',
      };
    }
    return {
      type,
      short: shortReviewerType(type),
      verdict: r.verdict ?? '',
      held: r.heldFromRound != null,
      heldFromRound: r.heldFromRound ?? null,
      findings: r.findings ?? [],
      durationMs: r.durationMs ?? 0,
      at: r.at ?? '',
    };
  });
}

// gateSummary reads the gate the finish needs: one row per required type
// with its verdict and whether it still covers the tip (fresh) or holds an
// older one. Missing verdicts are pending.
export function gateSummary(ticket) {
  const gate = ticket?.gate ?? {};
  const required = gate.required?.length ? gate.required : REQUIRED_REVIEWERS;
  const verdicts = gate.verdicts ?? ticket?.verdicts ?? {};
  return required.map((type) => {
    const v = verdicts[type];
    if (!v) return { type, short: shortReviewerType(type), verdict: 'pending', fresh: false, at: '' };
    return {
      type,
      short: shortReviewerType(type),
      verdict: v.verdict ?? 'pending',
      fresh: !!v.fresh,
      at: v.at ?? '',
    };
  });
}

export function boxProgress(boxes) {
  const list = boxes ?? [];
  return { done: list.filter((b) => b.checked).length, total: list.length };
}

// dropPendingEcho removes the single pending send an echoed prompt Event
// confirms. Matching is exact (text plus attachment identity) so one echo
// drops one pending: duplicate pending texts drain one at a time, a partial
// overlap drops nothing, and an image-only pending (empty text with
// attachments) still matches its echo instead of lingering as a ghost.
export function dropPendingEcho(list, e) {
  const text = String(e?.text ?? '');
  const images = Number(e?.images ?? 0) || 0;
  const ids = [...(e?.imageIds ?? [])].map(String).sort().join(',');
  const mode = String(e?.mode ?? '');
  const i = (list ?? []).findIndex((p) => {
    if (String(p?.text ?? '') !== text) return false;
    if ((Number(p?.images ?? 0) || 0) !== images) return false;
    if ([...(p?.imageIds ?? [])].map(String).sort().join(',') !== ids) return false;
    const pm = String(p?.mode ?? '');
    if (pm && mode && pm !== mode) return false;
    return true;
  });
  if (i === -1) return list;
  return list.filter((_, j) => j !== i);
}

// Terminal ticket states: the Run SSE snapshot will not move them again,
// so the page can close the stream like RunsView does for finished Runs.
export function ticketFinished(ticket) {
  return ticket?.state === 'merged' || ticket?.state === 'discarded';
}

export function runFinished(run) {
  return run?.status === 'done' || run?.status === 'failed' || run?.status === 'abandoned';
}

// agentsOf lists one ticket's sessions: worker, both reviewers, scouts.
// Older snapshots without agents synthesize the worker and reviewers from
// the session id fields so the panel never comes up empty.
export function agentsOf(ticket) {
  if (ticket?.agents?.length) return ticket.agents;
  const out = [];
  if (ticket?.workerSession) {
    out.push({ sessionId: ticket.workerSession, role: 'worker', harness: '', status: '', name: '' });
  }
  for (const [type, sid] of Object.entries(ticket?.reviewerSessions ?? {})) {
    if (sid) out.push({ sessionId: sid, role: type, harness: '', status: '', name: '' });
  }
  return out;
}

export function agentRoleLabel(role) {
  if (role === 'worker') return 'worker';
  if (role === 'scout') return 'scout';
  if (String(role ?? '').startsWith('reviewer')) return 'reviewer';
  return String(role ?? '');
}

// sinceLabel renders a stateSince timestamp as a short elapsed label ("11m").
// now is injectable for tests.
export function sinceLabel(iso, now = Date.now()) {
  const t = new Date(iso ?? '').getTime();
  if (!Number.isFinite(t)) return '';
  const s = Math.max(0, Math.floor((now - t) / 1000));
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 48) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

// ticketTypeOf is the type a Ticket runs as: an override wins over the file.
export function ticketTypeOf(ticket) {
  return ticket?.effectiveType || ticket?.type || 'code';
}

// blockersOpen lists the blockers of a Ticket that have not merged yet. A
// blocker names a Ticket by id or number; unknown refs count as open.
export function blockersOpen(ticket, run) {
  const all = run?.tickets ?? [];
  return (ticket?.blockers ?? []).filter((ref) => {
    const hit = all.find((t) => t.id === ref || String(t.num) === String(ref));
    return !hit || hit.state !== 'merged';
  });
}

// nextStep says what the Ticket is doing and what the User does next, in one
// short line: the lead of the Ticket page. `tone` is a Chip tone, `action` is
// the one control the page can raise for it (steer, unpark, or none).
export function nextStep(ticket, run) {
  if (!ticket) return null;
  const state = ticket.state ?? 'queued';
  const runStatus = run?.status ?? '';
  if (state === 'merged') return { tone: 'done', label: 'Merged', next: 'Nothing to do.', action: 'none' };
  if (state === 'discarded') return { tone: 'neutral', label: 'Discarded', next: 'Nothing to do.', action: 'none' };
  if (state === 'parked') return { tone: 'paused', label: 'Parked', next: 'Unpark to continue.', action: 'unpark' };
  if (state === 'needs-you' || ticket.needsYou) {
    return { tone: 'needs-you', label: 'Needs you', next: ticket.needsYou || 'Steer the worker or retry.', action: 'steer' };
  }
  if (state === 'paused' || runStatus === 'paused') {
    return { tone: 'paused', label: 'Paused', next: 'Resume the Run to continue.', action: 'none' };
  }
  if (runStatus === 'failed' || runStatus === 'abandoned') {
    return { tone: 'failed', label: runStatus === 'failed' ? 'Run failed' : 'Run abandoned', next: 'Open the Run to resume.', action: 'none' };
  }
  if (state === 'queued') {
    const open = blockersOpen(ticket, run);
    return {
      tone: 'neutral',
      label: 'Queued',
      next: open.length ? `Waiting on ${open.length} ${open.length === 1 ? 'blocker' : 'blockers'}.` : 'Starts next.',
      action: 'none',
    };
  }
  if (state === 'in-review') {
    const n = roundNumber(ticket);
    return { tone: 'working', label: n > 0 ? `In review · round ${n}` : 'In review', next: 'Waiting on reviewers.', action: 'none' };
  }
  if (state === 'merging') return { tone: 'working', label: 'Merging', next: 'Merging into the branch.', action: 'none' };
  return { tone: 'working', label: 'Running', next: 'Worker is working. Steer if it drifts.', action: 'steer' };
}

// shortRef shortens a blocker ref to its leading Ticket number ("01"), as the
// graph cards do; blockersLabel joins them into the "after 01, 02" chip text.
export function shortRef(ref) {
  return /^(\d+)[-_:\s]/.exec(String(ref))?.[1] ?? String(ref);
}

export function blockersLabel(blockers) {
  return `after ${(blockers ?? []).map(shortRef).join(', ')}`;
}
