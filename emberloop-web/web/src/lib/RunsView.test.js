import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import RunsView from './RunsView.svelte';

function stubApi({ specs, preview }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u) => ({
      ok: true,
      json: async () => {
        if (String(u).includes('/api/specs/preview')) return preview;
        return specs;
      },
    })),
  );
}

const preview = {
  spec: '/repo/.scratch/demo/spec.md',
  title: 'Demo spec',
  tickets: [
    { id: '01-first', num: 1, title: 'First', blockers: [], done: 1, total: 2 },
    { id: '02-second', num: 2, title: 'Second', blockers: ['01-first'], done: 0, total: 1 },
  ],
};

describe('RunsView', () => {
  it('shows a clear message when no spec is found', async () => {
    stubApi({ specs: { specs: [] }, preview });
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    expect(await within(container).findByText(/no specs found/i)).toBeTruthy();
  });

  it('lists specs with title and count, then previews blockers and progress', async () => {
    stubApi({ specs: { specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 }] }, preview });
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/demo spec/i));
    await tick();
    expect(await within(container).findByText('First')).toBeTruthy();
    expect(within(container).getAllByText('01-first').length).toBe(2);
    expect(within(container).getByText('1/2')).toBeTruthy();
  });

  it('surfaces an unparsable Blocked by line as a row error', async () => {
    const bad = {
      ...preview,
      tickets: [{ id: '02-bad', num: 2, title: 'Bad', blockers: [], done: 0, total: 1, error: 'cannot parse Blocked by line' }],
    };
    stubApi({ specs: { specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 1 }] }, preview: bad });
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/demo spec/i));
    await tick();
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
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => ({
        ok: true,
        json: async () => {
          const s = String(u);
          if (s.includes('/api/runs/')) return { run: live };
          if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live };
          if (s.includes('/api/specs/preview')) return preview;
          return { specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 }] };
        },
      })),
    );
    const onOpenSession = vi.fn();
    const { container } = render(RunsView, { props: { workDir: '/repo', onOpenSession } });
    await fireEvent.click(await within(container).findByText(/demo spec/i));
    await tick();
    await fireEvent.click(await within(container).findByText(/start run/i));
    const table = await within(container).findByTestId('live-tickets');
    expect(within(table).getByText('running')).toBeTruthy();
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
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u, opts) => ({
        ok: true,
        json: async () => {
          const s = String(u);
          if (s.includes('/api/runs/')) return { run: live };
          if (s.includes('/api/runs') && opts?.method === 'POST') return { run: live };
          if (s.includes('/api/specs/preview')) return preview;
          return { specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 1 }] };
        },
      })),
    );
    const { container } = render(RunsView, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/demo spec/i));
    await tick();
    await fireEvent.click(await within(container).findByText(/start run/i));
    const table = await within(container).findByTestId('live-tickets');
    expect(within(table).getByText('Spec review')).toBeTruthy();
    expect(within(table).getByText('Standards')).toBeTruthy();
    expect(within(table).getByText('Tip')).toBeTruthy();
    expect(within(table).getByText('pass')).toBeTruthy();
    expect(within(table).getByText(/changes \(stale\)/)).toBeTruthy();
    expect(within(table).getByText('abcdef12')).toBeTruthy();
    expect(within(table).getByText(/not reviewed to a pass/)).toBeTruthy();
  });
});
