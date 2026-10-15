import { SETTINGS_PAGE_IDS } from './settings.js';
// Browser router: stable Project-id paths and canonical Session URLs.
// Pure parse/build functions live here so tests cover them without the DOM;
// App.svelte owns history.pushState and popstate around it.
//
// Paths: / is the All projects Dashboard; /p/<id> scopes Sessions;
// /p/<id>/runs[/<runId>[/<ticketId>]] and /p/<id>/settings stay scoped;
// /p/none is the home-directory scope. /s/<id> remains canonical.
export function parsePath(input) {
  const raw = String(input ?? '');
  const fragment = raw.indexOf('#');
  const address = fragment === -1 ? raw : raw.slice(0, fragment);
  const q = address.indexOf('?');
  let path = q === -1 ? address : address.slice(0, q);
  const search = q === -1 ? '' : address.slice(q + 1);
  if (!path.startsWith('/')) path = `/${path}`;
  if (path.length > 1) path = path.replace(/\/+$/, '');
  if (path === '') path = '/';
  const params = new URLSearchParams(search);
  const segs = path
    .split('/')
    .filter(Boolean)
    .map((s) => {
      try {
        return decodeURIComponent(s);
      } catch {
        return s;
      }
    });
  if (segs.length === 0) return { name: 'home' };
  if (segs[0] === 's' && segs.length === 2 && segs[1]) {
    const route = { name: 'session', id: segs[1] };
    const at = asPosInt(params.get('at'));
    if (at) route.at = at;
    return route;
  }
  if (segs[0] === 'p' && segs[1]) {
    if (segs.length === 2) return { name: 'project', projectId: segs[1] };
    if (segs[2] === 'runs') {
      if (segs.length === 3) return { name: 'project-runs', projectId: segs[1] };
      if (segs.length === 4 && segs[3]) return { name: 'project-run', projectId: segs[1], runId: segs[3] };
      if (segs.length === 5 && segs[3] && segs[4])
        return { name: 'project-ticket', projectId: segs[1], runId: segs[3], ticketId: segs[4] };
    }
    if (segs[2] === 'settings' && segs.length === 3) return { name: 'project-settings', projectId: segs[1] };
  }
  if (segs[0] === 'runs' && segs.length === 1) return { name: 'runs' };
  if (segs[0] === 'runs' && segs.length === 2 && segs[1]) return { name: 'run', runId: segs[1] };
  if (segs[0] === 'runs' && segs.length === 3 && segs[1] && segs[2])
    return { name: 'ticket', runId: segs[1], ticketId: segs[2] };
  if (segs[0] === 'settings' && segs.length === 1) return { name: 'settings' };
  if (segs[0] === 'settings' && segs.length === 2 && SETTINGS_PAGE_IDS.includes(segs[1]))
    return { name: 'settings', section: segs[1] };
  if (segs[0] === 'quota' && segs.length === 1) return { name: 'quota' };
  if (segs[0] === 'providers' && segs.length === 1) return { name: 'providers' };
  if (segs[0] === 'memory' && segs.length === 1) return { name: 'memory' };
  return { name: 'unknown', path };
}

function asPosInt(v) {
  if (v == null || v === '') return 0;
  const n = Number(v);
  return Number.isInteger(n) && n > 0 ? n : 0;
}

export function buildPath(route) {
  switch (route?.name) {
    case 'home':
      return '/';
    case 'project':
      return `/p/${encodeURIComponent(route.projectId)}`;
    case 'project-runs':
      return `/p/${encodeURIComponent(route.projectId)}/runs`;
    case 'project-settings':
      return `/p/${encodeURIComponent(route.projectId)}/settings`;
    case 'project-run':
      return `/p/${encodeURIComponent(route.projectId)}/runs/${encodeURIComponent(route.runId)}`;
    case 'project-ticket':
      return `/p/${encodeURIComponent(route.projectId)}/runs/${encodeURIComponent(route.runId)}/${encodeURIComponent(route.ticketId)}`;
    case 'session':
      return `/s/${encodeURIComponent(route.id)}${route.at ? `?at=${route.at}` : ''}`;
    case 'runs':
      return '/runs';
    case 'run':
      return `/runs/${encodeURIComponent(route.runId)}`;
    case 'ticket':
      return `/runs/${encodeURIComponent(route.runId)}/${encodeURIComponent(route.ticketId)}`;
    case 'settings':
      return route.section ? `/settings/${encodeURIComponent(route.section)}` : '/settings';
    case 'quota':
      return '/quota';
    case 'providers':
      return '/providers';
    case 'memory':
      return '/memory';
    default:
      return '/';
  }
}

// sameRoute compares route identity, including Session scroll targets and Project ids.
export function sameRoute(a, b) {
  if (!a || !b || a.name !== b.name) return false;
  if (a.name === 'session') return a.id === b.id && (a.at ?? 0) === (b.at ?? 0);
  if (a.name === 'run') return a.runId === b.runId;
  if (a.name === 'ticket') return a.runId === b.runId && a.ticketId === b.ticketId;
  if (a.name === 'settings') return a.section === b.section;
  if (a.name === 'project' || a.name === 'project-runs' || a.name === 'project-settings') return a.projectId === b.projectId;
  if (a.name === 'project-run') return a.projectId === b.projectId && a.runId === b.runId;
  if (a.name === 'project-ticket') return a.projectId === b.projectId && a.runId === b.runId && a.ticketId === b.ticketId;
  return true;
}
