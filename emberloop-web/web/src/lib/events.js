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
      return { name: String(name), args: typeof args === 'string' ? args : JSON.stringify(args), output: String(output ?? '') };
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
  };
}

export function cardForEvent(e) {
  const text = e.text ?? '';
  switch (e.kind) {
    case 'prompt':
      if (e.mode === 'steer') return { card: 'user', title: 'Steer', body: text, collapsed: false };
      if (e.mode === 'followup') return { card: 'user', title: 'Follow-up', body: text, collapsed: false };
      return { card: 'user', title: 'You', body: text, collapsed: false };
    case 'assistant_chunk':
      return { card: 'assistant', title: 'Agent', body: text, collapsed: false };
    case 'thinking_chunk':
      return { card: 'thinking', title: 'Thinking', body: text, collapsed: true };
    case 'tool_call_started':
    case 'tool_call_updated': {
      const t = parseToolText(text);
      const body = [t.args, t.output].filter(Boolean).join('\n');
      return { card: 'tool', title: t.name, body, collapsed: true };
    }
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
    blocks.push({ key: e.seq, card: c.card, title: c.title, body: c.body, collapsed: c.collapsed, open: e.kind === 'tool_call_started', events: [e] });
  }
  return blocks;
}
