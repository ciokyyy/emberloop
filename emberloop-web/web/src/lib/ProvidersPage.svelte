<script>
  import { onDestroy, onMount } from 'svelte';
  import { Check, Copy, KeyRound, LockKeyhole, Terminal, Trash2, X } from '@lucide/svelte';
  import Badge from './ui/badge.svelte';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import {
    cancelChatGPTDeviceCodeLogin,
    cancelClaudeLogin,
    getChatGPTDeviceCodeLogin,
    getClaudeLogin,
    getProviders,
    refreshQuota,
    removeProviderKey,
    sendClaudeLoginCode,
    setProviderKey,
    startChatGPTDeviceCodeLogin,
    startClaudeLogin,
  } from './api.js';

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

  const groups = [
    {
      id: 'subscription_logins',
      title: 'Subscription logins',
      hint: 'Pig subscription logins are listed here. Claude is managed by Claude Code, not pig.',
      icon: Check,
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
    if (admin && chatGPTLoginActive(chatGPTLogin)) {
      void cancelChatGPTDeviceCodeLogin(chatGPTLogin.id).catch(() => {});
    }
    if (admin && claudeLogin?.state === 'running') {
      void cancelClaudeLogin(claudeLogin.id).catch(() => {});
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
    void refreshQuota('claude').catch(() => {});
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
  });
</script>

<div class="flex min-h-0 flex-1 flex-col" data-testid="providers-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-16">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <header class="animate-rise flex flex-col gap-2">
        <div class="flex flex-wrap items-center gap-2.5">
          <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-card" aria-hidden="true"><KeyRound class="size-4" /></span>
          <h1 class="text-3xl font-semibold tracking-tight max-md:text-2xl">Providers</h1>
          <Badge variant="outline">Global</Badge>
          {#if admin}<Badge variant="outline">Admin managed</Badge>{/if}
        </div>
        <p class="text-sm text-muted-foreground">Provider status for this host. Pig API keys are stored in <code class="font-mono">auth.json</code>; Claude Code keeps its login in the host <code class="font-mono">~/.claude</code>.</p>
      </header>

      {#if !admin}
        <div class="flex items-start gap-2 rounded-xl bg-muted p-3 text-sm text-muted-foreground" data-testid="providers-read-only">
          <LockKeyhole class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <span>Provider status is read-only. Ask an admin to connect or change credentials.</span>
        </div>
      {/if}

      {#if loadError}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{loadError}</p>
      {/if}

      {#if loading}
        <div class="grid gap-4" aria-label="Loading providers" data-testid="providers-loading">
          {#each [0, 1, 2, 3, 4] as i (i)}
            <div class="h-24 animate-pulse rounded-2xl border bg-card"></div>
          {/each}
        </div>
      {:else}
        {#each groups as group (group.id)}
          {@const GroupIcon = group.icon}
          {@const rows = providers.filter((provider) => provider.group === group.id)}
          {#if rows.length}
            <section class="animate-rise flex flex-col gap-2" aria-labelledby={`providers-${group.id}`} data-testid="provider-group" data-group={group.id}>
              <div class="flex items-start gap-2 px-1">
                <GroupIcon class="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <div class="min-w-0">
                  <h2 id={`providers-${group.id}`} class="text-sm font-semibold">{group.title}</h2>
                  <p class="text-xs text-muted-foreground">{group.hint}</p>
                </div>
              </div>
              <div class="divide-y divide-border/60 rounded-2xl border bg-card">
                {#each rows as provider (provider.id)}
                  <article class="p-4" data-testid="provider-row" data-provider={provider.id} data-source={provider.source}>
                    <div class="flex flex-col gap-3 sm:flex-row sm:items-center">
                      <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-muted" aria-hidden="true">
                        {#if provider.group === 'api_keys'}<KeyRound class="size-4" />{:else if provider.group === 'cli_only'}<Terminal class="size-4" />{:else}<Check class="size-4" />{/if}
                      </span>
                      <div class="min-w-0 flex-1 leading-tight">
                        <div class="flex flex-wrap items-center gap-2">
                          <h3 class="text-sm font-medium">{provider.name}</h3>
                          <Badge variant={provider.connected ? 'success' : 'secondary'} data-testid="provider-status">
                            {provider.connected ? 'Connected' : 'Not connected'}
                          </Badge>
                        </div>
                        <p class="mt-1 text-xs text-muted-foreground" data-testid="provider-source">
                          {provider.sourceLabel || 'Not connected'}
                          {#if provider.keySet}<span class="ml-1" data-testid="provider-key-set">· API key set</span>{/if}
                        </p>
                      </div>
                      {#if provider.id === 'claude'}
                        <div class="flex flex-wrap items-center gap-2 sm:ml-auto max-md:w-full max-md:[&>*]:flex-1">
                          {#if admin}
                            <Button variant="outline" size="sm" disabled={loginBusy || claudeLogin?.state === 'running'} aria-busy={loginBusy} onclick={() => startClaudeLoginFlow(provider)}>
                              <Terminal />{claudeLogin?.state === 'running' ? 'Login running' : provider.connected ? 'Reconnect' : 'Connect'}
                            </Button>
                          {:else}
                            <Badge variant="outline" class="max-md:min-h-11 max-md:justify-center"><LockKeyhole class="size-3.5" />Admin only</Badge>
                          {/if}
                        </div>
                      {:else if provider.keyEditable}
                        <div class="flex flex-wrap items-center gap-2 sm:ml-auto max-md:w-full max-md:[&>*]:flex-1">
                          {#if admin}
                            <Button variant="outline" size="sm" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])} onclick={() => beginEdit(provider)}>
                              <KeyRound />{provider.keySet ? 'Replace key' : 'Add key'}
                            </Button>
                            {#if provider.keySet}
                              <Button variant="ghost" size="sm" class="text-destructive" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])} onclick={() => removeKey(provider)}>
                                <Trash2 />Remove
                              </Button>
                            {/if}
                          {:else}
                            <Badge variant="outline" class="max-md:min-h-11 max-md:justify-center"><LockKeyhole class="size-3.5" />Admin only</Badge>
                          {/if}
                        </div>
                      {:else if provider.id === 'openai-codex'}
                        <div class="flex items-center gap-2 sm:ml-auto max-md:w-full">
                          {#if admin}
                            <Button variant="outline" size="sm" disabled={busy['openai-codex']} aria-busy={Boolean(busy['openai-codex'])} onclick={startChatGPTLogin}>
                              <Check />{provider.connected ? 'Reconnect ChatGPT' : 'Connect ChatGPT'}
                            </Button>
                          {:else}
                            <Badge variant="outline" class="max-md:min-h-11 max-md:justify-center"><LockKeyhole class="size-3.5" />Admin only</Badge>
                          {/if}
                        </div>
                      {:else if !admin && !provider.command && !provider.cliHint}
                        <Badge variant="outline" class="max-md:min-h-11 max-md:justify-center"><LockKeyhole class="size-3.5" />Admin only</Badge>
                      {/if}
                      {#if provider.command}
                        <div class="flex min-w-0 items-center gap-2 sm:ml-auto max-md:w-full">
                          <code class="min-w-0 flex-1 truncate rounded-lg bg-muted px-2.5 py-2 font-mono text-xs text-muted-foreground">{provider.command}</code>
                          <Button variant="ghost" size="icon-sm" class="shrink-0" aria-label={`Copy ${provider.name} command`} title="Copy command" onclick={() => copyCommand(provider)}>
                            {#if copied === provider.id}<Check />{:else}<Copy />{/if}
                          </Button>
                        </div>
                      {:else if provider.cliHint}
                        <p class="text-xs text-muted-foreground sm:ml-auto" data-testid="provider-cli-guidance">{provider.cliHint}</p>
                      {/if}
                    </div>

                    {#if provider.id === 'claude' && admin && claudeLogin?.id}
                      {@const authorizationURL = claudeLoginURL(claudeLogin.output)}
                      <div class="mt-3 grid gap-3 rounded-xl border bg-background p-3" data-testid="claude-login-panel" aria-live="polite">
                        <div class="flex flex-wrap items-center justify-between gap-2">
                          <p class="text-sm font-medium" data-testid="claude-login-state">Claude login {claudeLogin.state}</p>
                          {#if authorizationURL}
                            <a class="text-sm text-primary underline underline-offset-4" href={authorizationURL} target="_blank" rel="noopener noreferrer">Open Claude sign-in page</a>
                          {/if}
                        </div>
                        <pre class="max-h-48 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted p-3 font-mono text-xs" data-testid="claude-login-output">{cleanClaudeOutput(claudeLogin.output) || 'Waiting for Claude Code…'}</pre>
                        {#if claudeLogin.error}<p role="alert" class="text-xs text-destructive">{claudeLogin.error}</p>{/if}
                        {#if loginError}<p role="alert" class="text-xs text-destructive">{loginError}</p>{/if}
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
                          <p class="text-sm text-muted-foreground" data-testid="claude-login-success">Claude is connected. Quota refreshed.</p>
                        {/if}
                      </div>
                    {/if}

                    {#if provider.id === 'openai-codex' && chatGPTLogin}
                      <div class="mt-3 grid gap-3 rounded-xl border bg-background p-3" data-testid="chatgpt-device-login" data-status={chatGPTLogin.status}>
                        {#if chatGPTLogin.status === 'starting'}
                          <p class="text-sm text-muted-foreground" role="status">Starting ChatGPT device-code login…</p>
                        {:else if chatGPTLogin.status === 'waiting'}
                          <p class="text-sm">Open the verification page on any device, then enter this code:</p>
                          <a class="w-fit text-sm text-primary underline underline-offset-4" href={chatGPTLogin.verificationUrl} target="_blank" rel="noopener noreferrer">{chatGPTLogin.verificationUrl}</a>
                          <code class="w-fit rounded-lg bg-muted px-3 py-2 font-mono text-lg font-semibold tracking-wider" data-testid="chatgpt-user-code">{chatGPTLogin.userCode}</code>
                          <div class="flex flex-wrap items-center gap-2 max-md:[&>*]:flex-1">
                            <p class="flex-1 text-xs text-muted-foreground" role="status">Waiting for approval…</p>
                            <Button variant="ghost" disabled={busy['openai-codex']} onclick={cancelChatGPTLogin}><X />Cancel login</Button>
                          </div>
                        {:else if chatGPTLogin.status === 'complete'}
                          <p class="text-sm text-muted-foreground" role="status">ChatGPT connected. The ChatGPT Quota card is refreshing.</p>
                        {:else if chatGPTLogin.status === 'expired'}
                          <p class="text-sm text-muted-foreground" role="status">The device code expired. Start a new login to connect ChatGPT.</p>
                        {:else if chatGPTLogin.status === 'canceled'}
                          <p class="text-sm text-muted-foreground" role="status">ChatGPT login canceled.</p>
                        {:else}
                          <p class="text-sm text-destructive" role="alert">{chatGPTLogin.error || 'ChatGPT login failed. Try again.'}</p>
                        {/if}
                      </div>
                    {/if}

                    {#if editingID === provider.id && admin}
                      <form class="mt-3 grid gap-3 rounded-xl border bg-background p-3" data-testid="provider-key-form" onsubmit={(event) => saveKey(event, provider)}>
                        <Input
                          name={`provider-key-${provider.id}`}
                          type="password"
                          autocomplete="new-password"
                          spellcheck="false"
                          placeholder="Paste API key"
                          aria-label={`API key for ${provider.name}`}
                          bind:value={keyDrafts[provider.id]}
                        />
                        <p class="text-xs text-muted-foreground">Saved to pig <code class="font-mono">auth.json</code> on the host. Only the set status is returned; the key is never shown again.</p>
                        {#if rowErrors[provider.id]}<p role="alert" class="text-xs text-destructive">{rowErrors[provider.id]}</p>{/if}
                        <div class="flex flex-wrap gap-2 max-md:[&>*]:flex-1">
                          <Button type="submit" disabled={busy[provider.id]} aria-busy={Boolean(busy[provider.id])}><KeyRound />Save key</Button>
                          <Button variant="ghost" onclick={() => cancelEdit(provider)} disabled={busy[provider.id]}><X />Cancel</Button>
                        </div>
                      </form>
                    {:else if rowErrors[provider.id]}
                      <p role="alert" class="mt-2 text-xs text-destructive">{rowErrors[provider.id]}</p>
                    {/if}
                  </article>
                {/each}
              </div>
            </section>
          {/if}
        {/each}
      {/if}
    </div>
  </div>
</div>
