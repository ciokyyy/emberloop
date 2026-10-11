import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import MemoryPage from './MemoryPage.svelte';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubMemory({
  rules = [],
  observations = [],
  queue = [],
  ledger = [],
  status = null,
  profile = { version: 0, lines: [], rendered: '' },
  versions = [],
  profileQueue = [],
  seen = {},
  failStatus = false,
} = {}) {
  const defaultStatus = {
    available: true,
    queue: { pending: 1, approved: 0, rejected: 0, total: 1, budget: 50, overBudget: false, avgApprovalLatencyHours: 1 },
    ledgerSize: 1,
    lastLedger: { id: 1, runId: 'queue', stage: 'queue_approve', status: 'SUCCESS' },
    lastNightly: null,
  };
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      const method = opts?.method ?? 'GET';
      const body = opts?.body ? JSON.parse(opts.body) : null;
      seen[`${method} ${url}`] = body;
      if (url.includes('/api/memory/status')) {
        if (failStatus) return { ok: false, status: 503, json: async () => ({ available: false }) };
        return { ok: true, json: async () => status ?? defaultStatus };
      }
      if (url.includes('/api/rules/queue/batch') && method === 'POST') {
        seen.batch = body;
        return { ok: true, json: async () => [{ id: 1, status: 'approved', ruleId: 'r1' }] };
      }
      if (url.includes('/api/rules/queue/') && url.endsWith('/decide') && method === 'POST') {
        seen.decide = body;
        return { ok: true, json: async () => ({ ruleId: 'r1' }) };
      }
      if (url.includes('/api/rules/queue') && method === 'GET') return { ok: true, json: async () => queue };
      if (url.includes('/api/rules') && method === 'GET') {
        if (url.includes('q=')) {
          const q = decodeURIComponent(url.split('q=')[1].split('&')[0]);
          return { ok: true, json: async () => rules.filter((r) => JSON.stringify(r).includes(q)) };
        }
        return { ok: true, json: async () => rules };
      }
      if (url.includes('/api/observations') && method === 'GET') {
        if (url.includes('q=')) {
          const q = decodeURIComponent(url.split('q=')[1].split('&')[0]);
          return { ok: true, json: async () => observations.filter((o) => JSON.stringify(o).includes(q)) };
        }
        return { ok: true, json: async () => observations };
      }
      if (url.includes('/api/memory/ledger')) return { ok: true, json: async () => ledger };
      if (url.includes('/api/memory/purge') && method === 'POST') {
        seen.purge = body;
        return { ok: true, json: async () => ({ observations: 1, rules: 1, queue: 0, ledger: 0, scrubbed: 0, snapshots: 0 }) };
      }
      if (url.includes('/api/profile/versions')) return { ok: true, json: async () => versions };
      if (url.includes('/api/profile/restore') && method === 'POST') {
        seen.restore = body;
        return { ok: true, json: async () => ({ version: 2, lines: [], rendered: '' }) };
      }
      if (url.includes('/api/profile/queue/') && method === 'POST') {
        return { ok: true, json: async () => ({}) };
      }
      if (url.includes('/api/profile/queue')) return { ok: true, json: async () => profileQueue };
      if (url.includes('/api/profile') && method === 'PUT') return { ok: true, json: async () => ({}) };
      if (url.includes('/api/profile') && method === 'GET') return { ok: true, json: async () => profile };
      return { ok: true, json: async () => [] };
    }),
  );
  return seen;
}

const ruleA = { ruleId: 'r1', version: 1, text: 'use tabs for Go files', kind: 'preference', subjectKey: 'tabs', status: 'active' };
const ruleB = { ruleId: 'r2', version: 1, text: 'wrap errors always', kind: 'preference', subjectKey: 'errors', status: 'active' };
const obsA = { id: 11, sessionId: 's1', eventId: 'e1', quote: 'always wrap errors here', kind: 'preference', subjectKey: 'errors', evidenceCount: 1 };
const queueItem = {
  id: 7,
  kind: 'rule',
  status: 'pending',
  payload: JSON.stringify({ text: 'use tabs for Go files', quotes: ['uses tabs here'], sessions: ['s1'], reason: 'tabs reason', kind: 'preference', subject_key: 'tabs' }),
};

describe('MemoryPage list and search', () => {
  it('lists rules and observations', async () => {
    stubMemory({ rules: [ruleA, ruleB], observations: [obsA], queue: [] });
    const { container } = render(MemoryPage);
    expect(await within(container).findByTestId('memory-page')).toBeTruthy();
    expect(await within(container).findByText('use tabs for Go files')).toBeTruthy();
    expect(within(container).getByText('wrap errors always')).toBeTruthy();
    expect(within(container).getByText('always wrap errors here')).toBeTruthy();
  });

  it('searches rules through the query', async () => {
    stubMemory({ rules: [ruleA, ruleB], observations: [], queue: [] });
    const { container } = render(MemoryPage);
    await within(container).findByText('use tabs for Go files');
    const input = await within(container).findByLabelText('Search rules');
    await fireEvent.input(input, { target: { value: 'tabs' } });
    await tick();
    await fireEvent.submit(input.closest('form'));
    await waitFor(() => expect(within(container).queryByText('wrap errors always')).toBeNull());
    expect(within(container).getByText('use tabs for Go files')).toBeTruthy();
  });

  it('searches observations', async () => {
    stubMemory({ rules: [], observations: [obsA], queue: [] });
    const { container } = render(MemoryPage);
    await within(container).findByText('always wrap errors here');
    const input = await within(container).findByLabelText('Search observations');
    await fireEvent.input(input, { target: { value: 'wrap' } });
    await tick();
    await fireEvent.submit(input.closest('form'));
    expect(await within(container).findByText('always wrap errors here')).toBeTruthy();
  });
});

describe('MemoryPage queue', () => {
  it('shows quotes and sessions with approve, edit, reject and batch', async () => {
    const seen = stubMemory({ rules: [], observations: [], queue: [queueItem] });
    const { container } = render(MemoryPage);
    const row = await within(container).findByTestId('memory-queue-7');
    expect(within(row).getByText('use tabs for Go files')).toBeTruthy();
    expect(within(row).getByText(/uses tabs here/)).toBeTruthy();
    expect(within(row).getByText(/Sessions: s1/)).toBeTruthy();
    await fireEvent.click(within(row).getByText('Approve'));
    await within(container).findByText('Rule approved.');
    expect(seen.decide).toMatchObject({ status: 'approved' });
  });

  it('edits a proposal before approving', async () => {
    const seen = stubMemory({ rules: [], observations: [], queue: [queueItem] });
    const { container } = render(MemoryPage);
    const row = await within(container).findByTestId('memory-queue-7');
    await fireEvent.click(within(row).getByText('Edit'));
    const input = await within(row).findByLabelText('Rule text');
    await fireEvent.input(input, { target: { value: 'use tabs everywhere' } });
    await tick();
    await fireEvent.click(within(row).getByText('Save'));
    await within(container).findByText('Rule approved.');
    expect(seen.decide).toMatchObject({ status: 'approved', text: 'use tabs everywhere' });
  });

  it('batch approves a session', async () => {
    const seen = stubMemory({ rules: [], observations: [], queue: [queueItem] });
    const { container } = render(MemoryPage);
    await within(container).findByTestId('memory-queue-7');
    await fireEvent.click(await within(container).findByText(/Approve session/));
    await within(container).findByText(/Approved 1 proposal/);
    expect(seen.batch).toMatchObject({ sessionId: 's1', status: 'approved' });
  });
});

describe('MemoryPage versions', () => {
  it('lists versions and restores one as a new version', async () => {
    const seen = stubMemory({
      rules: [],
      observations: [],
      queue: [],
      versions: [{ version: 1, lines: [{ id: 'l1', text: 'use tabs', tag: 'workflow' }] }],
    });
    const { container } = render(MemoryPage);
    await fireEvent.click(await within(container).findByRole('button', { name: 'Restore version 1' }));
    await within(container).findByText(/Restored version 1/);
    expect(seen.restore).toEqual({ version: 1 });
  });
});

describe('MemoryPage purge', () => {
  it('needs the typed confirmation and reports what it removed', async () => {
    const seen = stubMemory({ rules: [ruleA], observations: [obsA], queue: [] });
    const { container } = render(MemoryPage);
    await within(container).findByText('use tabs for Go files');
    await fireEvent.click(await within(container).findByLabelText('Purge rule r1'));
    await fireEvent.click(await within(container).findByLabelText('Purge observation 11'));
    const confirm = await within(container).findByLabelText('Type PURGE to confirm');
    const btn = within(container).getByText('Purge selected');
    expect(btn.disabled).toBe(true);
    await fireEvent.input(confirm, { target: { value: 'PURGE' } });
    await tick();
    expect(btn.disabled).toBe(false);
    await fireEvent.click(btn);
    await within(container).findByText(/Removed 1 rule, 1 observation/);
    expect(seen.purge).toMatchObject({ confirm: 'PURGE' });
  });
});

describe('MemoryPage status', () => {
  it('shows the available chip with stats', async () => {
    stubMemory({ rules: [], observations: [], queue: [queueItem] });
    const { container } = render(MemoryPage);
    await waitFor(() => expect(within(container).getByTestId('memory-status').textContent).toMatch(/Available/));
    expect(await within(container).findByTestId('memory-stats')).toBeTruthy();
  });

  it('shows unavailable when the engine fails', async () => {
    stubMemory({ rules: [], observations: [], queue: [], failStatus: true });
    const { container } = render(MemoryPage);
    await waitFor(() => expect(within(container).getByTestId('memory-status').textContent).toMatch(/Unavailable/));
  });
});
