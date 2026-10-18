import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
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
afterEach(() => cleanup());

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
  it('loads specs from the Project path without asking for a folder', async () => {
    stubSpecs({ specs: [{ spec: '/project/.scratch/demo/spec.md', title: 'Demo spec', tickets: 2 }] });
    const { container } = render(RunsSidebar, { props: { workDir: '/project', projectId: 'project-1' } });
    expect(await within(container).findByTestId('project-runs-discovery')).toBeTruthy();
    expect(within(container).getByText('Specs in /project')).toBeTruthy();
    expect(within(container).queryByLabelText(/repo directory/i)).toBeNull();
    expect(await within(container).findByRole('button', { name: /demo spec/i })).toBeTruthy();
    expect(discovery.repo).toBe('/project');
  });

  it('waits for Project loading before reporting an unavailable path', async () => {
    stubSpecs({ specs: [] });
    const { container, rerender } = render(RunsSidebar, {
      props: { workDir: '', projectId: 'project-1', projectsLoaded: false },
    });
    expect(within(container).queryByText('Project folder is unavailable.')).toBeNull();

    await rerender({ workDir: '/project', projectId: 'project-1', projectsLoaded: true });
    expect(await within(container).findByText(/no specs found under this directory/i)).toBeTruthy();
    expect(within(container).queryByText('Project folder is unavailable.')).toBeNull();
  });
  it('ignores stale discovery results after switching Projects', async () => {
    let releaseOld;
    let oldResponseRead;
    const oldResponseReadPromise = new Promise((resolve) => { oldResponseRead = resolve; });
    const fetchMock = vi.fn((input) => {
      const url = String(input);
      if (url.includes('repo=%2Fold')) {
        return new Promise((resolve) => {
          releaseOld = () => resolve({
            ok: true,
            json: async () => {
              oldResponseRead();
              return { specs: [{ spec: '/old/spec.md', title: 'Old spec', tickets: 1 }] };
            },
          });
        });
      }
      if (url.includes('repo=%2Fnew')) {
        return Promise.resolve({
          ok: true,
          json: async () => ({ specs: [{ spec: '/new/spec.md', title: 'New spec', tickets: 1 }] }),
        });
      }
      return Promise.resolve({ ok: true, json: async () => ({ runs: [] }) });
    });
    vi.stubGlobal('fetch', fetchMock);
    const { container, rerender } = render(RunsSidebar, {
      props: { workDir: '/old', projectId: 'project-old', projectsLoaded: true },
    });
    await vi.waitFor(() => expect(releaseOld).toBeTypeOf('function'));

    await rerender({ workDir: '/new', projectId: 'project-new', projectsLoaded: true });
    expect(await within(container).findByRole('button', { name: /new spec/i })).toBeTruthy();
    releaseOld();
    await oldResponseReadPromise;
    await tick();

    expect(within(container).queryByRole('button', { name: /old spec/i })).toBeNull();
    expect(discovery.repo).toBe('/new');
  });
});


describe('RunsSidebar run list', () => {
  const runsBody = {
    runs: [
      {
        id: 'run-old',
        spec: '/repo/.scratch/demo/spec.md',
        status: 'done',
        repo: '/repo',
        projectId: 'project-1',
        owner: 'user-1',
        updatedAt: '2026-10-08T10:00:00Z',
        counts: { total: 1, merged: 1, needsYou: 0 },
      },
      {
        id: 'run-new',
        spec: '/repo/.scratch/demo/spec.md',
        status: 'needs-you',
        repo: '/repo',
        projectId: 'project-1',
        owner: 'user-1',
        updatedAt: '2026-10-09T10:00:00Z',
        counts: { total: 2, merged: 0, needsYou: 1 },
      },
    ],
  };
  function stubRuns() {
    const fetchMock = vi.fn(async (u) => ({
      ok: true,
      json: async () => {
        if (String(u).includes('/api/specs/preview')) return preview;
        if (String(u).includes('/api/specs')) return { specs: [{ spec: preview.spec, title: preview.title, tickets: 2 }] };
        return runsBody;
      },
    }));
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
  }

  it('lists runs newest first with merged counts and navigates on pick', async () => {
    const fetchMock = stubRuns();
    const onNavigate = vi.fn();
    const { container } = render(RunsSidebar, { props: { workDir: '/repo', onNavigate } });
    const items = await within(container).findAllByRole('button', { name: /0\/2|1\/1/ });
    expect(items).toHaveLength(2);
    // Newest run first: 0/2 before 1/1.
    expect(items[0].textContent).toMatch(/0\/2/);
    expect(items[1].textContent).toMatch(/1\/1/);
    expect(fetchMock.mock.calls.some(([url]) => /\/api\/runs\/[^/]+$/.test(String(url)))).toBe(false);
    await fireEvent.click(items[0]);
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-new');
  });

  it('seeds a deep link from the Run summary without fetching ticket detail', async () => {
    const fetchMock = stubRuns();
    const { container } = render(RunsSidebar, { props: { workDir: '', runId: 'run-new' } });
    await vi.waitFor(() => expect(discovery.selected).toBe(preview.spec));
    expect(discovery.repo).toBe('/repo');
    expect(within(container).getByRole('button', { name: /Demo spec/ })).toBeTruthy();
    expect(fetchMock.mock.calls.some(([url]) => /\/api\/runs\/[^/]+$/.test(String(url)))).toBe(false);
  });

  it('shows needs-you counts from summaries and navigates to Run detail', async () => {
    stubRuns();
    const onNavigate = vi.fn();
    const { container } = render(RunsSidebar, { props: { workDir: '/repo', onNavigate } });
    const strip = await within(container).findByTestId('needs-you-strip');
    expect(strip.textContent).toMatch(/needs you \(1\)/i);
    await fireEvent.click(within(strip).getByRole('button', { name: /1 ticket need attention/i }));
    expect(onNavigate).toHaveBeenCalledWith('/runs/run-new');
  });

  it('shows needs-you Runs with the needs-you glyph, not working', async () => {
    stubRuns();
    const { container } = render(RunsSidebar, { props: { workDir: '/repo', onNavigate: vi.fn() } });
    await within(container).findByTestId('needs-you-strip');
    expect(container.querySelector('[data-status="needs-you"]')).not.toBeNull();
  });
});
