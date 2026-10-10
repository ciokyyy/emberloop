<!-- Quota page: one card per provider (Claude, OpenCode-Go, Antigravity
     Gemini, Antigravity 3P) with a percent bar per window and a reset
     countdown, plus per-card and refresh-all buttons. Loads once on mount
     and never polls; only the buttons fetch again. -->
<script>
  import { ArrowLeft, RefreshCw } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import { getQuota, refreshQuota } from './api.js';
  import { thresholdClass, resetCountdown, staleLabel } from './quota.js';

  let { onBack = () => {} } = $props();

  const BAR = { calm: 'bg-green-500', warm: 'bg-amber-500', hot: 'bg-red-500' };
  const PCT_TEXT = { calm: 'text-muted-foreground', warm: 'text-amber-300', hot: 'text-red-400' };

  let providers = $state([]);
  let loading = $state(true);
  let error = $state('');
  let now = $state(Date.now());
  // Per-card busy and transport errors keep a failing refresh from blanking
  // a card that already shows good values.
  let busy = $state({});
  let cardError = $state({});
  let refreshingAll = $state(false);

  function apply(body) {
    providers = body?.providers ?? [];
    now = Date.now();
  }

  async function load() {
    try {
      apply(await getQuota());
    } catch (e) {
      error = e.message || 'Could not load quota.';
    } finally {
      loading = false;
    }
  }
  load();

  async function runRefresh(key = '') {
    error = '';
    if (key) busy = { ...busy, [key]: true, };
    else refreshingAll = true;
    cardError = key ? { ...cardError, [key]: '' } : {};
    try {
      apply(await refreshQuota(key));
    } catch (e) {
      // Transport failure (Daemon down or a rejected key): keep the last
      // values and put the error on the card (or the page for refresh-all).
      const text = e.message || 'Refresh failed.';
      if (key) cardError = { ...cardError, [key]: text };
      else error = text;
    } finally {
      if (key) busy = { ...busy, [key]: false };
      else refreshingAll = false;
    }
  }
</script>

<div class="flex min-h-0 flex-1 flex-col" data-testid="quota-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-16">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div class="flex items-end gap-3">
        <div class="flex flex-col gap-1">
          <Button variant="ghost" size="sm" class="-ml-3 w-fit text-muted-foreground" onclick={onBack}><ArrowLeft />Back</Button>
          <h1 class="text-3xl font-semibold tracking-tight">Quota</h1>
          <p class="text-sm text-muted-foreground">How much of each account window is used, and when it resets. Loaded once — refresh to update.</p>
        </div>
        <Button variant="outline" size="sm" class="ml-auto shrink-0" onclick={() => runRefresh('')} disabled={refreshingAll}>
          <RefreshCw class={refreshingAll ? 'animate-spin' : ''} />Refresh all
        </Button>
      </div>

      {#if error}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}

      {#if loading}
        <div class="grid gap-4 sm:grid-cols-2">
          {#each [0, 1, 2, 3] as i (i)}
            <div class="h-40 animate-pulse rounded-2xl bg-card"></div>
          {/each}
        </div>
      {:else}
        <div class="grid gap-4 sm:grid-cols-2">
          {#each providers as p (p.key)}
            <div class="animate-rise flex flex-col rounded-2xl border bg-card p-4" data-testid="quota-card" data-key={p.key} data-state={p.state}>
              <div class="flex items-center gap-2">
                <h2 class="text-sm font-semibold">{p.label}</h2>
                {#if p.state === 'error'}<Badge variant="destructive">error</Badge>{/if}
                {#if p.key === 'claude' && p.state !== 'not_configured'}
                  <span class="text-xs text-muted-foreground" data-testid="quota-updated">{staleLabel(p.updated_at, now)}</span>
                {/if}
                <Button
                  variant="ghost"
                  size="icon-sm"
                  class="ml-auto"
                  aria-label={`Refresh ${p.label}`}
                  disabled={busy[p.key] || refreshingAll}
                  onclick={() => runRefresh(p.key)}
                >
                  <RefreshCw class={busy[p.key] ? 'animate-spin' : ''} />
                </Button>
              </div>

              {#if p.state === 'not_configured'}
                <p class="mt-3 text-sm text-muted-foreground" data-testid="quota-not-configured">Not configured</p>
              {:else}
                {#if p.error || cardError[p.key]}
                  <p role="alert" class="mt-2 text-xs text-destructive">{cardError[p.key] || p.error}</p>
                {/if}
                <div class="mt-3 flex flex-col gap-3">
                  {#each p.windows as w (w.label)}
                    {@const tone = thresholdClass(w.pct)}
                    <div>
                      <div class="flex items-baseline justify-between text-xs">
                        <span class="font-medium">{w.label}</span>
                        <span class={`tabular-nums font-medium ${PCT_TEXT[tone]}`} data-testid="quota-pct">{w.pct}%</span>
                      </div>
                      <div
                        class="mt-1 h-1.5 overflow-hidden rounded-full bg-muted"
                        role="progressbar"
                        aria-label={`${p.label} ${w.label}`}
                        aria-valuenow={w.pct}
                        aria-valuemin="0"
                        aria-valuemax="100"
                      >
                        <div class={`h-full rounded-full transition-all duration-300 ${BAR[tone]}`} style={`width: ${Math.min(100, Math.max(0, w.pct))}%`}></div>
                      </div>
                      {#if resetCountdown(w.resets_at, now)}<span class="mt-1 block text-[11px] text-muted-foreground">resets in {resetCountdown(w.resets_at, now)}</span>{/if}
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</div>
