import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import TicketPage from './TicketPage.svelte';

class FakeSource {
  static instances = [];
  constructor(url) {
    this.url = String(url);
    this.onmessage = null;
    this.closed = false;
    FakeSource.instances.push(this);
  }
  close() {
    this.closed = true;
  }
  emit(data) {
    this.onmessage?.({ data: JSON.stringify(data) });
  }
}

const reviewVerdicts = {
  'reviewer-spec': { verdict: 'pass', at: '4444333fff02', fresh: false },
  'reviewer-standards': { verdict: 'changes', at: 'bad73e8aaa01', fresh: false },
};

const ticket = (over = {}) => ({
  id: '07-ui-design',
  num: 7,
  title: 'UI design',
  blockers: [],
  state: 'in-review',
  stateSince: new Date(Date.now() - 11 * 60 * 1000).toISOString(),
  workerSession: 'sess-worker1',
  reviewerSessions: { 'reviewer-spec': 'sess-spec1', 'reviewer-standards': 'sess-std1' },
  verdicts: { ...reviewVerdicts },
  gate: { required: ['reviewer-standards', 'reviewer-spec'], verdicts: { ...reviewVerdicts } },
  tip: '4444333fff02',
  branch: 'implement/07-ui-design',
  body: '## Brief\nImplement ticket 07.',
  boxes: [
    { text: 'Styled transcript', checked: true },
    { text: 'Docs updated', checked: false },
  ],
  lastRefusal: '',
  needsYou: '',
  followups: 'Registry port declined.',
  agents: [
    { sessionId: 'sess-worker1', role: 'worker', harness: 'claude', status: 'working', name: '07-ui-design' },
    { sessionId: 'sess-std1', role: 'reviewer-standards', harness: 'claude', status: 'working', name: 'standards' },
    { sessionId: 'sess-spec1', role: 'reviewer-spec', harness: 'claude', status: 'idle', name: 'spec' },
    { sessionId: 'sess-scout1', role: 'scout', harness: 'pig', status: 'idle', name: 'find-events-usage' },
  ],
  rounds: [
    {
      n: 1,
      tip: 'bad73e8aaa01',
      startedAt: '2026-10-09T10:00:00Z',
      endedAt: '2026-10-09T10:02:00Z',
      reviews: {
        'reviewer-standards': {
          verdict: 'changes',
          findings: [{ file: 'events.js', line: 78, severity: 'major', problem: 'tool blocks all open' }],
          at: 'bad73e8aaa01',
          durationMs: 68000,
        },
        'reviewer-spec': { verdict: 'pass', findings: [], at: 'bad73e8aaa01', durationMs: 65000 },
      },
      declined: ['Full shadcn registry port: same tokens.'],
      sentFindings: [
        {
          findingId: 'r1-reviewer-standards-1',
          reviewer: 'reviewer-standards',
          file: 'events.js',
          line: 78,
          severity: 'major',
          problem: 'tool blocks all open',
          blocking: true,
          disposition: { findingId: 'r1-reviewer-standards-1', disposition: 'fixed' },
        },
        {
          findingId: 'r1-reviewer-spec-1',
          reviewer: 'reviewer-spec',
          file: 'RunsView.svelte',
          line: 12,
          severity: 'minor',
          problem: 'align the button',
          blocking: false,
          disposition: { findingId: 'r1-reviewer-spec-1', disposition: 'declined', reason: 'same tokens' },
        },
      ],
      sentToWorkerAt: '2026-10-09T10:03:00Z',
    },
    {
      n: 2,
      tip: '4444333fff02',
      startedAt: '2026-10-09T10:05:00Z',
      reviews: {
        'reviewer-spec': { verdict: 'pass', findings: [], heldFromRound: 1, at: 'bad73e8aaa01' },
      },
    },
  ],
  ...over,
});

const runWith = (t) => ({
  id: 'run-1',
  spec: '/repo/.scratch/demo/spec.md',
  repo: '/repo',
  status: 'running',
  tickets: [t],
});

function stubApi(current) {
  FakeSource.instances = [];
  vi.stubGlobal('EventSource', FakeSource);
  const fetchMock = vi.fn(async (u) => {
    const s = String(u);
    if (s.includes('/api/sessions/')) {
      return {
        ok: true,
        json: async () => ({ session: { id: 'x', status: 'working' }, events: [], hasMore: false, firstSeq: 0 }),
      };
    }
    if (s.includes('/api/runs/')) return { ok: true, json: async () => ({ run: current() }) };
    throw new Error(`unexpected fetch ${s}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const runStream = () => FakeSource.instances.find((i) => i.url.includes('/api/runs/'));
const runFetches = (fetchMock) => fetchMock.mock.calls.filter(([u]) => String(u).includes('/api/runs/') && !String(u).includes('/events'));

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('TicketPage', () => {
  it('renders the state track, rounds, panel and worker composer', async () => {
    let current = runWith(ticket());
    stubApi(() => current);
    const onOpenSession = vi.fn();
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {}, onOpenSession },
    });
    const page = await within(container).findByTestId('ticket-page');
    await within(container).findByText(/UI design/);
    expect(within(page).getByTestId('ticket-title').textContent).toMatch(/UI design/);

    // Two cards for round 2: standards re-judging, spec pass held.
    const dividers = within(page).getAllByTestId('round-divider');
    expect(dividers[1].textContent).toMatch(/review round 2/);
    expect(within(page).getByText('pass held')).toBeTruthy();
    expect(within(page).getByText('re-judging')).toBeTruthy();
    // The finding-response card identifies blocking state and the stored
    // worker dispositions, including a declined reason for a minor.
    const sentFindings = within(page).getByTestId('sent-findings-card');
    expect(within(sentFindings).getByText('Review findings sent to worker')).toBeTruthy();
    expect(within(sentFindings).getByText('blocking')).toBeTruthy();
    expect(within(sentFindings).getByText('non-blocking')).toBeTruthy();
    expect(within(sentFindings).getByText('r1-reviewer-standards-1')).toBeTruthy();
    const dispositions = within(sentFindings).getAllByTestId('finding-disposition');
    expect(dispositions[0].textContent).toContain('fixed');
    expect(dispositions[1].textContent).toContain('declined');
    expect(dispositions[1].textContent).toContain('same tokens');

    const panel = within(page).getByTestId('ticket-panel');
    expect(within(panel).getByText('07-ui-design')).toBeTruthy();
    expect(within(panel).getByText('find-events-usage')).toBeTruthy();
    const acceptance = within(panel).getByTestId('acceptance');
    expect(within(acceptance).getByText(/Acceptance · 1 of 2/)).toBeTruthy();
    const gate = within(panel).getByTestId('gate');
    expect(within(gate).getByText('standards')).toBeTruthy();
    expect(within(gate).getByText('changes')).toBeTruthy();
    expect(within(panel).getByText(/Registry port declined/)).toBeTruthy();

    // The worker keeps its composer; the worker transcript reuses Transcript.
    expect(within(page).getByTestId('worker-transcript')).toBeTruthy();
    expect(within(page).getByTestId('worker-composer')).toBeTruthy();

    // The worker agent opens in the Session view; reviewers open read-only.
    await fireEvent.click(within(panel).getByText('07-ui-design'));
    expect(onOpenSession).toHaveBeenCalledWith('sess-worker1');
  });

  it('opens reviewers and scouts read-only in a drawer with no composer', async () => {
    let current = runWith(ticket());
    stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    const page = await within(container).findByTestId('ticket-page');
    await within(container).findByText(/UI design/);
    const agents = within(page).getByTestId('agents');
    await fireEvent.click(within(agents).getByText('standards'));
    const drawer = await within(container).findByTestId('agent-drawer');
    expect(within(drawer).getByText(/Read-only transcript/)).toBeTruthy();
    expect(within(drawer).queryByLabelText('message')).toBeNull();
    await fireEvent.click(within(drawer).getByRole('button', { name: 'Close transcript' }));
    await tick();
    expect(within(container).queryByTestId('agent-drawer')).toBeNull();
  });

  it('ticks acceptance live from the Run SSE without refetching', async () => {
    let current = runWith(ticket());
    const fetchMock = stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    const page = await within(container).findByTestId('ticket-page');
    await within(page).findByText(/Acceptance · 1 of 2/);
    expect(runStream()).toBeTruthy();

    current = runWith(ticket({ boxes: [{ text: 'Styled transcript', checked: true }, { text: 'Docs updated', checked: true }] }));
    runStream().emit({ run: current });
    await within(page).findByText(/Acceptance · 2 of 2/);
    // The SSE snapshot updates the page; no polling refetch happens.
    expect(runFetches(fetchMock)).toHaveLength(1);
  });

  it('shows the needs-you banner and alert on the track', async () => {
    let current = runWith(ticket({ state: 'needs-you', needsYou: 'same refusal ×3. Steer or park.' }));
    stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    const page = await within(container).findByTestId('ticket-page');
    expect(await within(page).findByTestId('needs-you')).toBeTruthy();
    expect(within(await within(page).findByTestId('state-track')).getByText('needs you')).toBeTruthy();
  });

  it('closes every stream on unmount', async () => {
    let current = runWith(ticket());
    stubApi(() => current);
    const { container, unmount } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    await within(container).findByTestId('ticket-page');
    await within(container).findByText(/UI design/);
    await tick();
    await tick();
    expect(FakeSource.instances.length).toBeGreaterThan(0);
    unmount();
    await tick();
    expect(FakeSource.instances.every((s) => s.closed)).toBe(true);
  });

  it('never opens a worker stream for a getSession resolving after unmount', async () => {
    let current = runWith(ticket());
    let resolveSession = null;
    const gated = new Promise((res) => (resolveSession = res));
    FakeSource.instances = [];
    vi.stubGlobal('EventSource', FakeSource);
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        const s = String(u);
        if (s.includes('/api/sessions/')) return gated;
        if (s.includes('/api/runs/')) return { ok: true, json: async () => ({ run: current }) };
        throw new Error(`unexpected fetch ${s}`);
      }),
    );
    const { container, unmount } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    await within(container).findByTestId('ticket-page');
    // The Run snapshot loads (ticket renders) while the worker fetch stays
    // gated; the Run stream opens from the snapshot.
    await within(container).findByText(/UI design/);
    expect(runStream()).toBeTruthy();
    const before = FakeSource.instances.length;
    unmount();
    resolveSession({ ok: true, json: async () => ({ session: { id: 'x' }, events: [], hasMore: false, firstSeq: 0 }) });
    await gated;
    await tick();
    await new Promise((r) => setTimeout(r, 10));
    const sessions = FakeSource.instances.filter((i) => i.url.includes('/api/sessions/'));
    expect(sessions).toHaveLength(0);
    expect(FakeSource.instances.length).toBe(before);
  });

  it('closes the Run stream when the run is finished', async () => {
    let current = { ...runWith(ticket({ state: 'merged' })), status: 'done' };
    stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    await within(container).findByTestId('ticket-page');
    await within(container).findByText(/UI design/);
    await tick();
    await tick();
    // Finished: the loader never subscribes, so no Run stream exists.
    expect(runStream()).toBeFalsy();
  });

  it('shows a reconnecting indicator when the Run stream errors', async () => {
    let current = runWith(ticket());
    stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    await within(container).findByTestId('ticket-page');
    await within(container).findByText(/UI design/);
    expect(runStream()).toBeTruthy();
    runStream().onerror?.();
    await within(container).findByTestId('stream-reconnecting');
  });

  it('expands a long brief and renders duplicate boxes', async () => {
    const body = ['# T', 'l1', 'l2', 'l3', 'l4', 'l5', 'l6', 'l7'].join('\n');
    let current = runWith(
      ticket({ body, boxes: [{ text: 'Same box', checked: true }, { text: 'Same box', checked: false }] }),
    );
    stubApi(() => current);
    const { container } = render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });
    const page = await within(container).findByTestId('ticket-page');
    const brief = await within(page).findByTestId('ticket-brief');
    expect(within(brief).getByText(/Show 2 more lines/)).toBeTruthy();
    await fireEvent.click(within(brief).getByText(/Show 2 more lines/));
    expect(within(brief).getByText(/Show less/)).toBeTruthy();
    // Duplicate checkbox texts render twice instead of colliding on the key.
    expect(within(page).getAllByText('Same box')).toHaveLength(2);
  });

  // Per-ticket actions post to their Run ticket route and apply the
  // answered snapshot at once, ahead of the SSE echo.
  function stubActions(current, onAction) {
    FakeSource.instances = [];
    vi.stubGlobal('EventSource', FakeSource);
    const calls = [];
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => {
        const s = String(u);
        if (s.includes('/api/sessions/')) {
          return { ok: true, json: async () => ({ session: { id: 'x', status: 'idle' }, events: [], hasMore: false, firstSeq: 0 }) };
        }
        if (opts?.method === 'POST' && /\/tickets\/.+\/(steer|retry|park|discard)$/.test(s)) {
          calls.push({ url: s, body: opts.body ? JSON.parse(opts.body) : {} });
          return { ok: true, json: async () => ({ run: onAction(s, current()) }) };
        }
        if (s.includes('/api/runs/')) return { ok: true, json: async () => ({ run: current() }) };
        throw new Error(`unexpected fetch ${s}`);
      }),
    );
    return calls;
  }

  const renderTicket = () =>
    render(TicketPage, {
      props: { runId: 'run-1', ticketId: '07-ui-design', onNavigate: () => {} },
    });

  it('retries a needs-you ticket through its route and clears the banner', async () => {
    let current = runWith(ticket({ state: 'needs-you', needsYou: 'same refusal ×3' }));
    const calls = stubActions(() => current, (url) => {
      expect(url).toMatch(/\/tickets\/07-ui-design\/retry$/);
      current = runWith(ticket({ state: 'running', needsYou: '', lastRefusal: 'stale gate refusal' }));
      return current;
    });
    const { container } = renderTicket();
    const page = await within(container).findByTestId('ticket-page');
    expect(await within(page).findByTestId('needs-you')).toBeTruthy();
    await fireEvent.click(within(page).getByRole('button', { name: /^retry$/i }));
    expect(calls).toHaveLength(1);
    // The answered snapshot applies at once: the needs-you banner gives
    // way to the leftover refusal without waiting for the SSE echo.
    expect(await within(page).findByTestId('last-refusal')).toBeTruthy();
    expect(within(page).queryByTestId('needs-you')).toBeNull();
  });

  it('parks a ticket and unparks it back through retry', async () => {
    let current = runWith(ticket());
    const calls = stubActions(() => current, (url) => {
      if (url.endsWith('/park')) current = runWith(ticket({ state: 'parked' }));
      if (url.endsWith('/retry')) current = runWith(ticket({ state: 'running' }));
      return current;
    });
    const { container } = renderTicket();
    const page = await within(container).findByTestId('ticket-page');
    await within(page).findByTestId('ticket-actions');
    await fireEvent.click(within(page).getByRole('button', { name: /^park$/i }));
    expect(calls.at(-1).url).toMatch(/\/tickets\/07-ui-design\/park$/);
    expect((await within(page).findAllByText('parked')).length).toBeGreaterThan(0);
    await fireEvent.click(within(page).getByRole('button', { name: /^unpark$/i }));
    expect(calls.at(-1).url).toMatch(/\/tickets\/07-ui-design\/retry$/);
    // The answered snapshot swaps Unpark back to Park.
    expect(await within(page).findByRole('button', { name: /^park$/i })).toBeTruthy();
    expect(within(page).queryByRole('button', { name: /^unpark$/i })).toBeNull();
  });

  it('discards only after a second confirming click', async () => {
    let current = runWith(ticket());
    const calls = stubActions(() => current, () => {
      current = runWith(ticket({ state: 'discarded' }));
      return current;
    });
    const { container } = renderTicket();
    const page = await within(container).findByTestId('ticket-page');
    await within(page).findByTestId('ticket-actions');
    await fireEvent.click(within(page).getByRole('button', { name: /^discard$/i }));
    expect(calls).toHaveLength(0);
    await fireEvent.click(within(page).getByRole('button', { name: /confirm discard/i }));
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toMatch(/\/tickets\/07-ui-design\/discard$/);
    // A discarded ticket loses its actions.
    await tick();
    await tick();
    expect(within(page).queryByTestId('ticket-actions')).toBeNull();
  });

  it('steers the worker through its route from the inline form', async () => {
    let current = runWith(ticket());
    const calls = stubActions(() => current, () => current);
    const { container } = renderTicket();
    const page = await within(container).findByTestId('ticket-page');
    await within(page).findByTestId('ticket-actions');
    await fireEvent.click(within(page).getByRole('button', { name: /^steer$/i }));
    const input = await within(page).findByLabelText(/steer the worker/i);
    await fireEvent.input(input, { target: { value: 'try the registry port' } });
    await fireEvent.click(within(page).getByRole('button', { name: /send steer/i }));
    expect(calls).toHaveLength(1);
    expect(calls[0].url).toMatch(/\/tickets\/07-ui-design\/steer$/);
    expect(calls[0].body).toEqual({ text: 'try the registry port' });
  });

  it('hides the actions on merged and discarded tickets', async () => {
    let current = runWith(ticket({ state: 'merged', workerSession: 'sess-worker1' }));
    stubApi(() => current);
    const { container } = renderTicket();
    const page = await within(container).findByTestId('ticket-page');
    await within(page).findByText(/UI design/);
    await tick();
    expect(within(page).queryByTestId('ticket-actions')).toBeNull();
  });
});
