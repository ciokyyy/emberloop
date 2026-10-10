import { parseToolText, splitMcpName } from './events.js';
// Subagent display helpers: the pure layer behind the named Subagent cards
// and the Group header. GroupCard.svelte renders from these; the tests pin
// them here so card markup stays thin.

// fallbackName mirrors the Daemon's agentDisplayName: an explicit spawn
// name wins, otherwise the first four words of the prompt. It never
// returns a bare id.
export function fallbackName(prompt = '') {
  const words = String(prompt ?? '').split(/\s+/).filter(Boolean).slice(0, 4);
  return words.length ? words.join(' ') : 'Subagent';
}

export function displayName({ name = '', prompt = '' } = {}) {
  const n = String(name ?? '').trim();
  return n || fallbackName(prompt);
}

// parseSpawned reads a subagent_spawned Event text: the current JSON
// {id, group, name} payload or a legacy bare child id.
export function parseSpawned(text = '') {
  const raw = String(text ?? '');
  try {
    const obj = JSON.parse(raw);
    if (obj && typeof obj === 'object' && obj.id) {
      return { id: String(obj.id), group: obj.group ? String(obj.group) : '', name: obj.name ? String(obj.name) : '' };
    }
  } catch {
    // Legacy bare id.
  }
  return { id: raw.trim(), group: '', name: '' };
}

// parseFinished reads a subagent_finished Event text: the group id, or the
// per-child JSON payload each-mode delivery writes.
export function parseFinished(text = '') {
  const raw = String(text ?? '');
  try {
    const obj = JSON.parse(raw);
    if (obj && typeof obj === 'object' && (obj.group || obj.id)) {
      return { group: obj.group ? String(obj.group) : '', id: obj.id ? String(obj.id) : '' };
    }
  } catch {
    // Group-mode text is the bare group id.
  }
  return { group: raw.trim(), id: '' };
}

// latestTool summarizes the newest tool call in child Events as one line
// ("Edit stt.js"): the tool name plus the first file-like argument token.
export function latestTool(events = []) {
  for (let i = events.length - 1; i >= 0; i--) {
    const e = events[i];
    if (e.kind !== 'tool_call_started') continue;
    const t = parseToolText(e.text);
    const mcp = t.server ? { name: t.name } : splitMcpName(t.name);
    const args = String(t.args ?? '');
    const file = (args.match(/[\w\-.~]+\/[\w\-.~/]*[\w\-.~]+|[\w\-~]+\.\w+/) ?? [])[0] ?? '';
    const hint = file || args.replace(/\s+/g, ' ').trim().slice(0, 28);
    return [mcp.name || 'tool', hint].filter(Boolean).join(' ');
  }
  return '';
}

// childState reduces a child Session to its card state. An idle child
// with no reply yet is still running: it spawned but its turn has not
// answered.
export function childState(status = 'idle', events = []) {
  if (status === 'failed') return 'failed';
  if (status === 'working') return 'running';
  if (status === 'ended') return 'done';
  const replied = events.some((e) => e.kind === 'assistant_chunk' || e.kind === 'tool_call_started');
  return replied ? 'done' : 'running';
}

// groupCounts tallies one Group header line from member states.
export function groupCounts(states = []) {
  const done = states.filter((s) => s === 'done').length;
  const failed = states.filter((s) => s === 'failed').length;
  const running = states.length - done - failed;
  return { total: states.length, done, failed, running, allDone: states.length > 0 && done + failed === states.length, anyFailed: failed > 0 };
}
