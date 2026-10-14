<script>
  import { onDestroy, onMount } from 'svelte';
  import { Check, Copy, KeyRound, LockKeyhole, Terminal, Trash2, X } from '@lucide/svelte';
  import Badge from './ui/badge.svelte';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import { getProviders, removeProviderKey, setProviderKey } from './api.js';

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

  const groups = [
    {
      id: 'subscription_logins',
      title: 'Subscription logins',
      hint: 'Pig OAuth and subscription providers. Their connection status is read-only here.',
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
  onDestroy(() => clearTimeout(copyTimer));

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
        <p class="text-sm text-muted-foreground">Pig provider status for this host. API keys are stored in pig <code class="font-mono">auth.json</code>; the key itself is never shown after saving.</p>
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
                      {#if provider.keyEditable}
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
                      {:else if provider.command && admin}
                        <div class="flex min-w-0 items-center gap-2 sm:ml-auto max-md:w-full">
                          <code class="min-w-0 flex-1 truncate rounded-lg bg-muted px-2.5 py-2 font-mono text-xs text-muted-foreground">{provider.command}</code>
                          <Button variant="ghost" size="icon-sm" class="shrink-0" aria-label={`Copy ${provider.name} command`} title="Copy command" onclick={() => copyCommand(provider)}>
                            {#if copied === provider.id}<Check />{:else}<Copy />{/if}
                          </Button>
                        </div>
                      {:else if !admin}
                        <Badge variant="outline" class="max-md:min-h-11 max-md:justify-center"><LockKeyhole class="size-3.5" />Admin only</Badge>
                      {/if}
                    </div>

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
