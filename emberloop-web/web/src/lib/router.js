// Browser router: real paths for sessions, runs, settings, quota, providers and memory.
// Pure parse/build functions live here so tests cover them without the DOM;
// App.svelte owns history.pushState and popstate around them.
//
// Paths: /s/<id> session (optional ?at=<seq> deep link), /runs,
// /runs/<runId>, /runs/<runId>/<ticketId>, /settings, /quota, /providers,
// /memory, / home.
export function parsePath(input) {
  const raw = String(input ?? '');
  const q = raw.indexOf('?');
  let path = q === -1 ? raw : raw.slice(0, q);
  const search = q === -1 ? '' : raw.slice(q + 1);
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
  if (segs[0] === 'runs' && segs.length === 1) return { name: 'runs' };
  if (segs[0] === 'runs' && segs.length === 2 && segs[1]) return { name: 'run', runId: segs[1] };
  if (segs[0] === 'runs' && segs.length === 3 && segs[1] && segs[2])
    return { name: 'ticket', runId: segs[1], ticketId: segs[2] };
  if (segs[0] === 'settings' && segs.length === 1) return { name: 'settings' };
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
    case 'session':
      return `/s/${encodeURIComponent(route.id)}${route.at ? `?at=${route.at}` : ''}`;
    case 'runs':
      return '/runs';
    case 'run':
      return `/runs/${encodeURIComponent(route.runId)}`;
    case 'ticket':
      return `/runs/${encodeURIComponent(route.runId)}/${encodeURIComponent(route.ticketId)}`;
    case 'settings':
      return '/settings';
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

// sameRoute reports whether a navigation changes anything visible: same
// session id and same at target means no reload.
export function sameRoute(a, b) {
  if (!a || !b || a.name !== b.name) return false;
  if (a.name === 'session') return a.id === b.id && (a.at ?? 0) === (b.at ?? 0);
  if (a.name === 'run') return a.runId === b.runId;
  if (a.name === 'ticket') return a.runId === b.runId && a.ticketId === b.ticketId;
  return true;
}
