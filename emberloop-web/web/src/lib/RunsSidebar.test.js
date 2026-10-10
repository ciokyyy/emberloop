import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import RunsSidebar from './RunsSidebar.svelte';
import { discovery, resetDiscovery } from './runsDiscovery.svelte.js';

const preview = {
  spec: '/repo/.scratch/demo/spec.md',
  title: 'Demo spec',
  tickets: [
    { id: '01-first', num: 1, title: 'First', blockers: [], done: 1, total: 2 },
    { id: '02-second', num: 2, title: 'Second', blockers: ['01-first'], done: 0, total: 1 },
  ],
};

function stubSpecs(specs) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u) => ({
      ok: true,
      json: async () => {
        if (String(u).includes('/api/specs/preview')) return preview;
        if (String(u).includes('/api/specs')) return specs;
        return { runs: [] };
      },
    })),
  );
}

beforeEach(() => {
  resetDiscovery();
  vi.unstubAllGlobals();
});

describe('RunsSidebar spec discovery', () => {
  it('shows a clear message when no spec is found', async () => {
    stubSpecs({ specs: [] });
    const { container } = render(RunsSidebar, { props: { workDir: '/repo' } });
    expect(await within(container).findByText(/no specs found/i)).toBeTruthy();
  });

  it('asks for a repo directory when blank', async () => {
    stubSpecs({ specs: [] });
    const { container } = render(RunsSidebar, { props: { workDir: '' } });
    await fireEvent.click(await within(container).findByRole('button', { name: /find specs/i }));
    expect(await within(container).findByText(/enter a repo directory/i)).toBeTruthy();
  });

  it('lists specs with title and count, then picks the preview', async () => {
    stubSpecs({ specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 }] });
    const { container } = render(RunsSidebar, { props: { workDir: '/repo' } });
    await fireEvent.click(await within(container).findByText(/demo spec/i));
    await tick();
    await tick();
    expect(discovery.selected).toBe('/repo/.scratch/demo/spec.md');
    expect(discovery.previewTitle).toBe('Demo spec');
    expect(discovery.tickets).toHaveLength(2);
  });

  it('finds specs from the repo form', async () => {
    stubSpecs({ specs: [{ spec: '/repo/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 }] });
    const { container } = render(RunsSidebar, { props: { workDir: '' } });
    const input = await within(container).findByLabelText(/repo directory/i);
    await fireEvent.input(input, { target: { value: '/repo' } });
    await tick();
    await fireEvent.click(within(container).getByRole('button', { name: /find specs/i }));
    expect(await within(container).findByText(/demo spec/i)).toBeTruthy();
  });
});

describe('RunsSidebar run list', () => {
  const runsBody = {
    runs: [
      {
        id: 'run-old',
        spec: '/repo/.scratch/demo/spec.md',
        status: 'done',
        createdAt: '2026-10-08T10:00:00Z',
        tickets: [{ id: '01-first', state: 'merged' }],
      },
      {
        id: 'run-new',
        spec: '/repo/.scratch/demo/spec.md',
        status: 'running',
        createdAt: '2026-10-09T10:00:00Z',
        tickets: [
          { id: '01-first', state: 'needs-you', needsYou: 'same refusal ×3', num: 1, title: 'First' },
          { id: '02-second', state: 'running', num: 2, title: 'Second' },
        ],
      },
    ],
  };

  function stubRuns() {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => {
          if (String(u).includes('/api/specs/preview')) return preview;
          if (String(u).includes('/api/specs')) return { specs: [] };
          return runsBody;
        },
      })),
    );
  }

  it('lists runs newest first with merged counts and navigates on pick', async () => {
    stubRuns();
    const onNavigate = vi.fn();
    const { container } = render(RunsSidebar, { props: { workDir: '/repo', onNavigate } });
    const items = await within(container).findAllByRole('button', { name: /0\/2|1\/1/ });
    expect(items).toHaveLength(2);
    // Newest run first: 0/2 before 1/1.
    expect(items[0].textContent).toMatch(/0\/2/);
    expect(items[1].textContent).toMatch(/1\/1/);
    await fireEvent.click(items[0]);
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-new');
  });

  it('strips needs-you tickets out to their ticket pages', async () => {
    stubRuns();
    const onNavigate = vi.fn();
    const { container } = render(RunsSidebar, { props: { workDir: '/repo', onNavigate } });
    const strip = await within(container).findByTestId('needs-you-strip');
    expect(strip.textContent).toMatch(/needs you \(1\)/i);
    await fireEvent.click(within(strip).getByRole('button', { name: /first/i }));
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-new/01-first');
  });
});
