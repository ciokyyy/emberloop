// Pure dependency-graph helpers for the Runs view. No Svelte, no DOM:
// layoutRunGraph turns a Run's live tickets into dagre positions, and the
// RunsGraph component renders them (SVG edges, HTML cards, pan/zoom).
// Blockers resolve by ticket id or number; unknown blockers count as
// unmerged, so the node reads blocked instead of silently floating free.
import dagre from '@dagrejs/dagre';

// Fixed card footprint the layout reserves. The cards render at this size
// (title clamps to two lines) so an SSE status flip never moves a node.
export const GRAPH_NODE_W = 240;
export const GRAPH_NODE_H = 92;

// Graph tones: merged green, running blue, needs-you amber, blocked dim,
// parked/discarded muted, queued neutral.
export function ticketById(tickets) {
  return Object.fromEntries((tickets ?? []).map((t) => [t.id, t]));
}

export function resolveBlocker(byId, ref) {
  if (byId[ref]) return byId[ref];
  return Object.values(byId).find((t) => String(t.num) === String(ref)) ?? null;
}

function blockerMerged(byId, ref) {
  const t = resolveBlocker(byId, ref);
  return !!t && t.state === 'merged';
}

// Driver states are queued/running/in-review/merging/needs-you/parked/
// merged/discarded. "blocked" is derived: a queued ticket whose blockers
// are not all merged yet.
export function nodeStatus(ticket, byId) {
  const s = ticket.state ?? 'queued';
  if (s === 'merged') return 'merged';
  if (s === 'needs-you') return 'needs-you';
  if (s === 'parked' || s === 'discarded') return 'parked';
  if (s === 'running' || s === 'in-review' || s === 'merging') return 'running';
  const blockers = ticket.blockers ?? [];
  if (blockers.length && blockers.some((b) => !blockerMerged(byId, b))) return 'blocked';
  return 'queued';
}

// A blocker id reads as its leading ticket number on a card ("01"), the full
// id stays in the aria label and the table.
function shortRef(ref) {
  const m = /^(\d+)[-_:\s]/.exec(String(ref));
  return m ? m[1] : String(ref);
}

export function statusLabel(ticket, status) {
  if (status === 'blocked') return `blocked by ${(ticket.blockers ?? []).map(shortRef).join(', ') || '…'}`;
  if (status === 'parked') return ticket.state ?? 'parked';
  if (status === 'queued') return 'queued';
  return status;
}

// One-line card subtitle: the needs-you reason when stalled, else a short
// reviewer-verdict summary when there is one.
export function nodeSubtitle(ticket) {
  if ((ticket.needsYou ?? '') || (ticket.lastRefusal ?? '')) return ticket.needsYou || ticket.lastRefusal;
  const v = ticket.verdicts ?? {};
  const bits = ['reviewer-spec', 'reviewer-standards']
    .map((k) => (v[k] ? `${k === 'reviewer-spec' ? 'spec' : 'standards'} ${v[k].verdict}${v[k].fresh ? '' : ' (stale)'}` : ''))
    .filter(Boolean);
  return bits.join(' · ');
}

export function graphEdges(tickets) {
  const byId = ticketById(tickets);
  const seen = new Set();
  const edges = [];
  for (const t of tickets ?? []) {
    for (const b of t.blockers ?? []) {
      const src = resolveBlocker(byId, b);
      if (!src || src.id === t.id) continue;
      const key = `${src.id}→${t.id}`;
      if (seen.has(key)) continue;
      seen.add(key);
      edges.push({ from: src.id, to: t.id });
    }
  }
  return edges;
}

// Layout identity: ids plus blocker lists. Statuses, tips and verdicts do
// not move nodes, so SSE updates keep positions stable while this matches.
export function structureKey(tickets) {
  return (tickets ?? [])
    .map((t) => `${t.id}<${(t.blockers ?? []).join(',')}`)
    .sort()
    .join(';');
}

export function layoutRunGraph(tickets, opts = {}) {
  const list = dedupeTickets(tickets ?? []);
  if (!list.length) return { nodes: [], edges: [], width: 0, height: 0 };
  const g = new dagre.graphlib.Graph();
  g.setGraph({
    rankdir: 'LR',
    nodesep: opts.nodesep ?? 28,
    ranksep: opts.ranksep ?? 72,
    marginx: 24,
    marginy: 24,
  });
  g.setDefaultEdgeLabel(() => ({}));
  const byId = ticketById(list);
  for (const t of list) g.setNode(t.id, { width: GRAPH_NODE_W, height: GRAPH_NODE_H });
  for (const e of graphEdges(list)) g.setEdge(e.from, e.to);
  dagre.layout(g);
  const graph = g.graph();
  const nodes = list.map((t) => {
    const n = g.node(t.id);
    const status = nodeStatus(t, byId);
    return {
      id: t.id,
      num: t.num,
      title: t.title,
      state: t.state,
      status,
      label: statusLabel(t, status),
      subtitle: nodeSubtitle(t),
      blockers: t.blockers ?? [],
      x: n.x - n.width / 2,
      y: n.y - n.height / 2,
      cx: n.x,
      cy: n.y,
      width: n.width,
      height: n.height,
      ticket: t,
    };
  });
  const edges = g.edges().map((e) => ({ from: e.v, to: e.w, points: g.edge(e).points ?? [] }));
  return { nodes, edges, width: graph.width ?? 0, height: graph.height ?? 0 };
}

// Duplicate ticket ids collapse to one node (first wins): dagre keys nodes
// by id and Svelte keys cards by id, so dupes would either stack or throw.
export function dedupeTickets(tickets) {
  const seen = new Set();
  const out = [];
  for (const t of tickets ?? []) {
    if (seen.has(t?.id)) continue;
    seen.add(t?.id);
    out.push(t);
  }
  return out;
}

// Smooth horizontal cubic through dagre's route points (LR ranks).
export function edgePath(points) {
  const pts = points ?? [];
  if (!pts.length) return '';
  if (pts.length === 1) return `M${pts[0].x} ${pts[0].y}`;
  let d = `M${pts[0].x} ${pts[0].y}`;
  for (let i = 1; i < pts.length; i++) {
    const a = pts[i - 1];
    const b = pts[i];
    const dx = Math.max(Math.abs(b.x - a.x) / 2, 8);
    d += ` C ${a.x + dx} ${a.y}, ${b.x - dx} ${b.y}, ${b.x} ${b.y}`;
  }
  return d;
}

export function countByStatus(nodes) {
  const counts = {};
  for (const n of nodes ?? []) counts[n.status] = (counts[n.status] ?? 0) + 1;
  return counts;
}
