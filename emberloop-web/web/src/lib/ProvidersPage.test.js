import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import ProvidersPage from './ProvidersPage.svelte';

const secret = 'provider-browser-secret';

function providerRows() {
  return [
    {
      id: 'openai-codex',
      name: 'ChatGPT Plus',
      group: 'subscription_logins',
      connected: true,
      source: 'oauth',
      sourceLabel: 'OAuth login',
      keySet: false,
      keyEditable: false,
    },
    {
      id: 'openrouter',
      name: 'OpenRouter',
      group: 'api_keys',
      connected: true,
      source: 'stored_key',
      sourceLabel: 'Stored key',
      keySet: true,
      keyEditable: true,
    },
    {
      id: 'xai',
      name: 'xAI',
      group: 'cli_only',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      command: 'pig login xai',
    },
  ];
}

function stubProviders() {
  let rows = providerRows();
  const calls = [];
  const fetchMock = vi.fn(async (input, options = {}) => {
    const url = String(input);
    const method = options.method ?? 'GET';
    calls.push({ url, method, body: options.body ? JSON.parse(options.body) : null });
    if (method === 'PUT') {
      rows = rows.map((row) => row.id === 'openrouter'
        ? { ...row, connected: true, source: 'stored_key', sourceLabel: 'Stored key', keySet: true }
        : row);
      return { ok: true, json: async () => ({ ok: true }) };
    }
    if (method === 'DELETE') {
      rows = rows.map((row) => row.id === 'openrouter'
        ? { ...row, connected: false, source: 'none', sourceLabel: 'Not connected', keySet: false }
        : row);
      return { ok: true, json: async () => ({ ok: true }) };
    }
    return { ok: true, json: async () => ({ providers: rows }) };
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('ProvidersPage', () => {
  it('groups SDK provider statuses without rendering credential values', async () => {
    stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });

    expect(await screen.findByRole('heading', { name: 'Providers' })).toBeTruthy();
    expect(await screen.findByRole('heading', { name: 'Subscription logins' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'API keys' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'CLI-only' })).toBeTruthy();
    expect(within(container.querySelector('[data-provider="openai-codex"]')).getByTestId('provider-source').textContent).toContain('OAuth login');
    expect(within(container.querySelector('[data-provider="openrouter"]')).getByTestId('provider-source').textContent).toContain('API key set');
    await screen.findAllByTestId('provider-row');
    expect(container.textContent).not.toContain(secret);
  });

  it('lets admins set and remove a key while only reading the set flag back', async () => {
    const calls = stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');
    const keyRow = container.querySelector('[data-provider="openrouter"]');

    await fireEvent.click(within(keyRow).getByRole('button', { name: 'Replace key' }));
    const input = within(keyRow).getByLabelText('API key for OpenRouter');
    expect(input.type).toBe('password');
    await fireEvent.input(input, { target: { value: secret } });
    await fireEvent.click(within(keyRow).getByRole('button', { name: 'Save key' }));
    await waitFor(() => expect(screen.queryByLabelText('API key for OpenRouter')).toBeNull());
    expect(calls.find((call) => call.method === 'PUT')).toMatchObject({
      url: '/api/providers/openrouter/key',
      body: { key: secret },
    });
    expect(container.textContent).not.toContain(secret);

    const removeButton = within(keyRow).getByRole('button', { name: 'Remove' });
    await waitFor(() => expect(removeButton.disabled).toBe(false));
    await fireEvent.click(removeButton);
    await waitFor(() => expect(within(keyRow).getByTestId('provider-status').textContent).toBe('Not connected'));
    expect(calls.find((call) => call.method === 'DELETE').url).toBe('/api/providers/openrouter/key');
    expect(within(keyRow).queryByTestId('provider-key-set')).toBeNull();
    expect(container.textContent).not.toContain(secret);
  });

  it('shows non-admins read-only status and keeps key controls locked', async () => {
    stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: false } });
    await screen.findAllByTestId('provider-row');

    expect(screen.getByTestId('providers-read-only').textContent).toContain('Ask an admin');
    const keyRow = container.querySelector('[data-provider="openrouter"]');
    expect(within(keyRow).getByText('Admin only')).toBeTruthy();
    expect(within(keyRow).queryByRole('button', { name: /key|remove/i })).toBeNull();
    expect(within(keyRow).getByTestId('provider-key-set')).toBeTruthy();
    const cliRow = container.querySelector('[data-provider="xai"]');
    expect(within(cliRow).getByText('Admin only')).toBeTruthy();
    expect(within(cliRow).queryByText('pig login xai')).toBeNull();
    expect(within(cliRow).queryByRole('button', { name: /copy/i })).toBeNull();
  });

  it('keeps provider rows and key controls phone-friendly', async () => {
    stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    const [group] = await screen.findAllByTestId('provider-group');
    expect(group.className).not.toMatch(/(?:sm|md):grid-cols-/);

    const keyRow = container.querySelector('[data-provider="openrouter"]');
    const replace = within(keyRow).getByRole('button', { name: 'Replace key' });
    expect([...replace.classList].some((name) => name === 'max-md:h-11' || name === 'max-md:min-h-11')).toBe(true);
    await fireEvent.click(replace);
    const input = within(keyRow).getByLabelText('API key for OpenRouter');
    expect(input.className).toContain('text-base');
    expect(input.className).toContain('h-11');
  });
});
