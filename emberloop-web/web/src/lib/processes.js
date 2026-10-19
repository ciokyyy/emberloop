// Background processes of one Session: the shape helpers the Processes panel
// reads, kept free of the DOM so they test on their own.

// normalizeProcesses turns one watch payload (or list response) into rows. A
// malformed payload reads as no processes rather than throwing inside a stream
// handler.
export function normalizeProcesses(payload) {
  const list = payload?.processes;
  if (!Array.isArray(list)) return [];
  return list.filter((p) => p && typeof p.id === 'string').map((p) => ({
    id: p.id,
    command: oneLine(p.command),
    pid: Number.isFinite(p.pid) ? p.pid : 0,
    startedAt: p.startedAt || '',
    running: p.running !== false,
    exit: p.exit || '',
    output: typeof p.output === 'string' ? p.output : '',
  }));
}

// oneLine collapses a command to its first line, so a row never grows.
export function oneLine(command) {
  return String(command ?? '').split('\n')[0].trim();
}

// formatAge renders how long a process has run, from its start time: seconds
// under a minute, then minutes, then hours.
export function formatAge(startedAt, now = Date.now()) {
  const started = Date.parse(startedAt);
  if (!Number.isFinite(started)) return '';
  const secs = Math.max(0, Math.round((now - started) / 1000));
  if (secs < 60) return `${secs}s`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m`;
  return `${Math.floor(secs / 3600)}h`;
}

// tailOf keeps the last n lines of a process's output.
export function tailOf(output, n = 40) {
  const lines = String(output ?? '').replace(/\n+$/, '').split('\n');
  return lines.length <= n ? lines.join('\n') : lines.slice(-n).join('\n');
}

// runningCount is the badge number: processes still alive.
export function runningCount(processes) {
  return processes.filter((p) => p.running).length;
}

// processesSummary is the one-line meta of a Session's Processes row.
export function processesSummary(processes) {
  const total = processes.length;
  if (total === 0) return 'Nothing running';
  const running = runningCount(processes);
  if (running === 0) return `${total} finished`;
  return `${running} running${total > running ? `, ${total - running} finished` : ''}`;
}
