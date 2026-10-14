import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, within, waitFor, cleanup } from '@testing-library/svelte';
import { tick } from 'svelte';
import QuotaPage from './QuotaPage.svelte';

const NOW = Date.UTC(2030, 0, 1);
const secs = (s) => NOW / 1000 + s;

function providerCards() {
  return [
    {
      key: 'claude',
      label: 'Claude',
      plan: 'Pro',
      source: 'Host login',
      state: 'ok',
      updated_at: NOW / 1000 - 8,
      windows: [
        { label: 'five_hour', pct: 36, resets_at: secs(3600), detail: '5-hour detail' },
        { label: 'seven_day', pct: 8, resets_at: secs(6 * 86400) },
      ],
    },
    {
      key: 'chatgpt',
      label: 'ChatGPT',
      plan: 'Plus',
      source: 'pig login',
      state: 'expired',
      updated_at: NOW / 1000 - 3 * 3600,
      windows: [{ label: '5h', pct: 4, resets_at: secs(4 * 3600) }],
    },
    {
      key: 'opencode-go',
      label: 'OpenCode-Go',
      plan: 'Go',
      source: 'API key',
      state: 'stale',
      error: 'upstream unavailable',
      updated_at: NOW / 1000 - 12 * 60,
      windows: [
        { label: '5h', pct: 0, resets_at: secs(120) },
        { label: 'wk', pct: 7, resets_at: secs(5 * 86400) },
      ],
    },
    {
      key: 'agy-gemini',
      label: 'Antigravity Gemini',
      plan: 'Gemini',
      source: 'pig login',
      state: 'not_configured',
      hint: 'Needs ~/.el/agy-oauth.json. Shows Gemini and Claude + GPT-OSS pools.',
      windows: [],
      updated_at: 0,
    },
    {
      key: 'openrouter',
      label: 'OpenRouter',
      plan: 'Credits',
      source: 'API key',
      state: 'ok',
      updated_at: NOW / 1000 - 8,
      windows: [
        { label: 'Credits', pct: 94, detail: '$59.16 of $63.00 · $3.84 left' },
        { label: 'Spend this week', pct: null, detail: '$2.37 · today $0.91 · month $5.70' },
      ],
    },
  ];
}

function stubFetch({ get = providerCards, post = null, gate = null } = {}) {
  const calls = [];
  vi.stubGlobal('fetch', vi.fn(async (url, opts) => {
    const method = opts?.method ?? 'GET';
    const body = opts?.body ? JSON.parse(opts.body) : null;
    calls.push({ url: String(url), method, body });
    if (String(url).includes('/api/quota/refresh')) {
      if (gate) await gate;
      const cards = typeof post === 'function' ? post(body) : (post ?? get);
      return { ok: true, json: async () => ({ providers: typeof cards === 'function' ? cards() : cards }) };
    }
    return { ok: true, json: async () => ({ providers: typeof get === 'function' ? get() : get }) };
  }));
  return calls;
}

beforeEach(() => vi.spyOn(Date, 'now').mockReturnValue(NOW));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('QuotaPage cards', () => {
  it('renders the redesigned read-only header and a two-column card grid', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    expect(await screen.findByRole('heading', { name: 'Quota' })).toBeTruthy();
    expect(screen.getByText('Read-only')).toBeTruthy();
    expect(screen.getByText('Percent used per account window, and when it resets. Dollars and tokens are secondary. Updates live.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Refresh all' })).toBeTruthy();
    expect(container.querySelector('.grid.gap-4.sm\\:grid-cols-2')).toBeTruthy();
  });

  it('renders percent-first cards, word labels, plan badges, sources and countdowns', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    const cards = await screen.findAllByTestId('quota-card');
    expect(cards.map((card) => card.dataset.key)).toEqual(['claude', 'chatgpt', 'opencode-go', 'agy-gemini', 'openrouter']);

    const claude = container.querySelector('[data-key="claude"]');
    expect(within(claude).getByTestId('quota-plan').textContent).toBe('Pro');
    expect(within(claude).getByTestId('quota-updated').textContent).toBe('updated 8s ago · Host login');
    expect(within(claude).getByRole('progressbar', { name: 'Claude 5-hour' }).getAttribute('aria-valuenow')).toBe('36');
    expect(within(claude).getByText('5-hour')).toBeTruthy();
    expect(within(claude).getByText('Weekly')).toBeTruthy();
    expect(within(claude).getByText('resets in 1h00m')).toBeTruthy();
    expect(within(claude).getByText('resets in 6d0h')).toBeTruthy();

    const bars = (label) => container.querySelector(`[aria-label="${label}"] > div`);
    expect(bars('Claude 5-hour').className).toContain('bg-green-500');
    expect(bars('OpenCode-Go 5-hour').className).toContain('bg-green-500');
    expect(bars('OpenRouter Credits').className).toContain('bg-red-500');
    expect(within(container.querySelector('[data-key="opencode-go"]')).getByTestId('quota-state').textContent).toContain('stale');
    expect(within(container.querySelector('[data-key="opencode-go"]')).getByTestId('quota-updated').textContent).toBe('updated 12m ago · retrying');
  });

  it('shows expired values dimmed with a host-login callout and connect link', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    const cards = await screen.findAllByTestId('quota-card');
    const expired = container.querySelector('[data-key="chatgpt"]');
    expect(within(expired).getByTestId('quota-state').textContent).toContain('login expired');
    expect(within(expired).getByTestId('quota-updated').textContent).toBe('last read 3h ago');
    expect(within(expired).getByTestId('quota-expired-callout').textContent).toContain('ChatGPT login expired');
    expect(within(expired).getByTestId('quota-windows').className).toContain('opacity-50');
    expect(within(expired).getByText('4%')).toBeTruthy();
    expect(within(expired).getByRole('link', { name: 'Reconnect in Providers' }).getAttribute('href')).toBe('/providers');
  });

  it('renders not-connected hints and a one-click Providers action without bars', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    const gemini = container.querySelector('[data-key="agy-gemini"]');
    expect(within(gemini).getByTestId('quota-state').textContent).toBe('not connected');
    expect(within(gemini).getByTestId('quota-not-connected').textContent).toContain('~/.el/agy-oauth.json');
    expect(within(gemini).getByTestId('quota-not-connected').textContent).toContain('Claude + GPT-OSS pools');
    expect(within(gemini).getByRole('link', { name: 'Connect in Providers' }).getAttribute('href')).toBe('/providers');
    expect(gemini.querySelector('[role="progressbar"]')).toBeNull();
  });

  it('renders OpenRouter spend as a text-only row, with details separate from its percent', async () => {
    stubFetch();
    const { container } = render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    const router = container.querySelector('[data-key="openrouter"]');
    expect(within(router).getByText('94%')).toBeTruthy();
    expect(within(router).getByText('$59.16 of $63.00 · $3.84 left')).toBeTruthy();
    const spend = within(router).getByTestId('quota-text-window');
    expect(spend.textContent).toContain('Spend this week');
    expect(spend.textContent).toContain('$2.37 · today $0.91 · month $5.70');
    expect(spend.className).toContain('border-t');
    expect(spend.querySelector('[role="progressbar"]')).toBeNull();
    expect(within(router).getAllByRole('progressbar')).toHaveLength(1);
  });
});

describe('QuotaPage refresh', () => {
  it('refreshes a card by key', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    const btn = await screen.findByRole('button', { name: 'Refresh Claude' });
    await fireEvent.click(btn);
    const post = calls.find((call) => call.method === 'POST');
    expect(post.url).toContain('/api/quota/refresh');
    expect(post.body).toEqual({ key: 'claude' });
  });

  it('refreshes all with an empty body', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    await fireEvent.click(await screen.findByRole('button', { name: 'Refresh all' }));
    const post = calls.find((call) => call.method === 'POST');
    expect(post.body).toEqual({});
  });

  it('shows busy controls without removing the last values during refresh', async () => {
    let release;
    const gate = new Promise((resolve) => { release = resolve; });
    stubFetch({ gate });
    render(QuotaPage);
    const pct = await screen.findByText('36%');
    const btn = screen.getByRole('button', { name: 'Refresh Claude' });
    fireEvent.click(btn);
    await tick();
    expect(btn.disabled).toBe(true);
    expect(btn.getAttribute('aria-busy')).toBe('true');
    expect(pct.isConnected).toBe(true);
    expect(screen.getByText('36%')).toBeTruthy();
    release();
    await waitFor(() => expect(btn.disabled).toBe(false));
  });

  it('shows refresh-all busy state and preserves card values until it completes', async () => {
    let release;
    const gate = new Promise((resolve) => { release = resolve; });
    stubFetch({ gate });
    render(QuotaPage);
    expect(await screen.findByText('36%')).toBeTruthy();
    const btn = screen.getByRole('button', { name: 'Refresh all' });
    fireEvent.click(btn);
    await tick();
    expect(btn.disabled).toBe(true);
    expect(btn.getAttribute('aria-busy')).toBe('true');
    expect(screen.getByText('36%')).toBeTruthy();
    release();
    await waitFor(() => expect(btn.disabled).toBe(false));
  });
});

describe('QuotaPage loading', () => {
  it('fetches once on mount and does not poll', async () => {
    const calls = stubFetch();
    render(QuotaPage);
    await screen.findAllByTestId('quota-card');
    expect(calls.filter((call) => call.method === 'GET')).toHaveLength(1);
    await new Promise((resolve) => setTimeout(resolve, 60));
    expect(calls.filter((call) => call.method === 'GET')).toHaveLength(1);
    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
  });
});
