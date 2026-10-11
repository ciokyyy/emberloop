import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import RunsView from './RunsView.svelte';
import { discovery, resetDiscovery } from './runsDiscovery.svelte.js';

const preview = {
  spec: '/repo/.scratch/demo/spec.md',
  title: 'Demo spec',
  tickets: [
    { id: '01-first', num: 1, title: 'First', blockers: [], done: 1, total: 2 },
    { id: '02-second', num: 2, title: 'Second', blockers: ['01-first'], done: 0, total: 1 },
  ],
};

const specRow = { spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 };

beforeEach(() => {
  resetDiscovery();
  vi.unstubAllGlobals();
});

// The sidebar owns the spec list; the main area starts from a picked
// preview, seeded here the way a sidebar pick leaves it.
function seedPreview(tickets = preview.tickets) {
  discovery.repo = '/repo';
  discovery.specs = [specRow];
  discovery.selected = specRow.spec;
  discovery.previewTitle = preview.title;
  discovery.tickets = tickets;
}

function stubRuns(fetchImpl) {
  vi.stubGlobal('fetch', vi.fn(fetchImpl));
}

describe('RunsView preview and live run', () => {
  it('previews blockers and progress for the picked spec', async () => {
    seedPreview();
    stubRuns(async () => ({ ok: true, json: async () => ({ runs: [] }) }));
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    expect(await within(container).findByText('First')).toBeTruthy();
    expect(within(container).getAllByText('01-first').length).toBe(2);
    expect(within(container).getByText('1/2')).toBeTruthy();
  });

  it('prompts for a sidebar pick with nothing selected', async () => {
    stubRuns(async () => ({ ok: true, json: async () => ({ runs: [] }) }));
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    expect(await within(container).findByText(/pick a spec from the sidebar/i)).toBeTruthy();
  });

  it('surfaces an unparsable Blocked by line as a row error', async () => {
    seedPreview([{ id: '02-bad', num: 2, title: 'Bad', blockers: [], done: 0, total: 1, error: 'cannot parse Blocked by line' }]);
    stubRuns(async () => ({ ok: true, json: async () => ({ runs: [] }) }));
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    expect((await within(container).findByRole('alert')).textContent).toMatch(/blocked by/i);
  });

  it('starts a Run and shows the live Ticket table with Worker links', async () => {
    const live = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status: 'running',
      tickets: [
        { id: '01-first', title: 'First', state: 'running', workerSession: 'sess-abcdef123456', lastRefusal: '' },
        { id: '02-second', title: 'Second', state: 'queued', workerSession: '', lastRefusal: '' },
      ],
    };
    seedPreview();
    stubRuns(async (u, opts) => ({
      ok: true,
      json: async () => {
        const s = String(u);
        if (s.includes('/api/runs/')) return { run: live };
        if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live };
        if (s.includes('/api/specs/preview')) return preview;
        return { specs: [specRow] };
      },
    }));
    const onOpenSession = vi.fn();
    const { container } = render(RunsView, { props: { workDir: '/repo', onOpenSession } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    await within(container).findByTestId('runs-graph');
    await fireEvent.click(within(container).getByRole('button', { name: 'List' }));
    const table = await within(container).findByTestId('live-tickets');
    expect(within(table).getByText('queued')).toBeTruthy();
    await fireEvent.click(within(table).getByRole('button'));
    expect(onOpenSession).toHaveBeenCalledWith('sess-abcdef123456');
  });

  it('shows both reviewer verdicts, tip freshness and the last refusal', async () => {
    const live = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status: 'running',
      tickets: [
        {
          id: '01-first', title: 'First', state: 'in-review', workerSession: 'sess-abcdef123456',
          verdicts: {
            'reviewer-spec': { verdict: 'pass', at: 'abcdef1234567890', fresh: true },
            'reviewer-standards': { verdict: 'changes', at: '1234567890abcdef', fresh: false },
          },
          tip: 'abcdef1234567890', lastRefusal: 'not reviewed to a pass: standards',
        },
      ],
    };
    seedPreview();
    stubRuns(async (u, opts) => ({
      ok: true,
      json: async () => {
        const s = String(u);
        if (s.includes('/api/runs/')) return { run: live };
        if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live };
        if (s.includes('/api/specs/preview')) return preview;
        return { specs: [specRow] };
      },
    }));
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    await within(container).findByTestId('runs-graph');
    await fireEvent.click(within(container).getByRole('button', { name: 'List' }));
    const table = await within(container).findByTestId('live-tickets');
    expect(within(table).getByText('Spec review')).toBeTruthy();
    expect(within(table).getByText('Standards')).toBeTruthy();
    expect(within(table).getByText('Tip')).toBeTruthy();
    expect(within(table).getByText('pass')).toBeTruthy();
    expect(within(table).getByText(/changes · stale/)).toBeTruthy();
    expect(within(table).getByText('abcdef12')).toBeTruthy();
    expect(within(table).getByText(/not reviewed to a pass/)).toBeTruthy();
  });

  it('shows the paused state with a Resume button, and Pause while running', async () => {
    let status = 'paused';
    const live = () => ({
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status,
      tickets: [
        { id: '01-first', title: 'First', state: 'running', workerSession: 'sess-abcdef123456', lastRefusal: '' },
      ],
    });
    const calls = [];
    seedPreview();
    stubRuns(async (u, opts) => {
      calls.push(String(u));
      if (String(u).endsWith('/resume')) {
        status = 'running';
        return { ok: true, json: async () => ({ run: live() }) };
      }
      if (String(u).endsWith('/pause')) {
        status = 'paused';
        return { ok: true, json: async () => ({ run: live() }) };
      }
      return {
        ok: true,
        json: async () => {
          const s = String(u);
          if (s.includes('/api/runs/')) return { run: live() };
          if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live() };
          if (s.includes('/api/specs/preview')) return preview;
          return { specs: [specRow] };
        },
      };
    });
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    await within(container).findByRole('button', { name: /resume/i });
    await fireEvent.click(await within(container).findByRole('button', { name: /resume/i }));
    expect(calls.some((u) => u.endsWith('/api/runs/run-1/resume'))).toBe(true);
    await within(container).findByRole('button', { name: /^pause/i });
    await fireEvent.click(await within(container).findByRole('button', { name: /^pause/i }));
    expect(calls.some((u) => u.endsWith('/api/runs/run-1/pause'))).toBe(true);
    await within(container).findByRole('button', { name: /resume/i });
  });

  it('offers Resume on a failed Run so it continues instead of restarting from zero', async () => {
    let status = 'failed';
    const live = () => ({
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status,
      error: 'could not finish 01-first: exit status 128',
      tickets: [
        { id: '01-first', title: 'First', state: 'running', workerSession: 'sess-abcdef123456', lastRefusal: '' },
      ],
    });
    const calls = [];
    seedPreview();
    stubRuns(async (u, opts) => {
      calls.push(String(u));
      if (String(u).endsWith('/resume')) {
        status = 'running';
        return { ok: true, json: async () => ({ run: live() }) };
      }
      return {
        ok: true,
        json: async () => {
          const s = String(u);
          if (s.includes('/api/runs/')) return { run: live() };
          if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live() };
          if (s.includes('/api/specs/preview')) return preview;
          return { specs: [specRow] };
        },
      };
    });
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    await fireEvent.click(await within(container).findByRole('button', { name: /^resume/i }));
    expect(calls.some((u) => u.endsWith('/api/runs/run-1/resume'))).toBe(true);
    await within(container).findByRole('button', { name: /^pause/i });
  });

  it('seeds the sidebar spec picker from the Run on a cold deep link', async () => {
    const live = {
      id: 'run-9',
      spec: '/repo/.scratch/demo/spec.md',
      repo: '/repo',
      status: 'running',
      tickets: [
        { id: '01-first', title: 'First', state: 'running', workerSession: '', lastRefusal: '' },
      ],
    };
    stubRuns(async (u) => ({
      ok: true,
      json: async () => {
        const s = String(u);
        if (s.includes('/api/runs/')) return { run: live };
        if (s.includes('/api/specs/preview')) return preview;
        return { specs: [specRow] };
      },
    }));
    // No repo context (cold link): the run still loads and the sidebar
    // store fills from the Run. The preview title lands last (list then
    // preview fetch), so waiting for it proves the seed finished.
    const { container } = render(RunsView, { props: { workDir: '', runId: 'run-9' } });
    expect(await within(container).findByText('First')).toBeTruthy();
    expect(await within(container).findByTestId('runs-graph')).toBeTruthy();
    expect(await within(container).findByText('Demo spec')).toBeTruthy();
    expect(discovery.specs).toHaveLength(1);
    expect(discovery.selected).toBe('/repo/.scratch/demo/spec.md');
  });

  it('shows the dependency graph by default, toggles to List, and opens tickets from cards', async () => {
    const live = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status: 'running',
      tickets: [
        { id: '01-first', num: 1, title: 'First', state: 'running', blockers: [], workerSession: '', lastRefusal: '' },
        { id: '02-second', num: 2, title: 'Second', state: 'queued', blockers: ['01-first'], workerSession: '', lastRefusal: '' },
      ],
    };
    seedPreview();
    stubRuns(async (u, opts) => ({
      ok: true,
      json: async () => {
        const s = String(u);
        if (s.includes('/api/runs/')) return { run: live };
        if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live };
        if (s.includes('/api/specs/preview')) return preview;
        return { specs: [specRow] };
      },
    }));
    const onNavigate = vi.fn();
    const { container } = render(RunsView, { props: { workDir: '/repo', onNavigate } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    // Graph first: Second waits on running First, so it reads blocked.
    await within(container).findByTestId('runs-graph');
    expect(within(container).queryByTestId('live-tickets')).toBeNull();
    expect(await within(container).findByText('blocked by 01')).toBeTruthy();
    await fireEvent.click(within(container).getByRole('button', { name: 'List' }));
    expect(await within(container).findByTestId('live-tickets')).toBeTruthy();
    expect(within(container).queryByTestId('runs-graph')).toBeNull();
    // Back to the graph, a card click deep-links to its ticket page.
    await fireEvent.click(within(container).getByRole('button', { name: 'Graph' }));
    await within(container).findByTestId('runs-graph');
    await fireEvent.click(within(container).getByRole('button', { name: /ticket second/i }));
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-1/02-second');
  });

  it('updates the graph live from Run SSE without polling', async () => {
    let snap = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      status: 'running',
      tickets: [
        { id: '01-first', num: 1, title: 'First', state: 'running', blockers: [], workerSession: '', lastRefusal: '' },
        { id: '02-second', num: 2, title: 'Second', state: 'queued', blockers: ['01-first'], workerSession: '', lastRefusal: '' },
      ],
    };
    const calls = [];
    seedPreview();
    stubRuns(async (u, opts) => {
      calls.push(String(u));
      return {
        ok: true,
        json: async () => {
          const s = String(u);
          if (s.includes('/api/runs/')) return { run: snap };
          if (s.includes('/api/runs') && opts?.method === 'POST') return { run: snap };
          if (s.includes('/api/specs/preview')) return preview;
          return { specs: [specRow] };
        },
      };
    });
    const sources = [];
    vi.stubGlobal(
      'EventSource',
      class {
        constructor(url) {
          this.url = url;
          sources.push(this);
        }
        set onmessage(fn) {
          this._fn = fn;
        }
        push(body) {
          this._fn?.({ data: JSON.stringify(body) });
        }
        close() {}
      },
    );
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/start run/i));
    await within(container).findByTestId('runs-graph');
    expect(sources.length).toBe(1);
    const gets = () => calls.filter((u) => u.includes('/api/runs/run-1')).length;
    const before = gets();
    // The Daemon publishes a fresh snapshot: First merged, Second running.
    snap = {
      ...snap,
      tickets: [
        { ...snap.tickets[0], state: 'merged' },
        { ...snap.tickets[1], state: 'running' },
      ],
    };
    sources.at(-1).push({ run: snap });
    await tick();
    const legend = await within(container).findByTestId('runs-legend');
    expect(legend.textContent).toMatch(/1 merged/);
    expect(legend.textContent).toMatch(/1 running/);
    // No follow-up fetch: the SSE snapshot alone repainted the graph.
    expect(gets()).toBe(before);
    vi.unstubAllGlobals();
  });

  it('wins the latest Run on fast runA -> runB navigation with one subscription', async () => {
    const mk = (id, title) => ({
      id,
      spec: '/repo/.scratch/demo/spec.md',
      repo: '/repo',
      status: 'running',
      tickets: [{ id: `${id}-t`, num: 1, title, state: 'running', blockers: [], workerSession: '', lastRefusal: '' }],
    });
    const resolvers = {};
    const gate = (id) => new Promise((res) => (resolvers[id] = res));
    const pending = { 'run-A': gate('run-A'), 'run-B': gate('run-B') };
    // The cold-link seed (listSpecs + preview) must answer like the Daemon.
    const specsBody = { specs: [specRow] };
    const previewBody = { spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: [] };
    stubRuns(async (u) => {
      const s = String(u);
      const m = /\/api\/runs\/(run-[AB])(?:\/|$|\?)/.exec(s);
      if (m) return pending[m[1]];
      if (s.includes('/api/specs/preview')) return { ok: true, json: async () => previewBody };
      return { ok: true, json: async () => specsBody };
    });
    class FakeSource {
      static instances = [];
      constructor(url) {
        this.url = String(url);
        this.closed = false;
        FakeSource.instances.push(this);
      }
      set onmessage(fn) {
        this._fn = fn;
      }
      close() {
        this.closed = true;
      }
    }
    FakeSource.instances = [];
    vi.stubGlobal('EventSource', FakeSource);
    const { rerender, container } = render(RunsView, { props: { workDir: '', runId: 'run-A' } });
    await tick();
    // Navigate before either fetch resolves.
    await rerender({ workDir: '', runId: 'run-B' });
    // The stale answer lands first: it must not win.
    resolvers['run-A']({ ok: true, json: async () => ({ run: mk('run-A', 'Ay') }) });
    await tick();
    await new Promise((r) => setTimeout(r, 10));
    resolvers['run-B']({ ok: true, json: async () => ({ run: mk('run-B', 'Bee') }) });
    expect(await within(container).findByText('Bee')).toBeTruthy();
    expect(within(container).queryByText('Ay')).toBeNull();
    await tick();
    await new Promise((r) => setTimeout(r, 10));
    // Exactly one subscription, to the winner.
    expect(FakeSource.instances).toHaveLength(1);
    expect(FakeSource.instances[0].url).toMatch(/run-B/);
    vi.unstubAllGlobals();
  });

  it('closes the Run stream on unmount', async () => {
    const live = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      repo: '/repo',
      status: 'running',
      tickets: [{ id: '01-first', num: 1, title: 'First', state: 'running', blockers: [], workerSession: '', lastRefusal: '' }],
    };
    const specsBody = { specs: [specRow] };
    const previewBody = { spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: [] };
    stubRuns(async (u) => {
      const s = String(u);
      if (s.includes('/api/specs/preview')) return { ok: true, json: async () => previewBody };
      if (s.includes('/api/specs')) return { ok: true, json: async () => specsBody };
      return { ok: true, json: async () => ({ run: live }) };
    });
    class FakeSource {
      static instances = [];
      constructor(url) {
        this.url = String(url);
        this.closed = false;
        FakeSource.instances.push(this);
      }
      set onmessage(fn) {
        this._fn = fn;
      }
      close() {
        this.closed = true;
      }
    }
    FakeSource.instances = [];
    vi.stubGlobal('EventSource', FakeSource);
    const { container, unmount } = render(RunsView, { props: { workDir: '', runId: 'run-1' } });
    // The graph renders at once from the loading placeholder, so wait for
    // the loaded ticket card: the stream subscribes on the same fetch.
    expect(await within(container).findByText('First')).toBeTruthy();
    await within(container).findByTestId('runs-graph');
    expect(FakeSource.instances.length).toBeGreaterThan(0);
    unmount();
    expect(FakeSource.instances.every((s) => s.closed)).toBe(true);
    vi.unstubAllGlobals();
  });

  it('banners needs-you tickets with deep-links to their ticket pages', async () => {
    const live = {
      id: 'run-1',
      spec: '/repo/.scratch/demo/spec.md',
      repo: '/repo',
      status: 'running',
      tickets: [
        { id: '01-first', num: 1, title: 'First', state: 'needs-you', blockers: [], workerSession: 'sess-1', needsYou: 'ticket 01-first needs you: the same refusal 3 times in a row' },
        { id: '02-second', num: 2, title: 'Second', state: 'running', blockers: [], workerSession: 'sess-2', needsYou: '' },
      ],
    };
    const specsBody = { specs: [specRow] };
    const previewBody = { spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: [] };
    stubRuns(async (u) => {
      const s = String(u);
      if (s.includes('/api/specs/preview')) return { ok: true, json: async () => previewBody };
      if (s.includes('/api/specs')) return { ok: true, json: async () => specsBody };
      return { ok: true, json: async () => ({ run: live }) };
    });
    const onNavigate = vi.fn();
    const { container } = render(RunsView, { props: { workDir: '', runId: 'run-1', onNavigate } });
    const banner = await within(container).findByTestId('run-needs-you');
    expect(banner.textContent).toMatch(/needs you \(1\)/i);
    expect(within(banner).getByText(/#1 first/i)).toBeTruthy();
    expect(within(banner).queryByText(/second/i)).toBeNull();
    await fireEvent.click(within(banner).getByRole('button'));
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-1/01-first');
    vi.unstubAllGlobals();
  });
});
