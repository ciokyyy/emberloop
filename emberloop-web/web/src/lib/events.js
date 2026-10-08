// Event-to-card mapping: the single place that turns Daemon Events into
// transcript cards. Every known kind maps to a card; unknown future kinds
// map to a generic card so the transcript never drops an Event.

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

export function cardForEvent(e) {
  const text = e.text ?? '';
  switch (e.kind) {
    case 'prompt': {
      const r = parseReport(text);
      if (r) return { card: 'report', title: r.title, body: r.body, collapsed: true, members: r.members };
      if (e.mode === 'steer') return { card: 'user', title: 'Steer', body: text, collapsed: false };
      if (e.mode === 'followup') return { card: 'user', title: 'Follow-up', body: text, collapsed: false };
      return { card: 'user', title: 'You', body: text, collapsed: false };
    }
    case 'assistant_chunk':
      return { card: 'assistant', title: 'Agent', body: text, collapsed: false };
    case 'thinking_chunk':
      return { card: 'thinking', title: 'Thinking', body: text, collapsed: true };
    case 'tool_call_started':
    case 'tool_call_updated': {
      const t = parseToolText(text);
      const mcp = splitMcpName(t.name);
      t.name = mcp.name;
      const body = [t.args, t.output].filter(Boolean).join('\n');
      return { card: 'tool', title: t.name, body, collapsed: true, server: mcp.server, toolId: t.id, status: t.status, input: t.args, output: t.output };
    }
    case 'usage':
      return { card: 'none', title: 'usage', body: text, collapsed: false };
    case 'diff':
      return { card: 'diff', title: 'Diff', body: text, collapsed: false };
    case 'permission':
      return { card: 'permission', title: 'Permission', body: text || 'Auto-approved and logged.', collapsed: false };
    case 'status':
      return { card: 'status', title: e.status || 'status', body: text, collapsed: false };
    case 'subagent_spawned':
      return { card: 'subagent', title: 'Subagent started', body: text, collapsed: false };
    case 'subagent_finished':
      return { card: 'subagent', title: 'Subagent finished', body: text, collapsed: false };
    case 'error':
      return { card: 'error', title: 'Error', body: text, collapsed: false };
    default:
      return { card: 'unknown', title: e.kind || 'event', body: text, collapsed: false };
  }
}

// groupTranscript merges streamable runs into single render blocks:
// consecutive assistant chunks join, and a tool started event absorbs the
// following updates for the same tool name. Everything else passes through.
export function groupTranscript(events) {
  const blocks = [];
  for (const e of events) {
    const c = cardForEvent(e);
    if (c.card === 'none') continue;
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
    if (c.card === 'assistant' && last?.card === 'assistant') {
      last.body += c.body;
      last.events.push(e);
      continue;
    }
    // A tool run stays open from its started event until the next tool
    // event: updates merge into the open run, a new start opens a new
    // card even for the same tool name.
    if (c.card === 'tool' && last?.card === 'tool' && last.title === c.title && last.open && e.kind === 'tool_call_updated') {
      last.body = [last.body, c.body].filter(Boolean).join('\n');
      last.events.push(e);
      continue;
    }
    if (c.card === 'tool' && last?.card === 'tool') last.open = false;
    blocks.push({ key: e.seq, card: c.card, title: c.title, body: c.body, collapsed: c.collapsed, server: c.server, members: c.members, toolId: c.toolId, status: c.status, input: c.input, output: c.output, open: e.kind === 'tool_call_started', events: [e] });
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
