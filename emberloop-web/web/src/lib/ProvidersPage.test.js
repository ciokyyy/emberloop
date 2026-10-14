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
      id: 'claude',
      name: 'Claude',
      group: 'subscription_logins',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
    },
    {
      id: 'google-antigravity',
      name: 'Google Antigravity',
      group: 'subscription_logins',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      connectable: true,
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
      command: 'pig login openrouter',
    },
    {
      id: 'anthropic',
      name: 'Anthropic',
      group: 'subscription_logins',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      command: 'pig login anthropic',
    },
    {
      id: 'amazon-bedrock',
      name: 'Amazon Bedrock',
      group: 'cli_only',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      cliHint: 'configure via CLI',
    },
    {
      id: 'google-vertex',
      name: 'Google Vertex',
      group: 'cli_only',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      cliHint: 'configure via CLI',
    },
    {
      id: 'cloudflare-ai-gateway',
      name: 'Cloudflare AI Gateway',
      group: 'cli_only',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      cliHint: 'configure via CLI',
    },
    {
      id: 'cloudflare-workers-ai',
      name: 'Cloudflare Workers AI',
      group: 'cli_only',
      connected: false,
      source: 'none',
      sourceLabel: 'Not connected',
      keySet: false,
      keyEditable: false,
      cliHint: 'configure via CLI',
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

function antigravityLoginSnapshot(state, fields = {}) {
  return Object.fromEntries(
    Object.entries({ id: 'login-1', state, ...fields }).filter(([, value]) => value !== ''),
  );
}

function stubProviders() {
  let rows = providerRows();
  let claudeLoginState = 'running';
  let antigravityLoginState;
  let missingLogin = false;
  const calls = [];
  const connectURL = '/api/providers/google-antigravity/connect';
  const url = 'https://accounts.google.com/o/oauth2/v2/auth';
  const instructions = 'Sign in, then paste the localhost callback URL or code.';
  const fetchMock = vi.fn(async (input, options = {}) => {
    const requestURL = String(input);
    const method = options.method ?? 'GET';
    calls.push({ url: requestURL, method, body: options.body ? JSON.parse(options.body) : null });
    if (requestURL === '/api/providers/claude/login' && method === 'POST') {
      return { ok: true, json: async () => ({ id: 'login-1', state: 'running', output: '\u001b[31mVisit https://claude.ai/oauth/authorize?code=abc\u001b[0m\nPaste code:' }) };
    }
    if (requestURL === '/api/providers/claude/login/login-1/code' && method === 'POST') {
      claudeLoginState = 'succeeded';
      rows = rows.map((row) => row.id === 'claude'
        ? { ...row, connected: true, source: 'host_claude', sourceLabel: 'Claude Code login' }
        : row);
      return { ok: true, json: async () => ({ ok: true }) };
    }
    if (requestURL === '/api/providers/claude/login/login-1' && method === 'GET') {
      return { ok: true, json: async () => ({ id: 'login-1', state: claudeLoginState, output: '\u001b[32mVisit https://claude.ai/oauth/authorize?code=abc\u001b[0m\nLogin successful' }) };
    }
    if (requestURL === '/api/quota/refresh' && method === 'POST') {
      return { ok: true, json: async () => ({ providers: [] }) };
    }
    if (requestURL === connectURL && method === 'POST') {
      antigravityLoginState = antigravityLoginSnapshot('waiting', {
        url,
        instructions,
        prompt: 'Paste the Antigravity OAuth callback URL or code:',
      });
      return { ok: true, json: async () => ({ ...antigravityLoginState }) };
    }
    if (requestURL.startsWith(`${connectURL}/`)) {
      if (method === 'GET') {
        if (missingLogin) return { ok: false, status: 404 };
        return { ok: true, json: async () => ({ ...antigravityLoginState }) };
      }
      if (method === 'POST') {
        if (options.body && JSON.parse(options.body).value === 'bad callback') {
          antigravityLoginState = antigravityLoginSnapshot('waiting', {
            url,
            instructions,
            prompt: 'Paste the Antigravity OAuth callback URL or code:',
            error: 'The callback URL or code is invalid.',
          });
        } else {
          antigravityLoginState = antigravityLoginSnapshot('progress', { url, instructions });
        }
        return { ok: true, json: async () => ({ ok: true }) };
      }
      if (method === 'DELETE') return { ok: true, json: async () => ({ ok: true }) };
    }
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
  return {
    calls,
    completeLogin() {
      antigravityLoginState = antigravityLoginSnapshot('complete', {
        url,
        instructions,
        progress: 'Project discovery complete.',
      });
      rows = rows.map((row) => row.id === 'google-antigravity'
        ? { ...row, connected: true, source: 'oauth', sourceLabel: 'OAuth login' }
        : row);
    },
    expireLogin() {
      missingLogin = true;
    },
  };
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('ProvidersPage', () => {
  it('shows a loading skeleton until provider status arrives', async () => {
    let resolveFetch;
    vi.stubGlobal('fetch', vi.fn(() => new Promise((resolve) => { resolveFetch = resolve; })));
    render(ProvidersPage, { props: { admin: true } });

    expect(screen.queryByTestId('provider-row')).toBeNull();
    resolveFetch({ ok: true, json: async () => ({ providers: providerRows() }) });
    expect(await screen.findAllByTestId('provider-row')).toHaveLength(10);
    expect(screen.queryByTestId('providers-loading')).toBeNull();
  });

  it('shows an alert when loading provider status fails', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 503 })));
    render(ProvidersPage, { props: { admin: false } });

    expect((await screen.findByRole('alert')).textContent).toContain('request failed: 503');
    expect(screen.queryByTestId('providers-loading')).toBeNull();
  });

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
  it('copies CLI-only OAuth commands and names cloud-auth CLI configuration', async () => {
    const writeText = vi.fn(async () => {});
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');

    const oauthRow = container.querySelector('[data-provider="anthropic"]');
    const cloudProviderIDs = [
      'amazon-bedrock',
      'google-vertex',
      'cloudflare-ai-gateway',
      'cloudflare-workers-ai',
    ];
    for (const id of cloudProviderIDs) {
      const cloudRow = container.querySelector(`[data-provider="${id}"]`);
      expect(within(cloudRow).getByTestId('provider-cli-guidance').textContent).toBe('configure via CLI');
      expect(within(cloudRow).queryByRole('button', { name: /copy/i })).toBeNull();
    }
    expect(within(oauthRow).getByText('pig login anthropic')).toBeTruthy();
    await fireEvent.click(within(oauthRow).getByRole('button', { name: 'Copy Anthropic command' }));
    const openrouterRow = container.querySelector('[data-provider="openrouter"]');
    expect(within(openrouterRow).getByText('pig login openrouter')).toBeTruthy();
    await fireEvent.click(within(openrouterRow).getByRole('button', { name: 'Copy OpenRouter command' }));
    expect(writeText).toHaveBeenCalledWith('pig login openrouter');
    expect(writeText).toHaveBeenCalledWith('pig login anthropic');
  });

  it('connects Claude through the host PTY, sends the pasted code, and refreshes its quota', async () => {
    const { calls } = stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');
    const claudeRow = container.querySelector('[data-provider="claude"]');

    await fireEvent.click(within(claudeRow).getByRole('button', { name: 'Connect' }));
    expect(await within(claudeRow).findByTestId('claude-login-panel')).toBeTruthy();
    expect(within(claudeRow).getByRole('link', { name: 'Open Claude sign-in page' }).getAttribute('href'))
      .toBe('https://claude.ai/oauth/authorize?code=abc');
    expect(within(claudeRow).getByTestId('claude-login-output').textContent).not.toContain('\u001b');

    const codeInput = within(claudeRow).getByLabelText('Claude login code');
    expect(codeInput.type).toBe('password');
    await fireEvent.input(codeInput, { target: { value: 'paste-me-123' } });
    await fireEvent.click(within(claudeRow).getByRole('button', { name: 'Send code' }));

    expect(await within(claudeRow).findByTestId('claude-login-success')).toBeTruthy();
    expect(calls.find((call) => call.url.endsWith('/code'))).toMatchObject({
      url: '/api/providers/claude/login/login-1/code',
      method: 'POST',
      body: { code: 'paste-me-123' },
    });
    await waitFor(() => expect(calls.some((call) => call.url === '/api/quota/refresh')).toBe(true));
    expect(calls.find((call) => call.url === '/api/quota/refresh')).toMatchObject({
      method: 'POST',
      body: { key: 'claude' },
    });
    await waitFor(() => expect(within(claudeRow).getByTestId('provider-status').textContent).toBe('Connected'));
  });

  it('connects Antigravity by link and keeps the callback panel open after bad input', async () => {
    const { calls, completeLogin } = stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');
    const row = container.querySelector('[data-provider="google-antigravity"]');
    await fireEvent.click(within(row).getByRole('button', { name: 'Connect' }));

    const panel = await screen.findByTestId('antigravity-connect-panel');
    const link = within(panel).getByRole('link', { name: 'Open Google sign-in' });
    expect(link.href).toBe('https://accounts.google.com/o/oauth2/v2/auth');
    const callback = within(panel).getByLabelText('Antigravity callback URL or code');
    await fireEvent.input(callback, { target: { value: 'bad callback' } });
    await fireEvent.click(within(panel).getByRole('button', { name: 'Submit callback' }));
    expect((await screen.findByRole('alert')).textContent).toContain('callback URL or code is invalid');
    expect(screen.getByTestId('antigravity-connect-panel')).toBeTruthy();
    expect(within(panel).getByLabelText('Antigravity callback URL or code').value).toBe('bad callback');

    await fireEvent.input(callback, { target: { value: 'valid-code' } });
    await fireEvent.click(within(panel).getByRole('button', { name: 'Submit callback' }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Submit callback' })).toBeNull());
    expect(screen.getByTestId('antigravity-connect-panel')).toBeTruthy();
    expect(screen.queryByRole('alert')).toBeNull();

    completeLogin();
    await waitFor(() => expect(within(row).getByTestId('provider-status').textContent).toBe('Connected'));
    await waitFor(() => expect(screen.queryByTestId('antigravity-connect-panel')).toBeNull());
    await waitFor(() => expect(calls.filter((call) => call.url === '/api/quota/refresh')).toHaveLength(2));
    expect(calls.filter((call) => call.url === '/api/quota/refresh').map((call) => call.body)).toEqual([
      { key: 'agy-gemini' },
      { key: 'agy-3p' },
    ]);
  });

  it('stops Antigravity polling when the login session expires', async () => {
    const { calls, expireLogin } = stubProviders();
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');
    const row = container.querySelector('[data-provider="google-antigravity"]');
    await fireEvent.click(within(row).getByRole('button', { name: 'Connect' }));
    await screen.findByTestId('antigravity-connect-panel');
    expireLogin();

    expect((await screen.findByRole('alert')).textContent).toContain('request failed: 404');
    await waitFor(() => expect(screen.queryByTestId('antigravity-connect-panel')).toBeNull());
    const getCount = () => calls.filter((call) =>
      call.method === 'GET' && call.url.startsWith('/api/providers/google-antigravity/connect/'),
    ).length;
    const countAfterExpiry = getCount();
    await new Promise((resolve) => setTimeout(resolve, 600));
    expect(getCount()).toBe(countAfterExpiry);
  });
  it('lets admins set and remove a key while only reading the set flag back', async () => {
    const { calls } = stubProviders();
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
    expect(within(cliRow).getByText('pig login xai')).toBeTruthy();
    expect(within(cliRow).getByRole('button', { name: 'Copy xAI command' })).toBeTruthy();
  });

  it('shows the ChatGPT device code and link and lets the admin cancel', async () => {
    const calls = [];
    vi.stubGlobal('fetch', vi.fn(async (input, options = {}) => {
      const url = String(input);
      const method = options.method ?? 'GET';
      calls.push({ url, method });
      if (url === '/api/providers') return { ok: true, json: async () => ({ providers: providerRows() }) };
      if (url === '/api/providers/openai-codex/device-code' && method === 'POST') {
        return { ok: true, json: async () => ({ login: { id: 'login-1', status: 'starting' } }) };
      }
      if (url.endsWith('/device-code/login-1') && method === 'GET') {
        return {
          ok: true,
          json: async () => ({
            login: {
              id: 'login-1',
              status: 'waiting',
              userCode: 'WXYZ-7',
              verificationUrl: 'https://auth.openai.com/codex/device',
              expiresAt: 1893456000000,
            },
          }),
        };
      }
      if (url.endsWith('/device-code/login-1') && method === 'DELETE') {
        return { ok: true, json: async () => ({ login: { id: 'login-1', status: 'canceled' } }) };
      }
      throw new Error(`unexpected request: ${method} ${url}`);
    }));
    const { container } = render(ProvidersPage, { props: { admin: true } });
    await screen.findAllByTestId('provider-row');
    const chatGPT = container.querySelector('[data-provider="openai-codex"]');
    await fireEvent.click(within(chatGPT).getByRole('button', { name: 'Reconnect ChatGPT' }));

    await screen.findByTestId('chatgpt-user-code');
    const panel = await screen.findByTestId('chatgpt-device-login');
    expect(panel.dataset.status).toBe('waiting');
    expect(within(panel).getByTestId('chatgpt-user-code').textContent).toBe('WXYZ-7');
    expect(within(panel).getByRole('link', { name: 'https://auth.openai.com/codex/device' }).getAttribute('href')).toBe('https://auth.openai.com/codex/device');
    await fireEvent.click(within(panel).getByRole('button', { name: 'Cancel login' }));
    expect(await screen.findByText('ChatGPT login canceled.')).toBeTruthy();
    expect(calls).toContainEqual({ url: '/api/providers/openai-codex/device-code', method: 'POST' });
    expect(calls).toContainEqual({ url: '/api/providers/openai-codex/device-code/login-1', method: 'DELETE' });
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
