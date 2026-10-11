// Event-to-card mapping: the single place that turns Daemon Events into
// transcript cards. Every known kind maps to a card; unknown future kinds
// map to a generic card so the transcript never drops an Event.
import { parseSpawned, parseFinished } from './subagents.js';

export function parseToolText(text = '') {
  const raw = String(text ?? '');
  try {
    const obj = JSON.parse(raw);
    if (obj && typeof obj === 'object') {
      const name = obj.name ?? obj.tool ?? obj.function ?? 'tool';
      const args = obj.input ?? obj.args ?? obj.arguments ?? '';
      const output = obj.output ?? obj.result ?? '';
      return {
        name: String(name),
        args: typeof args === 'string' ? args : JSON.stringify(args),
        output: String(output ?? ''),
        id: obj.id ? String(obj.id) : '',
        server: obj.server ? String(obj.server) : '',
        status: obj.status ? String(obj.status) : '',
      };
    }
  } catch {
    // Not JSON: first line holds "name args", the rest is output.
  }
  const lines = raw.split('\n');
  const first = (lines[0] ?? '').trim();
  const space = first.indexOf(' ');
  return {
    name: space === -1 ? first || 'tool' : first.slice(0, space),
    args: space === -1 ? '' : first.slice(space + 1),
    output: lines.slice(1).join('\n').trim(),
    id: '',
    status: '',
  };
}

// splitMcpName turns "mcp__server__tool" into { server, name }: the UI shows
// the tool by its own name with the server as a quiet tag.
export function splitMcpName(raw = '') {
  const m = /^mcp__(.+?)__(.+)$/.exec(String(raw));
  return m ? { server: m[1], name: m[2] } : { server: '', name: String(raw) };
}

// parseReport recognizes the grouped Subagent result the Daemon sends its
// parent as a Steer. It is an agent report, not something the user typed.
export function parseReport(text = '') {
  const m = /^Subagents (\S+) finished:\n?([\s\S]*)$/.exec(String(text));
  if (!m) return null;
  const members = [...m[2].matchAll(/^- (\S+) \((\w+)\): ([\s\S]*?)(?=^- \S+ \(\w+\): |$(?![\s\S]))/gm)].map((x) => ({
    id: x[1],
    kind: x[2],
    reply: x[3].trim(),
  }));
  const n = members.length || 1;
  return { title: n === 1 ? 'Subagent report' : `${n} subagent reports`, body: m[2].trim(), members };
}

// parseSudoResult reads the TOON message the Daemon sends after a sudo
// request settled: "sudo_result:" then indented key: value lines, text values
// as quoted JSON strings. Anything else is not a sudo result.
export function parseSudoResult(text = '') {
  const t = String(text);
  if (!t.startsWith('sudo_result:')) return null;
  const out = { id: '', status: '', exit: 0, command: '', output: '' };
  for (const line of t.split('\n').slice(1)) {
    const m = /^\s+(\w+): ([\s\S]*)$/.exec(line);
    if (!m) continue;
    const [, key, raw] = m;
    let val = raw;
    if (raw.startsWith('"')) {
      try {
        val = JSON.parse(raw);
      } catch {
        val = raw;
      }
    }
    if (key === 'request') out.id = val;
    else if (key === 'exit') out.exit = Number(val);
    else if (key in out) out[key] = val;
  }
  return out;
}

// parseSudoRequest reads the JSON Text of sudo_request and sudo_resolved.
export function parseSudoRequest(text) {
  try {
    const r = JSON.parse(text);
    return r && typeof r === 'object' ? r : {};
  } catch {
    return {};
  }
}

// parseCompaction reads the JSON Text of a compaction Event; a malformed
// payload still renders as a compaction with no numbers.
export function parseCompaction(text) {
  try {
    const c = JSON.parse(text);
    return c && typeof c === 'object' ? c : {};
  } catch {
    return {};
  }
}

// withImageNote appends the attachment count to a user message body.
export function withImageNote(text, count) {
  if (!count) return text;
  const note = `_${count} image${count > 1 ? 's' : ''} attached_`;
  return text ? `${text}\n\n${note}` : note;
}

// STRIP_EXCLUDED names the tools that keep their own cards and never join
// a strip: spawn keeps the Group card, submit and sudo keep their own.
const STRIP_EXCLUDED = new Set(['spawn', 'submit', 'sudo']);

// stripRunStatus reduces one tool run's status to the strip's three states.
export function stripRunStatus(status = '') {
  const s = String(status ?? '').toLowerCase();
  if (s === 'failed') return 'failed';
  if (s === 'completed' || s === 'complete') return 'completed';
  return 'running';
}

export function cardForEvent(e) {
  const text = e.text ?? '';
  switch (e.kind) {
    case 'prompt': {
      if (e.handoff) return { card: 'report', title: 'Handoff', body: text, collapsed: true };
      const sr = parseSudoResult(text);
      if (sr) return { card: 'sudoresult', title: 'Sudo result', body: sr.output, collapsed: true, sudo: sr };
      const r = parseReport(text);
      if (r) return { card: 'report', title: r.title, body: r.body, collapsed: true, members: r.members };
      // imageIds are Session-stored attachments: the Card renders them as
      // thumbnails, replacing the literal note.
      const imageIds = e.imageIds ?? [];
      const body = imageIds.length ? text : withImageNote(text, e.images);
      if (e.mode === 'steer') return { card: 'user', title: 'Steer', body, collapsed: false, imageIds };
      if (e.mode === 'followup') return { card: 'user', title: 'Follow-up', body, collapsed: false, imageIds };
      return { card: 'user', title: 'You', body, collapsed: false, imageIds };
    }
    case 'assistant_chunk':
      return { card: 'assistant', title: 'Agent', body: text, collapsed: false };
    case 'thinking_chunk':
      return { card: 'thinking', title: 'Thinking', body: text, collapsed: true };
    case 'tool_call_started':
    case 'tool_call_updated':
    case 'tool_call_finished': {
      const t = parseToolText(text);
      const mcp = t.server ? { server: t.server, name: t.name } : splitMcpName(t.name);
      t.name = mcp.name;
      const body = [t.args, t.output].filter(Boolean).join('\n');
      return { card: 'tool', title: t.name, body, collapsed: true, server: mcp.server, toolId: t.id, status: t.status, input: t.args, output: t.output };
    }
    case 'compaction_started': {
      const c = parseCompaction(text);
      return { card: 'compaction', title: 'Compressing context', body: '', collapsed: true, compaction: { ...c, state: 'running' } };
    }
    case 'compaction': {
      const c = parseCompaction(text);
      return { card: 'compaction', title: 'Context compressed', body: c.summary ?? '', collapsed: true, compaction: { ...c, state: c.aborted || c.error ? 'failed' : 'done' } };
    }
    case 'compressed': {
      let c = {};
      try { c = JSON.parse(text); } catch { /* keep the divider */ }
      const bits = [`#${c.number ?? '?'}`, c.tokensBefore ? `${c.tokensBefore} tokens before` : '', c.model, c.fallback].filter(Boolean);
      return { card: 'divider', title: 'Context compressed', body: bits.join(' · '), collapsed: false };
    }
    case 'sudo_request': {
      const r = parseSudoRequest(text);
      return { card: 'sudo', title: 'Sudo request', body: r.command ?? '', collapsed: false, sudo: { ...r, state: r.state || 'pending' } };
    }
    case 'sudo_resolved':
      return { card: 'none', title: 'sudo', body: '', collapsed: false, sudo: parseSudoRequest(text) };
    case 'usage':
      return { card: 'none', title: 'usage', body: text, collapsed: false };
    case 'diff':
      return { card: 'diff', title: 'Diff', body: text, collapsed: false };
    case 'permission':
      return { card: 'permission', title: 'Permission', body: text || 'Auto-approved and logged.', collapsed: false };
    case 'status':
      // Idle and working are covered by the running indicator and the
      // sidebar dot; only terminal pills stay in the transcript.
      if (e.status === 'idle' || e.status === 'working') return { card: 'none', title: e.status, body: text, collapsed: false };
      return { card: 'status', title: e.status || 'status', body: text, collapsed: false };
    case 'subagent_spawned': {
      const p = parseSpawned(text);
      return { card: 'subagent', title: p.name || 'Subagent', body: text, collapsed: false, childId: p.id, groupId: p.group, name: p.name };
    }
    case 'subagent_finished': {
      const f = parseFinished(text);
      return { card: 'subagent', title: 'Subagent finished', body: text, collapsed: false, finished: true, groupId: f.group, childId: f.id };
    }
    case 'error':
      return { card: 'error', title: 'Error', body: text, collapsed: false };
    default:
      return { card: 'unknown', title: e.kind || 'event', body: text, collapsed: false };
  }
}

// memberOf resolves one Group member's display fields from the spawn
// payload plus the live Session list. The name never falls back to a bare
// id: spawn payload, Session meta, then the generic label.
function memberOf(spawned, sessionsById) {
  const meta = sessionsById[spawned.childId] ?? {};
  return {
    id: spawned.childId,
    name: spawned.name || meta.name || 'Subagent',
    harness: meta.harness ?? '',
    scout: Boolean(meta.scout),
    status: meta.status ?? '',
  };
}

// groupTranscript merges streamable runs into single render blocks:
// consecutive assistant chunks join, consecutive tool calls (except spawn,
// submit and sudo, which keep their own cards) collapse into one live strip
// block per burst that settles once the transcript moves on, and one spawn
// call's subagent_spawned Events merge into one Group header block that a
// later subagent_finished Event for the same Group settles. sessionsById is
// the live Session list keyed by id, used for member names and statuses.
// Everything else passes through.
export function groupTranscript(events, sessionsById = {}, opts = {}) {
  const blocks = [];
  for (const e of events) {
    const c = cardForEvent(e);
    // A resolved Event updates its request card in place.
    if (e.kind === 'sudo_resolved') {
      const open = blocks.findLast((b) => b.card === 'sudo' && b.sudo?.id === c.sudo?.id);
      if (open) {
        open.sudo = { ...open.sudo, ...c.sudo };
        open.events.push(e);
      }
      continue;
    }
    if (c.card === 'none') continue;
    // A spawn tool call is replaced by its Group card; only a failed call
    // keeps its own card so the error stays visible.
    if (c.card === 'tool' && String(c.title ?? '').toLowerCase() === 'spawn' && stripRunStatus(c.status || e.status) !== 'failed') continue;
    if (c.card === 'subagent' && !c.finished) {
      const last = blocks[blocks.length - 1];
      const member = memberOf(c, sessionsById);
      if (last?.card === 'group' && !last.finished && (c.groupId ? last.groupId === c.groupId : !last.groupId)) {
        last.members.push(member);
        last.events.push(e);
        last.title = `${last.members.length} subagent${last.members.length === 1 ? '' : 's'}`;
        continue;
      }
      blocks.push({ key: e.seq, card: 'group', title: '1 subagent', body: '', collapsed: false, groupId: c.groupId, members: [member], finished: false, events: [e] });
      continue;
    }
    if (c.card === 'subagent' && c.finished) {
      const hit = [...blocks].reverse().find((b) => b.card === 'group' && (c.groupId ? b.groupId === c.groupId : true));
      if (hit) {
        hit.finished = true;
        hit.events.push(e);
        continue;
      }
      blocks.push({ key: e.seq, card: c.card, title: c.title, body: c.body, collapsed: c.collapsed, finished: true, groupId: c.groupId, events: [e] });
      continue;
    }
    // Consecutive tool calls collapse into one strip card per burst. A new
    // start always opens a new run inside the open strip; an update joins
    // its run by tool id, or the open same-name run when the Backend sent
    // no id. Anything else starts a new burst after the transcript moved on.
    if (c.card === 'tool' && !STRIP_EXCLUDED.has(String(c.title ?? '').toLowerCase())) {
      const status = c.status || e.status || '';
      const run = { key: e.seq, name: c.title, server: c.server, input: c.input, output: c.output, status, toolId: c.toolId, events: [e] };
      const last = blocks[blocks.length - 1];
      if (last?.card === 'toolstrip') {
        if (run.toolId) {
          const hit = last.runs.find((r) => r.toolId && r.toolId === run.toolId);
          if (hit) {
            hit.title = run.name || hit.title;
            hit.server = run.server || hit.server;
            hit.input = run.input || hit.input;
            hit.output = run.output || hit.output;
            hit.status = run.status || hit.status;
            hit.events.push(e);
            last.events.push(e);
            continue;
          }
        }
        if (e.kind !== 'tool_call_started') {
          const open = last.runs[last.runs.length - 1];
          if (open && open.name === run.name && stripRunStatus(open.status) === 'running') {
            open.server = run.server || open.server;
            open.input = run.input || open.input;
            open.output = run.output || open.output;
            open.status = run.status || open.status;
            open.events.push(e);
            last.events.push(e);
            continue;
          }
        }
        last.runs.push(run);
        last.title = run.name || last.title;
        last.events.push(e);
        continue;
      }
      blocks.push({ key: e.seq, card: 'toolstrip', title: run.name, collapsed: false, runs: [run], settled: false, elapsedMs: 0, status: stripRunStatus(status), events: [e] });
      continue;
    }
    if (c.toolId) {
      const hit = blocks.findLast((b) => b.toolId === c.toolId);
      if (hit) {
        hit.title = c.title || hit.title;
        hit.server = c.server || hit.server;
        hit.input = c.input || hit.input;
        hit.output = c.output || hit.output;
        hit.status = c.status || hit.status;
        hit.body = [hit.input, hit.output].filter(Boolean).join('\n');
        hit.events.push(e);
        continue;
      }
    }
    const last = blocks[blocks.length - 1];
    // The end of a compaction replaces its running card in place.
    if (c.card === 'compaction' && e.kind === 'compaction') {
      const open = blocks.findLast((b) => b.card === 'compaction' && b.compaction?.state === 'running');
      if (open) {
        Object.assign(open, { title: c.title, body: c.body, compaction: c.compaction });
        open.events.push(e);
        continue;
      }
    }
    // Thinking arrives as paragraph chunks split by whitespace-only
    // separators: join consecutive ones, drop a leading blank.
    if (c.card === 'thinking') {
      // Thinking between tool calls joins the open strip as its own run.
      if (last?.card === 'toolstrip') {
        const lr = last.runs[last.runs.length - 1];
        if (lr?.thinking) {
          lr.output += c.body;
          lr.events.push(e);
          last.events.push(e);
          continue;
        }
        if (!c.body.trim()) continue;
        last.runs.push({ key: e.seq, name: 'thinking', thinking: true, input: '', output: c.body, status: 'completed', events: [e] });
        last.events.push(e);
        continue;
      }
      if (last?.card === 'thinking') {
        last.body += c.body;
        last.events.push(e);
        continue;
      }
      if (!c.body.trim()) continue;
    }
    if (c.card === 'assistant' && last?.card === 'assistant') {
      last.body += c.body;
      last.events.push(e);
      continue;
    }
    // An excluded tool run stays open from its started event until the next
    // tool event: updates merge into the open run, a new start opens a new
    // card even for the same tool name.
    if (c.card === 'tool' && last?.card === 'tool' && last.title === c.title && last.open && e.kind === 'tool_call_updated') {
      last.body = [last.body, c.body].filter(Boolean).join('\n');
      last.events.push(e);
      continue;
    }
    if (c.card === 'tool' && last?.card === 'tool') last.open = false;
    blocks.push({ key: e.seq, card: c.card, title: c.title, body: c.body, collapsed: c.collapsed, server: c.server, members: c.members, toolId: c.toolId, status: c.status, input: c.input, output: c.output, compaction: c.compaction, sudo: c.sudo, imageIds: c.imageIds, open: e.kind === 'tool_call_started', events: [e] });
  }
  for (let i = 0; i < blocks.length; i++) {
    const b = blocks[i];
    if (b.card !== 'toolstrip') continue;
    // A strip settles once the transcript moves on: only the last block
    // stays live. The settled strip renders its one-line summary.
    // With the Session no longer working (opts.working === false) nothing is
    // live: a cancelled turn never reports its tool end, so open runs close
    // as failed instead of spinning forever.
    const idle = opts.working === false;
    b.settled = idle || i < blocks.length - 1;
    if (idle) for (const r of b.runs) if (stripRunStatus(r.status) === 'running') r.status = 'failed';
    const times = [];
    for (const ev of b.events) {
      const t = Date.parse(ev.at);
      if (!Number.isNaN(t)) times.push(t);
    }
    b.elapsedMs = times.length > 1 ? Math.max(0, times[times.length - 1] - times[0]) : 0;
    b.status = stripRunStatus(b.runs[b.runs.length - 1]?.status);
  }
  return blocks;
}

// latestUsage returns { used, size } from the newest usage Event, or null.
export function latestUsage(events) {
  for (let i = events.length - 1; i >= 0; i--) {
    if (events[i].kind !== 'usage') continue;
    try {
      const u = JSON.parse(events[i].text);
      if (u && u.size > 0) return { used: Number(u.used) || 0, size: Number(u.size) };
    } catch {
      // Malformed usage Event: keep looking.
    }
  }
  return null;
}
