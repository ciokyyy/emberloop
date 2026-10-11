// Owner groups for the Session list: Runs and Sessions per User, then per
// Project, with Subagents and Run Workers nested under their parent row. Pure;
// the components render what these helpers return and the tests pin the rules.
import { buildTree, matchSession } from './sidebar.js';

// Owner palette slots: --owner-1 .. --owner-6 in app.css.
export const OWNER_TONES = 6;
export const FILTERS = ['mine', 'all', 'project'];

// CSS color of a User: its palette slot by position in the account list, so a
// User keeps its color as accounts are added. Unknown Users get the muted color.
export function ownerTone(users = [], id = '') {
  const i = users.findIndex((u) => u.id === id);
  return i < 0 ? 'var(--muted-foreground)' : `var(--owner-${(i % OWNER_TONES) + 1})`;
}

const trimSlash = (p) => String(p ?? '').replace(/\/+$/, '');

// Id of the Project whose folder is path, or '' for Other.
export function projectIdForPath(projects = [], path = '') {
  const want = trimSlash(path);
  if (!want) return '';
  return projects.find((p) => trimSlash(p.path) === want)?.id ?? '';
}

// Row for a Run. A Run has no Session: its Worker and Reviewer Sessions name it
// as parentId, so they nest under this row.
export function runRow(run = {}, projects = []) {
  return {
    id: run.id,
    kind: 'run',
    title: run.title || run.spec || run.id,
    harness: run.harness ?? '',
    status: run.status ?? '',
    needsYou: (run.tickets ?? []).some((t) => !!t.needsYou),
    owner: run.owner ?? '',
    projectId: projectIdForPath(projects, run.repo),
  };
}

const RUN_DOT = { running: 'working', paused: 'idle', failed: 'failed', done: 'ended' };

// Status dot of one row: capped, needs, working, failed, ended or idle.
export function dotState(row = {}) {
  if (row.capped) return 'capped';
  if (row.needsYou || row.status === 'needs-you') return 'needs';
  if (row.kind === 'run') return RUN_DOT[row.status] ?? 'idle';
  if (row.status === 'working' || row.status === 'failed' || row.status === 'ended') return row.status;
  return 'idle';
}
// Chat filter hit: a root row or one of its Subagents matches.
function rootMatches(root, kids, projectName, q) {
  return [root, ...(kids.get(root.id) ?? [])].some((s) => matchSession(s, projectName, q));
}

// buildOwnerGroups returns { groups, kids }. One group per User: own group
// first, then others by name. One section per Project: project order, Other
// last. Each section holds tree roots (Runs and root Sessions); kids maps a
// root id to its nested rows.
// filter: 'mine' keeps the own group; 'project' keeps rows of projectId;
// 'all' keeps every group. query: chat filter text, drops empty groups and
// sections. The own group lists every Project; other groups list only Projects
// with rows. A group's count is its root rows after owner and Project filters.
export function buildOwnerGroups({
  users = [],
  me = null,
  sessions = [],
  runs = [],
  projects = [],
  filter = 'mine',
  projectId = '',
  query = '',
} = {}) {
  // The signed-in User is online by definition, even before the list loads.
  const everyone = me && !users.some((u) => u.id === me.id)
    ? [...users, { id: me.id, username: me.username, admin: !!me.admin, online: true }]
    : users;
  const q = String(query ?? '').trim().toLowerCase();
  const rows = [...runs.map((r) => runRow(r, projects)), ...sessions].filter((r) => r.owner);
  const { roots, kids } = buildTree(rows);
  const projectById = new Map(projects.map((p) => [p.id, p]));
  const groups = new Map();
  const groupFor = (id) => {
    if (!groups.has(id)) {
      const u = everyone.find((x) => x.id === id) ?? { id, username: id.slice(0, 8), online: false };
      groups.set(id, {
        id,
        name: u.username,
        online: !!u.online,
        own: !!me && id === me.id,
        tone: ownerTone(everyone, id),
        count: 0,
        sections: new Map(),
      });
    }
    return groups.get(id);
  };
  for (const u of everyone) groupFor(u.id);

  for (const root of roots) {
    const key = projectById.has(root.projectId) ? root.projectId : '';
    if (filter === 'project' && key !== projectId) continue;
    const g = groupFor(root.owner);
    g.count += 1;
    const proj = projectById.get(key);
    if (q && !rootMatches(root, kids, proj?.name ?? '', q)) continue;
    if (!g.sections.has(key)) {
      g.sections.set(key, { key, name: proj?.name ?? 'Other', path: proj?.path ?? '', roots: [] });
    }
    g.sections.get(key).roots.push(root);
  }

  // The own group offers every Project, so a chat can start in an empty one.
  for (const g of groups.values()) {
    if (!g.own || q) continue;
    for (const p of projects) {
      if (filter === 'project' && p.id !== projectId) continue;
      if (!g.sections.has(p.id)) g.sections.set(p.id, { key: p.id, name: p.name, path: p.path, roots: [] });
    }
  }

  const order = [...projects.map((p) => p.id), ''];
  let list = [...groups.values()].map((g) => ({
    ...g,
    sections: order.filter((k) => g.sections.has(k)).map((k) => g.sections.get(k)),
  }));
  if (filter === 'mine') list = list.filter((g) => g.own);
  if (q) list = list.filter((g) => g.sections.length);
  list.sort((a, b) => Number(b.own) - Number(a.own) || a.name.localeCompare(b.name));
  return { groups: list, kids };
}
