// Sidebar helpers: attention tiers, the Session tree, chat filtering and the
// persisted collapse prefs. Pure except the localStorage helpers; the
// components stay thin and the tests pin behavior here.

// Attention tier for one Session row. Tiering reads only the status the
// session list already carries: failed (or a needsYou flag, when the Daemon
// reports one) needs you, working is working, everything else is recent.
export function tierOf(s = {}) {
  if (s.needsYou || s.status === 'failed' || s.status === 'needs-you') return 'needs';
  if (s.status === 'working') return 'working';
  return 'recent';
}

export const TIER_ORDER = ['needs', 'working', 'recent'];

export const TIER_LABEL = {
  needs: 'Needs you',
  working: 'Working',
  recent: 'Recent',
};

// Split a flat session list into tier buckets, keeping the incoming order
// (the Daemon already sorts newest activity first) inside each tier.
export function splitTiers(sessions = []) {
  const tiers = { needs: [], working: [], recent: [] };
  for (const s of sessions) tiers[tierOf(s)].push(s);
  return tiers;
}

// Tier buckets of tree roots. A Subagent never gets its own row in a tier:
// it always renders nested under its parent, and the parent sits in the most
// urgent tier among itself and its children, so a working child still lifts
// an idle parent into Working. Orphans (parent outside the list) are roots.
export function rootTiers(sessions = []) {
  const { roots, kids } = buildTree(sessions);
  const rank = (t) => TIER_ORDER.indexOf(t);
  const out = { needs: [], working: [], recent: [] };
  for (const r of roots) {
    let best = tierOf(r);
    for (const k of kids.get(r.id) ?? []) if (rank(tierOf(k)) < rank(best)) best = tierOf(k);
    out[best].push(r);
  }
  return out;
}

// Build the one-level Subagent tree: roots plus children by parent id.
// Orphans (a parent outside this list) stay roots, same as before.
export function buildTree(list = []) {
  const byId = new Map(list.map((s) => [s.id, s]));
  const kids = new Map();
  const roots = [];
  for (const s of list) {
    const parent = s.parentId;
    if (parent && byId.has(parent)) {
      if (!kids.has(parent)) kids.set(parent, []);
      kids.get(parent).push(s);
    } else {
      roots.push(s);
    }
  }
  return { roots, kids };
}

// A Group reads as all done when every child settled (ended or failed), so
// its parent row defaults collapsed. The sidebar has no child Events, so an
// idle child still counts as running.
export function groupAllDone(kids = []) {
  return kids.length > 0 && kids.every((k) => k.status === 'ended' || k.status === 'failed');
}

// Row label: Subagents show the spawn name (then the generated title); roots
// keep the harness label. The full id stays a tooltip, never a child label.
export function sessionLabel(s = {}) {
  return s.name || s.title || `${s.harness} · ${String(s.id ?? '').slice(0, 8)}`;
}

// Filter match for the chat search box: label, harness or project name.
export function matchSession(s = {}, projectName = '', q = '') {
  const needle = String(q ?? '').trim().toLowerCase();
  if (!needle) return true;
  return (
    sessionLabel(s).toLowerCase().includes(needle) ||
    String(s.harness ?? '').toLowerCase().includes(needle) ||
    String(projectName ?? '').toLowerCase().includes(needle)
  );
}

export function readJSON(key, fallback) {
  try {
    const raw = localStorage.getItem(key);
    return raw == null ? fallback : (JSON.parse(raw) ?? fallback);
  } catch {
    return fallback;
  }
}

export function writeJSON(key, value) {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    // Private mode: prefs just do not persist.
  }
}

// A persisted set of ids (collapsed projects, tiers, drawer state): arrays on
// disk, Sets in memory.
export function loadIdSet(key) {
  const raw = readJSON(key, []);
  return new Set(Array.isArray(raw) ? raw : []);
}

export function storeIdSet(key, set) {
  writeJSON(key, [...set]);
}

// Collapsed preference with a dynamic default: an explicit override wins,
// otherwise the caller-supplied default applies (subagent parents default to
// their groupAllDone state, which moves as children settle).
export function isCollapsed(prefs = {}, id, defaultCollapsed = false) {
  if (prefs[id] === true) return true;
  if (prefs[id] === false) return false;
  return defaultCollapsed;
}

export function togglePref(prefs = {}, id, defaultCollapsed = false) {
  return { ...prefs, [id]: !isCollapsed(prefs, id, defaultCollapsed) };
}
