// Live tool strip helpers: the pure layer behind ToolStrip.svelte. Grouping
// lives in events.js (groupTranscript emits one toolstrip block per burst);
// this file only formats what the strip shows, so it stays free of Svelte.
export const STRIP_MAX_HEIGHT = 232;

// Tool names that keep their own cards and never join a strip: spawn keeps
// the Group card, submit and sudo keep their approval/result cards.
const EXCLUDED = new Set(['spawn', 'submit', 'sudo']);

export function isStripExcluded(name = '') {
  return EXCLUDED.has(String(name ?? '').trim().toLowerCase());
}

// formatElapsed renders milliseconds as seconds with one decimal.
export function formatElapsed(ms = 0) {
  return `${(Math.max(0, Number(ms) || 0) / 1000).toFixed(1)}s`;
}

// stripSummary is the settled one-liner: "3 tools · 4.3s".
export function stripSummary(count = 0, elapsedMs = 0) {
  return `${count} tool${count === 1 ? '' : 's'} · ${formatElapsed(elapsedMs)}`;
}

// meaningfulInput drops the placeholder a tool shows while the model still
// writes its arguments ("{}", "[]", "null"), so the strip shows its shimmer
// instead of an empty object.
export function meaningfulInput(input = '') {
  const t = String(input ?? '').trim();
  return /^(\{\s*\}|\[\s*\]|null|"")$/.test(t) ? '' : t === '' ? '' : String(input);
}

// toolTarget is the header's quiet meta text: the first file-like token of
// the input wins (a path, a command), else a short slice of the input.
export function toolTarget(run = {}) {
  const input = meaningfulInput(run.input).replace(/\s+/g, ' ').trim();
  if (!input) return '';
  const file = input.match(/[\w\-.~]+\/[\w\-.~/]*[\w\-.~]+|[\w\-~]+\.\w+/);
  if (file) return file[0].slice(0, 80);
  return input.slice(0, 80);
}

// outputKind picks the per-tool renderer: read/grep file lines, edit/write
// red/green diff lines, bash terminal stream, everything else generic JSON.
export function outputKind(name = '') {
  const n = String(name ?? '').toLowerCase();
  if (/(^|[^a-z])(read|cat|grep|glob|ls|find|search)([^a-z]|$)/.test(n)) return 'file';
  if (n.includes('edit') || n.includes('write') || n.includes('apply') || n.includes('create') || n.includes('patch')) return 'diff';
  if (n.includes('bash') || n.includes('exec') || n.includes('shell') || n.includes('terminal') || n === 'sh') return 'stream';
  return 'json';
}

// toolIcon names the header glyph per tool family; ToolStrip.svelte maps
// these to @lucide/svelte components.
export function toolIcon(name = '') {
  const n = String(name ?? '').toLowerCase();
  if (n === 'thinking') return 'thinking';
  if (n.includes('grep') || n.includes('glob') || n.includes('search')) return 'grep';
  if (n.includes('edit') || n.includes('patch')) return 'edit';
  if (n.includes('write') || n.includes('create')) return 'write';
  if (n.includes('bash') || n.includes('exec') || n.includes('shell') || n.includes('terminal') || n === 'sh') return 'bash';
  if (n.includes('read') || n.includes('cat') || n.includes('ls') || n.includes('find')) return 'read';
  return 'tool';
}

// splitOutputLines turns one output snapshot into render lines. A single
// trailing newline is not a line; interior blanks stream as they arrive.
export function splitOutputLines(output = '') {
  const lines = String(output ?? '').split('\n');
  if (lines.length > 1 && lines[lines.length - 1] === '') lines.pop();
  return lines;
}

// prefersReducedMotion gates the slide/blur swap: reduced motion renders
// instant height with no moving parts.
export function prefersReducedMotion() {
  try {
    return typeof matchMedia !== 'undefined' && matchMedia('(prefers-reduced-motion: reduce)').matches;
  } catch {
    return false;
  }
}
// Per-line highlight memo: streaming re-renders every output line per delta,
// but only the tail lines are new. The same tag+line returns the same segment
// array, so Svelte keeps the existing spans instead of replacing them. The
// tag names the renderer explicitly (minification-safe); the cache is bounded
// and shared across strips.
const segCache = new Map();
export function memoSegs(tag, line, compute) {
  const key = `${tag}\n${line}`;
  const hit = segCache.get(key);
  if (hit) return hit;
  const segs = compute(line);
  if (segCache.size > 4000) segCache.clear();
  segCache.set(key, segs);
  return segs;
}
