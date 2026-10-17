<script>
  import { onDestroy, onMount, tick } from 'svelte';
  import { Check, Clock3, KeyRound, LockKeyhole, LogIn, Terminal, Trash2, X } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import Section from './ui/section.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import StatCard from './ui/stat-card.svelte';
  import QuotaWindows from './QuotaWindows.svelte';
  import Chip from './ui/chip.svelte';
  import PageHeader from './ui/page-header.svelte';
  import { icons, providerGlyph } from './icons.js';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import InfoHint from './ui/info-hint.svelte';
  import {
    cancelAntigravityLogin,
    cancelChatGPTDeviceCodeLogin,
    cancelClaudeLogin,
    getAntigravityLogin,
    getChatGPTDeviceCodeLogin,
    getClaudeLogin,
    getProviders,
    refreshQuota,
    removeProviderKey,
    sendClaudeLoginCode,
    setProviderKey,
    startAntigravityLogin as startAntigravityLoginRequest,
    startChatGPTDeviceCodeLogin,
    startClaudeLogin,
    submitAntigravityLogin,
  } from './api.js';
  import { quotaStore } from './quotaStore.js';
  import { quotaKeysForProvider, quotaState } from './quota.js';

  let { admin = false } = $props();
  let providers = $state([]);
  let loading = $state(true);
  let loadError = $state('');
  let editingID = $state('');
  let keyDrafts = $state({});
  let busy = $state({});
  let rowErrors = $state({});
  let copied = $state('');
  let copyTimer;
  let chatGPTLogin = $state(null);
  let chatGPTPollTimer;
  let chatGPTPollInFlight = false;
  let claudeLogin = $state(null);
  let loginCode = $state('');
  let loginBusy = $state(false);
  let loginError = $state('');
  let claudeLoginTimer;
  let refreshedClaudeLogin = '';
  let antigravityLogin = $state(null);
  let antigravityCallback = $state('');
  let antigravityBusy = $state(false);
  let connectPollTimer;
  let connectPollFailures = 0;
  let providerTargetFocused = false;

  // Percent first: each card reads its usage windows from the shared quota store.
  let quota = $state([]);
  let now = $state(Date.now());
  const GROUP_GLYPH = { subscription_logins: icons.provider, api_keys: KeyRound, cli_only: Terminal };
  const quotaEntries = (provider) => quotaKeysForProvider(provider.id).map((key) => quota.find((entry) => entry.key === key)).filter(Boolean);
  // One state per Provider: the worst state among its quota accounts.
  function quotaProviderState(entries) {
    const states = entries.map(quotaState);
    for (const state of ['expired', 'error', 'stale']) if (states.includes(state)) return state;
    return states.length && states.every((state) => state === 'not_connected') ? 'not_connected' : 'ok';
  }
  function quotaWindows(entries) {
    const multi = entries.length > 1;
    return entries.flatMap((entry) => (entry.windows ?? []).map((w) => ({ ...w, label: multi ? `${entry.label ?? entry.key} · ${w.label}` : w.label })));
  }
  const connectedCount = $derived(providers.filter((provider) => provider.connected).length);
  const loginCount = $derived(providers.filter((provider) => provider.connected && provider.group === 'subscription_logins').length);
  const keyCount = $derived(providers.filter((provider) => provider.keySet).length);

  const groups = [
    {
      id: 'subscription_logins',
      title: 'Subscription logins',
      hint: 'Connect ChatGPT or Antigravity here. Claude is managed by Claude Code, not pig; other subscription logins are read-only.',
      icon: LogIn,
    },
    {
      id: 'api_keys',
      title: 'API keys',
      hint: 'Set a key on the host Daemon. The saved value is never sent back to this page.',
      icon: KeyRound,
    },
    {
      id: 'cli_only',
      title: 'CLI-only',
      hint: 'Use the pig CLI on the host to configure these providers.',
      icon: Terminal,
    },
  ];
  function chatGPTLoginActive(login) {
    return login?.status === 'starting' || login?.status === 'waiting';
  }

  function stopChatGPTLoginPolling() {
    clearTimeout(chatGPTPollTimer);
    chatGPTPollTimer = undefined;
  }

  async function load() {
    try {
      const body = await getProviders();
      providers = Array.isArray(body?.providers) ? body.providers : [];
      loadError = '';
    } catch (error) {
      loadError = error?.message || 'Could not load providers.';
    } finally {
      loading = false;
    }
    await focusRequestedProvider();
  }

  async function focusRequestedProvider() {
    if (providerTargetFocused) return;
    providerTargetFocused = true;
    const target = window.location.hash.slice(1);
    if (!target.startsWith('provider-')) return;
    await tick();
    const row = document.getElementById(target);
    row?.scrollIntoView?.({ block: 'start' });
    row?.focus?.({ preventScroll: true });
  }


  async function refreshProviderQuota(providerId) {
    await Promise.all(quotaKeysForProvider(providerId).map(async (key) => {
      try {
        quotaStore.set(await refreshQuota(key));
      } catch {
        // A credential connection succeeds even if its usage endpoint is down.
      }
    }));
  }

  function beginEdit(provider) {
    if (!admin || !provider.keyEditable) return;
    editingID = provider.id;
    keyDrafts = { ...keyDrafts, [provider.id]: '' };
    rowErrors = { ...rowErrors, [provider.id]: '' };
  }

  function cancelEdit(provider) {
    editingID = '';
    keyDrafts = { ...keyDrafts, [provider.id]: '' };
    rowErrors = { ...rowErrors, [provider.id]: '' };
  }

  async function saveKey(event, provider) {
    event.preventDefault();
    const key = String(keyDrafts[provider.id] ?? '').trim();
    if (!key) {
      rowErrors = { ...rowErrors, [provider.id]: 'Enter an API key.' };
      return;
    }
    busy = { ...busy, [provider.id]: true };
    rowErrors = { ...rowErrors, [provider.id]: '' };
    try {
      await setProviderKey(provider.id, key);
      keyDrafts = { ...keyDrafts, [provider.id]: '' };
      editingID = '';
      await load();
      void refreshProviderQuota(provider.id);
    } catch (error) {
      rowErrors = { ...rowErrors, [provider.id]: error?.message || 'Could not save this key.' };
    } finally {
      busy = { ...busy, [provider.id]: false };
    }
  }

  async function removeKey(provider) {
    if (!admin || !provider.keyEditable || busy[provider.id]) return;
    busy = { ...busy, [provider.id]: true };
    rowErrors = { ...rowErrors, [provider.id]: '' };
    try {
      await removeProviderKey(provider.id);
      await load();
    } catch (error) {
      rowErrors = { ...rowErrors, [provider.id]: error?.message || 'Could not remove this key.' };
    } finally {
      busy = { ...busy, [provider.id]: false };
    }
  }

  async function pollAntigravityLogin(id) {
    if (antigravityLogin?.id !== id) return;
    let delay = 500;
    try {
      const next = await getAntigravityLogin(id);
      if (antigravityLogin?.id !== id) return;
      antigravityLogin = next;
      connectPollFailures = 0;
      rowErrors = { ...rowErrors, 'google-antigravity': '' };
      if (next.state === 'complete') {
        antigravityCallback = '';
        await load();
        void refreshProviderQuota('google-antigravity');
        if (antigravityLogin?.id === id) antigravityLogin = null;
        return;
      }
      if (next.state === 'error' || next.state === 'cancelled') return;
    } catch (error) {
      if (antigravityLogin?.id !== id) return;
      if (error?.status === 404) {
        rowErrors = { ...rowErrors, 'google-antigravity': 'Antigravity login expired. Start again.' };
        antigravityLogin = null;
        return;
      }
      rowErrors = { ...rowErrors, 'google-antigravity': error?.message || 'Could not check Antigravity login.' };
      connectPollFailures += 1;
      if (connectPollFailures >= 5) return;
      delay = Math.min(500 * 2 ** connectPollFailures, 5000);
    }
    if (antigravityLogin?.id === id) {
      connectPollTimer = setTimeout(() => void pollAntigravityLogin(id), delay);
    }
  }

  async function connectAntigravity() {
    if (!admin || antigravityBusy) return;
    const previous = antigravityLogin?.id;
    clearTimeout(connectPollTimer);
    connectPollFailures = 0;
    antigravityBusy = true;
    rowErrors = { ...rowErrors, 'google-antigravity': '' };
    try {
      if (previous) await cancelAntigravityLogin(previous).catch(() => {});
      antigravityCallback = '';
      antigravityLogin = await startAntigravityLoginRequest();
      void pollAntigravityLogin(antigravityLogin.id);
    } catch (error) {
      rowErrors = { ...rowErrors, 'google-antigravity': error?.message || 'Could not start Antigravity login.' };
    } finally {
      antigravityBusy = false;
    }
  }

  async function submitAntigravityCallback(event) {
    event.preventDefault();
    const id = antigravityLogin?.id;
    const value = String(antigravityCallback ?? '').trim();
    const callbackInput = Boolean(antigravityLogin?.url || antigravityLogin?.prompt?.toLowerCase().includes('callback'));
    if (!id || !value || antigravityBusy) return;
    antigravityBusy = true;
    rowErrors = { ...rowErrors, 'google-antigravity': '' };
    try {
      await submitAntigravityLogin(id, value);
      if (!callbackInput) antigravityCallback = '';
    } catch (error) {
      rowErrors = { ...rowErrors, 'google-antigravity': error?.message || 'Could not submit the Antigravity callback.' };
    } finally {
      antigravityBusy = false;
    }
  }

  async function closeAntigravityLogin() {
    const id = antigravityLogin?.id;
    clearTimeout(connectPollTimer);
    connectPollFailures = 0;
    antigravityLogin = null;
    antigravityCallback = '';
    if (id) await cancelAntigravityLogin(id).catch(() => {});
  }

  async function copyCommand(provider) {
    if (!provider.command || !globalThis.navigator?.clipboard?.writeText) return;
    try {
      await navigator.clipboard.writeText(provider.command);
      clearTimeout(copyTimer);
      copied = provider.id;
      copyTimer = setTimeout(() => {
        if (copied === provider.id) copied = '';
      }, 1500);
    } catch {
      copied = '';
    }
  }

  async function pollChatGPTLogin(id) {
    if (chatGPTPollInFlight || chatGPTLogin?.id !== id) return;
    chatGPTPollInFlight = true;
    try {
      const body = await getChatGPTDeviceCodeLogin(id);
      if (chatGPTLogin?.id !== id || !body?.login) return;
      chatGPTLogin = body.login;
      if (chatGPTLogin.status === 'complete') {
        stopChatGPTLoginPolling();
        await load();
      } else if (!chatGPTLoginActive(chatGPTLogin)) {
        stopChatGPTLoginPolling();
      }
    } catch (error) {
      rowErrors = { ...rowErrors, 'openai-codex': error?.message || 'Could not check ChatGPT login.' };
    } finally {
      chatGPTPollInFlight = false;
      scheduleChatGPTLoginPoll(id);
    }
  }

  function scheduleChatGPTLoginPoll(id) {
    stopChatGPTLoginPolling();
    if (chatGPTLogin?.id === id && chatGPTLoginActive(chatGPTLogin)) {
      chatGPTPollTimer = setTimeout(() => void pollChatGPTLogin(id), 1000);
    }
  }

  async function startChatGPTLogin() {
    if (!admin || busy['openai-codex']) return;
    stopChatGPTLoginPolling();
    chatGPTLogin = null;
    busy = { ...busy, 'openai-codex': true };
    rowErrors = { ...rowErrors, 'openai-codex': '' };
    try {
      const body = await startChatGPTDeviceCodeLogin();
      chatGPTLogin = body?.login ?? null;
      if (chatGPTLogin?.id) void pollChatGPTLogin(chatGPTLogin.id);
    } catch (error) {
      rowErrors = { ...rowErrors, 'openai-codex': error?.message || 'Could not start ChatGPT login.' };
    } finally {
      busy = { ...busy, 'openai-codex': false };
    }
  }

  async function cancelChatGPTLogin() {
    if (!admin || !chatGPTLoginActive(chatGPTLogin)) return;
    const id = chatGPTLogin.id;
    stopChatGPTLoginPolling();
    busy = { ...busy, 'openai-codex': true };
    try {
      const body = await cancelChatGPTDeviceCodeLogin(id);
      if (chatGPTLogin?.id === id && body?.login) chatGPTLogin = body.login;
    } catch (error) {
      rowErrors = { ...rowErrors, 'openai-codex': error?.message || 'Could not cancel ChatGPT login.' };
      scheduleChatGPTLoginPoll(id);
    } finally {
      busy = { ...busy, 'openai-codex': false };
    }
  }

  onDestroy(() => {
    clearTimeout(copyTimer);
    clearTimeout(claudeLoginTimer);
    stopChatGPTLoginPolling();
    clearTimeout(connectPollTimer);
    if (admin && chatGPTLoginActive(chatGPTLogin)) {
      void cancelChatGPTDeviceCodeLogin(chatGPTLogin.id).catch(() => {});
    }
    if (admin && claudeLogin?.state === 'running') {
      void cancelClaudeLogin(claudeLogin.id).catch(() => {});
    }
    if (admin && antigravityLogin?.id && !['complete', 'error', 'cancelled'].includes(antigravityLogin.state)) {
      void cancelAntigravityLogin(antigravityLogin.id).catch(() => {});
    }
  });

  function cleanClaudeOutput(output) {
    return String(output ?? '').replace(/\u001b(?:\[[0-?]*[ -/]*[@-~]|\][^\u0007]*(?:\u0007|\u001b\\))/g, '');
  }

  function claudeLoginURL(output) {
    const candidate = cleanClaudeOutput(output).match(/https:\/\/[^\s"'<>]+/)?.[0]?.replace(/[),.;]+$/, '');
    if (!candidate) return '';
    try {
      return new URL(candidate).protocol === 'https:' ? candidate : '';
    } catch {
      return '';
    }
  }

  function scheduleClaudeLoginPoll() {
    clearTimeout(claudeLoginTimer);
    claudeLoginTimer = setTimeout(() => void pollClaudeLogin(), 500);
  }

  function finishClaudeLogin(session) {
    if (session?.state !== 'succeeded' || refreshedClaudeLogin === session.id) return;
    refreshedClaudeLogin = session.id;
    loginCode = '';
    void load();
    void refreshProviderQuota('claude');
  }

  async function pollClaudeLogin() {
    const active = claudeLogin;
    if (!active?.id || active.state !== 'running') return;
    try {
      const latest = await getClaudeLogin(active.id);
      if (claudeLogin?.id !== latest.id) return;
      claudeLogin = latest;
      if (latest.state === 'running') scheduleClaudeLoginPoll();
      else finishClaudeLogin(latest);
    } catch (error) {
      if (claudeLogin?.id === active.id) {
        claudeLogin = { ...active, state: 'failed', error: error?.message || 'Could not read Claude login status.' };
      }
    }
  }

  async function startClaudeLoginFlow(provider) {
    if (!admin || loginBusy || claudeLogin?.state === 'running') return;
    loginBusy = true;
    loginError = '';
    rowErrors = { ...rowErrors, [provider.id]: '' };
    try {
      claudeLogin = await startClaudeLogin();
      loginCode = '';
      if (claudeLogin.state === 'running') scheduleClaudeLoginPoll();
      else finishClaudeLogin(claudeLogin);
    } catch (error) {
      rowErrors = { ...rowErrors, [provider.id]: error?.message || 'Could not start Claude login.' };
    } finally {
      loginBusy = false;
    }
  }

  async function submitClaudeLoginCode(event) {
    event.preventDefault();
    const code = String(loginCode ?? '').trim();
    if (!code) {
      loginError = 'Paste the code from Claude.';
      return;
    }
    loginBusy = true;
    loginError = '';
    try {
      await sendClaudeLoginCode(claudeLogin.id, code);
      loginCode = '';
      await pollClaudeLogin();
    } catch (error) {
      loginError = error?.message || 'Could not send the Claude login code.';
    } finally {
      loginBusy = false;
    }
  }

  async function cancelClaudeLoginFlow() {
    if (!claudeLogin?.id || loginBusy) return;
    clearTimeout(claudeLoginTimer);
    loginBusy = true;
    loginError = '';
    try {
      await cancelClaudeLogin(claudeLogin.id);
      claudeLogin = { ...claudeLogin, state: 'canceled', error: 'Claude login canceled.' };
      loginCode = '';
    } catch (error) {
      loginError = error?.message || 'Could not cancel Claude login.';
    } finally {
      loginBusy = false;
    }
  }

  onMount(() => {
    void load();
    const stopQuota = quotaStore.subscribe((value) => {
      quota = value;
      now = Date.now();
    });
    void quotaStore.load().catch(() => {});
    return stopQuota;
  });
</script>

{#snippet adminOnly()}
  <Chip tone="outline" icon={LockKeyhole} class="max-md:min-h-11 max-md:justify-center">Admin only</Chip>
{/snippet}

{#snippet card(provider)}
  {@const quotaKey = quotaKeysForProvider(provider.id)[0]}
  {@const Glyph = quotaKey ? providerGlyph(quotaKey) : GROUP_GLYPH[provider.group] ?? icons.provider}
  {@const entries = quotaEntries(provider)}
  {@const qState = quotaProviderState(entries)}
  {@const windows = provider.connected && qState !== 'not_connected' ? quotaWindows(entries) : []}
  <article
    id={`provider-${provider.id}`}
    tabindex="-1"
    class="animate-rise flex min-w-0 flex-col gap-4 rounded-2xl border bg-card p-4 shadow-elev-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40"
    data-testid="provider-row"
    data-provider={provider.id}
    data-source={provider.source}
  >
    <div class="flex items-center gap-3">
      <span class="grid size-10 shrink-0 place-items-center rounded-xl bg-muted text-fg-2" aria-hidden="true"><Glyph class="size-5" /></span>
      <div class="min-w-0 flex-1">
        <h3 class="truncate text-title">{provider.name}</h3>
        <p class="truncate text-caption text-fg-3" data-testid="provider-source">
          {provider.sourceLabel || 'Not connected'}
          {#if provider.keySet}<span class="ml-1" data-testid="provider-key-set">· API key set</span>{/if}
        </p>
      </div>
    </div>

    <div class="-mt-1 flex flex-wrap items-center gap-1.5">
      <Chip tone={provider.connected ? 'done' : 'paused'} dot data-testid="provider-status">{provider.connected ? 'Connected' : 'Not connected'}</Chip>
      {#if provider.connected && qState === 'stale'}
        <Chip tone="needs-you" icon={Clock3}>stale</Chip>
      {:else if provider.connected && qState === 'expired'}
        <Chip tone="needs-you" icon={LockKeyhole}>login expired</Chip>
      {:else if provider.connected && qState === 'error'}
        <Chip tone="failed" dot>quota error</Chip>
      {/if}
    </div>

    {#if windows.length}
      <QuotaWindows {windows} name={provider.name} {now} dimmed={qState === 'expired'} />
    {/if}

    {#if provider.id === 'claude'}
      <div class="flex flex-wrap items-center gap-2 max-md:[&>*]:flex-1">
        {#if admin}
          <Button variant="outline" size="sm" disabled={loginBusy || claudeLogin?.state === 'running'} aria-busy={loginBusy} onclick={() => startClaudeLoginFlow(provider)}>
            <Terminal />{claudeLogin?.state === 'running' ? 'Login running' : provider.connected ? 'Reconnect' : 'Connect'}
          </Button>
        {:else}
          {@render adminOnly()}
        {/if}
      </div>
    {:else if provider.keyEditable}
      <div class="flex flex-wrap items-center gap-2 max-md:[&>*]:flex-1">
        {#if admin}
          <Button variant="outline" size="sm" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])} onclick={() => beginEdit(provider)}>
            <KeyRound />{provider.keySet ? 'Replace key' : 'Add key'}
          </Button>
          {#if provider.keySet}
            <Button variant="ghost" size="sm" class="text-status-failed" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])} onclick={() => removeKey(provider)}>
              <Trash2 />Remove
            </Button>
          {/if}
        {:else}
          {@render adminOnly()}
        {/if}
      </div>
    {:else if provider.id === 'openai-codex'}
      <div class="flex items-center gap-2 max-md:[&>*]:flex-1">
        {#if admin}
          <Button variant="outline" size="sm" disabled={busy['openai-codex']} aria-busy={Boolean(busy['openai-codex'])} onclick={startChatGPTLogin}>
            <Check />{provider.connected ? 'Reconnect ChatGPT' : 'Connect ChatGPT'}
          </Button>
        {:else}
          {@render adminOnly()}
        {/if}
      </div>
    {:else if provider.connectable}
      <div class="flex flex-wrap items-center gap-2 max-md:[&>*]:flex-1">
        {#if admin}
          <Button variant="outline" size="sm" disabled={antigravityBusy} aria-busy={antigravityBusy} onclick={connectAntigravity}>
            <Check />{antigravityLogin?.state === 'error' ? 'Try again' : antigravityLogin ? 'Restart login' : 'Connect'}
          </Button>
        {:else}
          {@render adminOnly()}
        {/if}
      </div>
    {:else if !admin && !provider.command && !provider.cliHint}
      {@render adminOnly()}
    {/if}

    {#if provider.command}
      <div class="flex min-w-0 items-center gap-2">
        <code class="min-w-0 flex-1 truncate rounded-lg bg-muted px-3 py-2 font-mono text-caption text-fg-2">{provider.command}</code>
        <IconButton icon={copied === provider.id ? Check : icons.copy} label={`Copy ${provider.name} command`} onclick={() => copyCommand(provider)} />
      </div>
    {:else if provider.cliHint}
      <div class="flex items-center gap-1 text-caption text-fg-3" data-testid="provider-cli-guidance">
        <Terminal class="size-3.5 shrink-0" aria-hidden="true" />
        <span>Set up with the CLI</span>
        <InfoHint label={`How to set up ${provider.name}`}>{provider.cliHint}</InfoHint>
      </div>
    {/if}

    {#if provider.id === 'claude' && admin && claudeLogin?.id}
      {@const authorizationURL = claudeLoginURL(claudeLogin.output)}
      <div class="grid gap-3 rounded-xl border bg-surface-0 p-3" data-testid="claude-login-panel" aria-live="polite">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="text-body" data-testid="claude-login-state">Claude login {claudeLogin.state}</p>
          {#if authorizationURL}
            <a class="text-body text-primary underline underline-offset-4 max-md:flex max-md:min-h-11 max-md:items-center" href={authorizationURL} target="_blank" rel="noopener noreferrer">Open Claude sign-in page</a>
          {/if}
        </div>
        <pre class="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted p-3 font-mono text-caption" data-testid="claude-login-output">{cleanClaudeOutput(claudeLogin.output) || 'Waiting for Claude Code…'}</pre>
        {#if claudeLogin.error}<p role="alert" class="text-caption text-status-failed">{claudeLogin.error}</p>{/if}
        {#if loginError}<p role="alert" class="text-caption text-status-failed">{loginError}</p>{/if}
        {#if claudeLogin.state === 'running'}
          <form class="grid gap-3" data-testid="claude-login-form" onsubmit={submitClaudeLoginCode}>
            <Input
              name="claude-login-code"
              type="password"
              autocomplete="off"
              spellcheck="false"
              placeholder="Paste the code from Claude"
              aria-label="Claude login code"
              bind:value={loginCode}
            />
            <div class="flex flex-wrap gap-2 max-md:[&>*]:flex-1">
              <Button type="submit" disabled={loginBusy} aria-busy={loginBusy}><Check />Send code</Button>
              <Button variant="ghost" type="button" onclick={cancelClaudeLoginFlow} disabled={loginBusy}><X />Cancel login</Button>
            </div>
          </form>
        {:else if claudeLogin.state === 'succeeded'}
          <p class="text-body text-fg-2" data-testid="claude-login-success">Claude is connected. Quota refreshed.</p>
        {/if}
      </div>
    {/if}

    {#if provider.id === 'openai-codex' && chatGPTLogin}
      <div class="grid gap-3 rounded-xl border bg-surface-0 p-3" data-testid="chatgpt-device-login" data-status={chatGPTLogin.status}>
        {#if chatGPTLogin.status === 'starting'}
          <p class="text-body text-fg-2" role="status">Starting ChatGPT device-code login…</p>
        {:else if chatGPTLogin.status === 'waiting'}
          <p class="text-body">Open the verification page on any device, then enter this code:</p>
          <a class="w-fit break-all text-body text-primary underline underline-offset-4 max-md:flex max-md:min-h-11 max-md:items-center" href={chatGPTLogin.verificationUrl} target="_blank" rel="noopener noreferrer">{chatGPTLogin.verificationUrl}</a>
          <code class="w-fit rounded-lg bg-muted px-3 py-2 font-mono text-title tracking-wider" data-testid="chatgpt-user-code">{chatGPTLogin.userCode}</code>
          <div class="flex flex-wrap items-center gap-2 max-md:[&>*]:flex-1">
            <p class="flex-1 text-caption text-fg-3" role="status">Waiting for approval…</p>
            <Button variant="ghost" disabled={busy['openai-codex']} onclick={cancelChatGPTLogin}><X />Cancel login</Button>
          </div>
        {:else if chatGPTLogin.status === 'complete'}
          <p class="text-body text-fg-2" role="status">ChatGPT connected. The ChatGPT Quota card is refreshing.</p>
        {:else if chatGPTLogin.status === 'expired'}
          <p class="text-body text-fg-2" role="status">The device code expired. Start a new login to connect ChatGPT.</p>
        {:else if chatGPTLogin.status === 'canceled'}
          <p class="text-body text-fg-2" role="status">ChatGPT login canceled.</p>
        {:else}
          <p class="text-body text-status-failed" role="alert">{chatGPTLogin.error || 'ChatGPT login failed. Try again.'}</p>
        {/if}
      </div>
    {/if}

    {#if editingID === provider.id && admin}
      <form class="grid gap-3 rounded-xl border bg-surface-0 p-3" data-testid="provider-key-form" onsubmit={(event) => saveKey(event, provider)}>
        <Input
          name={`provider-key-${provider.id}`}
          type="password"
          autocomplete="new-password"
          spellcheck="false"
          placeholder="Paste API key"
          aria-label={`API key for ${provider.name}`}
          bind:value={keyDrafts[provider.id]}
        />
        <p class="text-caption text-fg-3">Saved to pig <code class="font-mono">auth.json</code> on the host. Only the set status is returned; the key is never shown again.</p>
        {#if rowErrors[provider.id]}<p role="alert" class="text-caption text-status-failed">{rowErrors[provider.id]}</p>{/if}
        <div class="flex flex-wrap gap-2 max-md:[&>*]:flex-1">
          <Button type="submit" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])}><KeyRound />Save key</Button>
          <Button variant="ghost" onclick={() => cancelEdit(provider)} disabled={busy[provider.id]}><X />Cancel</Button>
        </div>
      </form>
    {:else if provider.connectable && admin && antigravityLogin}
      <form class="grid gap-3 rounded-xl border bg-surface-0 p-3" data-testid="antigravity-connect-panel" onsubmit={submitAntigravityCallback}>
        {#if antigravityLogin.url}
          <a class="w-fit text-body underline underline-offset-4 max-md:flex max-md:min-h-11 max-md:items-center" href={antigravityLogin.url} target="_blank" rel="noopener noreferrer">Open Google sign-in</a>
        {/if}
        {#if antigravityLogin.instructions}<p class="text-caption text-fg-3">{antigravityLogin.instructions}</p>{/if}
        {#if antigravityLogin.progress}<p class="text-caption text-fg-3" role="status">{antigravityLogin.progress}</p>{/if}
        {#if antigravityLogin.prompt}<p class="text-caption text-fg-3">{antigravityLogin.prompt}</p>{/if}
        {#if antigravityLogin.url || antigravityLogin.prompt}
          <Input
            name="antigravity-callback"
            type={antigravityLogin.prompt?.toLowerCase().includes('client secret') ? 'password' : 'text'}
            autocomplete={antigravityLogin.prompt?.toLowerCase().includes('client secret') ? 'new-password' : 'off'}
            spellcheck="false"
            placeholder={antigravityLogin.url ? 'Paste callback URL or code' : 'Enter the requested value'}
            aria-label={antigravityLogin.url ? 'Antigravity callback URL or code' : antigravityLogin.prompt}
            bind:value={antigravityCallback}
          />
        {/if}
        {#if antigravityLogin.error}<p role="alert" class="text-caption text-status-failed">{antigravityLogin.error}</p>{/if}
        {#if rowErrors[provider.id]}<p role="alert" class="text-caption text-status-failed">{rowErrors[provider.id]}</p>{/if}
        <div class="flex flex-wrap gap-2 max-md:[&>*]:flex-1">
          {#if antigravityLogin.prompt && !['complete', 'error', 'cancelled'].includes(antigravityLogin.state)}
            <Button type="submit" disabled={antigravityBusy || !antigravityCallback.trim()} aria-busy={antigravityBusy}>{antigravityLogin.url ? 'Submit callback' : 'Continue'}</Button>
          {/if}
          <Button variant="ghost" onclick={closeAntigravityLogin} disabled={antigravityBusy}><X />Cancel</Button>
        </div>
      </form>
    {:else if rowErrors[provider.id]}
      <p role="alert" class="text-caption text-status-failed">{rowErrors[provider.id]}</p>
    {/if}
  </article>
{/snippet}

<div class="flex min-h-0 flex-1 flex-col" data-testid="providers-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-[max(4rem,env(safe-area-inset-bottom))] md:px-6">
    <div class="mx-auto flex w-full max-w-4xl flex-col gap-6">
      <PageHeader title="Providers" icon={icons.provider} scope="Global" class="animate-rise">
        {#snippet meta()}{#if admin}<Chip tone="outline" class="max-sm:hidden">Admin managed</Chip>{/if}{/snippet}
        {#snippet hint()}Provider status for this host. Pig API keys are stored in <code class="font-mono">auth.json</code>; Claude Code keeps its login in the host <code class="font-mono">~/.claude</code>.{/snippet}
      </PageHeader>

      {#if !admin}
        <div class="flex items-start gap-2 rounded-xl bg-muted p-3 text-caption text-fg-3" data-testid="providers-read-only">
          <LockKeyhole class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          <span>Provider status is read-only. Ask an admin to connect or change credentials.</span>
        </div>
      {/if}

      {#if loadError}
        <p role="alert" class="rounded-xl bg-status-failed/10 px-4 py-2 text-body text-status-failed">{loadError}</p>
      {/if}

      {#if loading}
        <div role="status" aria-label="Loading providers" class="@container flex flex-col gap-6" data-testid="providers-loading">
          <div class="grid grid-cols-3 gap-2 md:gap-4">
            {#each [0, 1, 2] as i (i)}<Skeleton class="h-24 rounded-2xl" />{/each}
          </div>
          <div class="grid gap-4 @2xl:grid-cols-2">
            {#each [0, 1, 2, 3] as i (i)}<Skeleton class="h-36 rounded-2xl" />{/each}
          </div>
        </div>
      {:else}
        {#if providers.length}
          <div class="grid grid-cols-3 gap-2 md:gap-4" data-testid="providers-summary">
            <StatCard label="Connected" value={connectedCount} unit={`/${providers.length}`} caption="providers" icon={icons.provider} />
            <StatCard label="Logins" value={loginCount} caption="subscriptions" icon={Check} />
            <StatCard label="Keys" value={keyCount} caption="API keys set" icon={KeyRound} />
          </div>
        {/if}
        {#each groups as group (group.id)}
          {@const rows = providers.filter((provider) => provider.group === group.id)}
          {#if rows.length}
            <Section title={group.title} icon={group.icon} hint={group.hint} headingLevel={2} class="animate-rise" data-testid="provider-group" data-group={group.id}>
              <div class="@container">
                <div class="grid gap-4 @2xl:grid-cols-2">
                  {#each rows as provider (provider.id)}
                    {@render card(provider)}
                  {/each}
                </div>
              </div>
            </Section>
          {/if}
        {/each}
      {/if}
    </div>
  </div>
</div>
