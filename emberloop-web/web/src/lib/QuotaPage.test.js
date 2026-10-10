import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, within, waitFor, cleanup } from '@testing-library/svelte';
import { tick } from 'svelte';
import QuotaPage from './QuotaPage.svelte';

// Fixed clock: Date.now is spied so the countdowns and age labels are exact.
const NOW = Date.UTC(2030, 0, 1);
const secs = (s) => NOW / 1000 + s;

function providerCards() {
  return [
    {
      key: 'claude',
      label: 'Claude',
      state: 'ok',
      updated_at: NOW / 1000 - 300,
      windows: [
        { label: '5h', pct: 10, resets_at: secs(3600) },
        { label: 'wk', pct: 92, resets_at: secs(6 * 86400) },
      ],
    },
    {
      key: 'opencode-go',
      label: 'OpenCode-Go',
      state: 'ok',
      updated_at: NOW / 1000,
      windows: [{ label: '5h', pct: 70, resets_at: secs(120) }],
    },
    { key: 'agy-gemini', label: 'Antigravity Gemini', state: 'not_configured', windows: [], updated_at: 0 },
    {
      key: 'agy-3p',
      label: 'Antigravity 3P',
      state: 'error',
      error: 'boom',
      updated_at: NOW / 1000,
      windows: [{ label: 'wk', pct: 80, resets_at: secs(90000) }],
    },
  ];
}

// stubFetch answers GET with one set of cards and POST with another, and
// records every call so the tests assert the request shape.
function stubFetch({ get = providerCards, post = null, gate = null } = {}) {
  const calls = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const method = opts?.method ?? 'GET';
      const body = opts?.body ? JSON.parse(opts.body) : null;
      calls.push({ url: String(u), method, body });
      if (String(u).includes('/api/quota/refresh')) {
        if (gate) await gate;
        const cards = typeof post === 'function' ? post(body) : (post ?? get);
        return { ok: true, json: async () => ({ providers: typeof cards === 'function' ? cards() : cards }) };
      }
      return { ok: true, json: async () => ({ providers: typeof get === 'function' ? get() : get }) };
    }),
  );
  return calls;
}

beforeEach(() => vi.spyOn(Date, 'now').mockReturnValue(NOW));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('QuotaPage cards', () => {
  it('renders one card per provider with bars, countdowns and the Claude age', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    const cards = await screen.findAllByTestId('quota-card');
    expect(cards.map((c) => c.dataset.key)).toEqual(['claude', 'opencode-go', 'agy-gemini', 'agy-3p']);

    const claude = container.querySelector('[data-key="claude"]');
    expect(within(claude).getByTestId('quota-updated').textContent).toBe('updated 5m ago');
    expect(within(claude).getByText('resets in 1h00m')).toBeTruthy();
    expect(within(claude).getByText('resets in 6d0h')).toBeTruthy();

    // Bar tone follows the header thresholds: 10 calm, 92 hot, 70 warm, 80 hot.
    const bar = (label) => container.querySelector(`[aria-label="${label}"] > div`);
    expect(bar('Claude 5h').className).toContain('bg-green-500');
    expect(bar('Claude wk').className).toContain('bg-red-500');
    expect(bar('OpenCode-Go 5h').className).toContain('bg-amber-500');
    expect(bar('Antigravity 3P wk').className).toContain('bg-red-500');
  });

  it('renders not-configured without bars', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    const gemini = container.querySelector('[data-key="agy-gemini"]');
    expect(within(gemini).getByTestId('quota-not-configured').textContent).toBe('Not configured');
    expect(gemini.querySelector('[role="progressbar"]')).toBeNull();
  });

  it('keeps stale windows and shows the error on a failing card', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    const tp = container.querySelector('[data-key="agy-3p"]');
    expect(within(tp).getByRole('alert').textContent).toBe('boom');
    expect(within(tp).getByTestId('quota-pct').textContent).toBe('80%');
  });
});

describe('QuotaPage refresh', () => {
  it('refreshes one card with its key', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    await screen.findByTestId('quota-page');
    await fireEvent.click(await screen.findByRole('button', { name: 'Refresh Claude' }));
    const post = calls.find((c) => c.method === 'POST');
    expect(post.url).toContain('/api/quota/refresh');
    expect(post.body).toEqual({ key: 'claude' });
  });

  it('refreshes all with an empty body', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    await screen.findByTestId('quota-page');
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh all' }));
    const post = calls.find((c) => c.method === 'POST');
    expect(post.body).toEqual({});
  });

  it('shows a card busy while the refresh is in flight', async () => {
    let release;
    const gate = new Promise((r) => {
      release = r;
    });
    stubFetch({ gate });
    render(QuotaPage);
    await screen.findByTestId('quota-page');
    const btn = await screen.findByRole('button', { name: 'Refresh Claude' });
    fireEvent.click(btn);
    await tick();
    expect(btn.disabled).toBe(true);
    release();
    await waitFor(() => expect(btn.disabled).toBe(false));
  });
});

describe('QuotaPage loading', () => {
  it('fetches once on mount and never polls', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    expect(calls.filter((c) => c.method === 'GET')).toHaveLength(1);
    await new Promise((r) => setTimeout(r, 60));
    expect(calls.filter((c) => c.method === 'GET')).toHaveLength(1);
    expect(calls.filter((c) => c.method === 'POST')).toHaveLength(0);
  });
});
