// Sidebar helpers: the Session tree, row labels, chat filtering and the
// persisted collapse prefs. Pure except the localStorage helpers; the
// components stay thin and the tests pin behavior here. Owner groups and row
// status live in owners.js.

// Build the one-level Subagent tree: roots plus children by parent id.
// Orphans (a parent outside this list) stay roots.
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
