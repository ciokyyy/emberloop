<!-- Read-only quota overview. Every bar is driven only by the Daemon's pct. -->
<script>
  import { onMount } from 'svelte';
  import {
    Clock3,
    Gauge,
    Gem,
    LockKeyhole,
    MessageCircle,
    Plug,
    RefreshCw,
    Route,
    Sparkles,
    Terminal,
    TriangleAlert,
  } from '@lucide/svelte';
  import Button, { buttonVariants } from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import { refreshQuota } from './api.js';
  import { quotaStore } from './quotaStore.js';
  import {
    quotaHint,
    quotaPlan,
    quotaState,
    quotaUpdatedLine,
    resetCountdown,
    thresholdClass,
    wordLabel,
  } from './quota.js';

  let providers = $state([]);
  let loading = $state(true);
  let error = $state('');
  let now = $state(Date.now());
  let busy = $state({});
  let cardError = $state({});
  let refreshingAll = $state(false);

  const BAR = { calm: 'bg-green-500', warm: 'bg-amber-500', hot: 'bg-red-500' };
  const PCT_TEXT = { calm: 'text-muted-foreground', warm: 'text-amber-300', hot: 'text-red-400' };
  const ICONS = {
    claude: Sparkles,
    chatgpt: MessageCircle,
    'openai-codex': MessageCircle,
    'opencode-go': Terminal,
    openrouter: Route,
    'agy-gemini': Gem,
    'agy-3p': Gem,
  };

  const cardIcon = (key) => ICONS[key] ?? Gauge;
  const hasBusyCard = () => Object.values(busy).some(Boolean);

  function apply(body) {
    if (quotaStore.set(body)) now = Date.now();
  }

  async function load() {
    try {
      await quotaStore.load();
    } catch (e) {
      error = e.message || 'Could not load quota.';
    } finally {
      loading = false;
    }
  }

  async function runRefresh(key = '') {
    if ((key && busy[key]) || (!key && (refreshingAll || hasBusyCard()))) return;
    error = '';
    if (key) busy = { ...busy, [key]: true };
    else refreshingAll = true;
    cardError = key ? { ...cardError, [key]: '' } : {};
    try {
      apply(await refreshQuota(key));
    } catch (e) {
      const text = e.message || 'Refresh failed.';
      if (key) cardError = { ...cardError, [key]: text };
      else error = text;
    } finally {
      if (key) busy = { ...busy, [key]: false };
      else refreshingAll = false;
    }
  }

  onMount(() => {
    const unsubscribe = quotaStore.subscribe((value) => {
      providers = value;
      now = Date.now();
    });
    void load();
    const timer = setInterval(() => (now = Date.now()), 30_000);
    return () => {
      clearInterval(timer);
      unsubscribe();
    };
  });
</script>

<div class="flex min-h-0 flex-1 flex-col" data-testid="quota-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-16">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <header class="animate-rise flex flex-col gap-2">
        <div class="flex flex-wrap items-center gap-2.5">
          <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-card" aria-hidden="true"><Gauge class="size-4" /></span>
          <h1 class="text-3xl font-semibold tracking-tight max-md:text-2xl">Quota</h1>
          <Badge variant="outline">Read-only</Badge>
          <Button variant="outline" size="sm" class="ml-auto" onclick={() => runRefresh('')} disabled={refreshingAll || hasBusyCard()} aria-busy={refreshingAll}>
            <RefreshCw class={refreshingAll ? 'animate-spin' : ''} />Refresh all
          </Button>
        </div>
        <p class="text-sm text-muted-foreground">Percent used per account window, and when it resets. Dollars and tokens are secondary. Updates live.</p>
      </header>

      {#if error}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}

      {#if loading}
        <div class="grid gap-4 md:grid-cols-2" aria-label="Loading quota" data-testid="quota-card-grid">
          {#each [0, 1, 2, 3, 4, 5] as i (i)}
            <div class="h-40 animate-pulse rounded-2xl border bg-card"></div>
          {/each}
        </div>
      {:else}
        <div class="grid gap-4 md:grid-cols-2" data-testid="quota-card-grid">
          {#each providers as p (p.key)}
            {@const state = quotaState(p)}
            {@const ProviderIcon = cardIcon(p.key)}
            <article class="animate-rise flex flex-col rounded-2xl border bg-card p-4" data-testid="quota-card" data-key={p.key} data-state={state}>
              <div class="flex items-center gap-2.5">
                <span class="grid size-9 shrink-0 place-items-center rounded-xl bg-muted" aria-hidden="true"><ProviderIcon class="size-4" /></span>
                <div class="min-w-0 flex-1 leading-tight">
                  <div class="flex flex-wrap items-center gap-1.5">
                    <h2 class="text-sm font-semibold">{p.label}</h2>
                    <Badge variant="outline" data-testid="quota-plan">{quotaPlan(p)}</Badge>
                    {#if state === 'stale'}
                      <Badge variant="warning" data-testid="quota-state"><Clock3 class="size-3" />stale</Badge>
                    {:else if state === 'expired'}
                      <Badge variant="warning" data-testid="quota-state"><LockKeyhole class="size-3" />login expired</Badge>
                    {:else if state === 'not_connected'}
                      <Badge variant="secondary" data-testid="quota-state">not connected</Badge>
                    {:else if state === 'error'}
                      <Badge variant="destructive" data-testid="quota-state">error</Badge>
                    {/if}
                  </div>
                  <p class="mt-1 text-[11px] text-muted-foreground" data-testid="quota-updated">{quotaUpdatedLine(p, now)}</p>
                </div>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  class="shrink-0"
                  aria-label={`Refresh ${p.label}`}
                  disabled={busy[p.key] || refreshingAll}
                  aria-busy={Boolean(busy[p.key] || refreshingAll)}
                  onclick={() => runRefresh(p.key)}
                >
                  <RefreshCw class={busy[p.key] || refreshingAll ? 'animate-spin' : ''} />
                </Button>
              </div>

              {#if state === 'not_connected'}
                <p class="mt-3 text-xs text-muted-foreground" data-testid="quota-not-connected">{quotaHint(p)}</p>
                <a href="/providers" class={`${buttonVariants({ variant: 'outline', size: 'sm' })} mt-3 w-fit`}>
                  <Plug class="size-4" />Connect in Providers
                </a>
              {:else}
                {#if state === 'expired'}
                  <div class="mt-3 flex items-start gap-2 rounded-xl bg-amber-500/10 p-3 text-xs text-amber-300" data-testid="quota-expired-callout">
                    <TriangleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
                    {#if p.hint}<span data-testid="quota-hint">{p.hint}</span>{/if}
                    {#if p.key === 'claude'}
                      <span>Host login expired, so quota cannot refresh. Run <code class="font-mono">claude auth login</code> or <a class="inline-flex items-center underline max-md:min-h-11" href="/providers">reconnect in Providers</a>.</span>
                    {:else}
                      <span>{p.label} login expired, so quota cannot refresh. <a class="inline-flex items-center underline max-md:min-h-11" href="/providers">Reconnect in Providers</a>.</span>
                    {/if}
                  </div>
                {/if}

                {#if state === 'error' && (p.error || cardError[p.key])}
                  <p role="alert" class="mt-2 text-xs text-destructive">{cardError[p.key] || p.error}</p>
                {:else if cardError[p.key]}
                  <p role="alert" class="mt-2 text-xs text-destructive">{cardError[p.key]}</p>
                {/if}

                {#if (p.windows ?? []).length}
                  <div class={`mt-3 flex flex-col gap-3 ${state === 'expired' ? 'opacity-50' : ''}`} data-testid="quota-windows">
                    {#each p.windows as w, i (`${w.label}-${i}`)}
                      {@const label = wordLabel(w.label)}
                      {@const tone = thresholdClass(w.pct)}
                      {@const countdown = resetCountdown(w.resets_at, now)}
                      <div data-testid="quota-window">
                        <div class="flex items-baseline justify-between gap-3 text-xs">
                          <span class="font-medium">{label}</span>
                          <span class={`tabular-nums font-medium ${PCT_TEXT[tone]}`} data-testid="quota-pct">{w.pct}%</span>
                        </div>
                        <div
                          class="mt-1 h-1.5 overflow-hidden rounded-full bg-muted"
                          role="progressbar"
                          aria-label={`${p.label} ${label}`}
                          aria-valuenow={w.pct}
                          aria-valuemin="0"
                          aria-valuemax="100"
                        >
                          <div class={`h-full rounded-full transition-all duration-300 ${BAR[tone]}`} style={`width: ${Math.min(100, Math.max(0, Number(w.pct)))}%`}></div>
                        </div>
                        <div class="mt-1 flex flex-wrap items-baseline justify-between gap-x-3 text-[11px] text-muted-foreground">
                          <span>{w.detail ?? ''}</span>
                          {#if countdown}<span>resets in {countdown}</span>{/if}
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else if state === 'error' && !p.error && !cardError[p.key]}
                  <p class="mt-3 text-xs text-muted-foreground">{p.error || 'No quota data available.'}</p>
                {/if}
              {/if}
            </article>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>
