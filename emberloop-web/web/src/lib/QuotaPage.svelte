<!-- Read-only quota overview. Every bar is driven only by the Daemon's pct. -->
<script>
  import { onMount } from 'svelte';
  import { Clock3, LockKeyhole, TriangleAlert } from '@lucide/svelte';
  import { buttonVariants } from './ui/button.svelte';
  import Chip from './ui/chip.svelte';
  import IconButton from './ui/icon-button.svelte';
  import InfoHint from './ui/info-hint.svelte';
  import Section from './ui/section.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import StatCard from './ui/stat-card.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import { icons, providerGlyph } from './icons.js';
  import { refreshQuota } from './api.js';
  import { quotaStore } from './quotaStore.js';
  import {
    highestPercent,
    quotaHint,
    quotaPlan,
    quotaState,
    quotaUpdatedLine,
    providerRowHref,
    resetCountdown,
    thresholdClass,
    wordLabel,
  } from './quota.js';

  let { onNavigate = null } = $props();
  let providers = $state([]);
  let loading = $state(true);
  let error = $state('');
  let now = $state(Date.now());
  let busy = $state({});
  let cardError = $state({});
  let refreshingAll = $state(false);

  const BAR = { calm: 'bg-status-done', warm: 'bg-status-needs-you', hot: 'bg-status-failed' };
  const PCT_TEXT = { calm: 'text-fg-1', warm: 'text-status-needs-you', hot: 'text-status-failed' };
  const STAT_TONE = { calm: null, warm: 'needs-you', hot: 'failed' };

  // Headline numbers: the busiest window anywhere, how many accounts are
  // connected, and how many need a look. All from the Daemon's pct and state.
  const summary = $derived.by(() => {
    const live = providers.filter((p) => quotaState(p) !== 'not_connected');
    let peak = null;
    let peakLabel = '';
    for (const p of live) {
      const pct = highestPercent(p);
      if (pct !== null && (peak === null || pct > peak)) {
        peak = pct;
        peakLabel = p.label;
      }
    }
    const attention = providers.filter((p) => ['stale', 'expired', 'error'].includes(quotaState(p))).length;
    return { peak, peakLabel, connected: live.length, total: providers.length, attention };
  });
  const hasBusyCard = () => Object.values(busy).some(Boolean);

  function navigateToProvider(event, key) {
    if (typeof onNavigate !== 'function' || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(providerRowHref(key));
  }

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

  async function retryLoad() {
    error = '';
    loading = true;
    await load();
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
    return unsubscribe;
  });
</script>

<div class="flex min-h-0 flex-1 flex-col" data-testid="quota-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-[max(4rem,env(safe-area-inset-bottom))] md:px-6">
    <div class="mx-auto flex w-full max-w-4xl flex-col gap-6">
      <header class="animate-rise flex items-center gap-3">
        <span class="grid size-10 shrink-0 place-items-center rounded-xl bg-card text-fg-2 shadow-elev-1" aria-hidden="true"><icons.quota class="size-5" /></span>
        <h1 class="shrink-0 text-headline">Quota</h1>
        <Chip tone="outline" class="max-sm:hidden">Read-only</Chip>
        <InfoHint label="About quota">Percent used per account window, and when it resets. Dollars and tokens are secondary. Updates live.</InfoHint>
        <IconButton
          icon={icons.refresh}
          label="Refresh all"
          variant="outline"
          size="icon"
          class="ml-auto"
          spin={refreshingAll}
          onclick={() => runRefresh('')}
          disabled={refreshingAll || hasBusyCard()}
          aria-busy={refreshingAll}
        />
      </header>

      {#if error && providers.length}
        <p role="alert" class="rounded-xl bg-status-failed/10 px-4 py-2 text-body text-status-failed">{error}</p>
      {/if}

      {#if loading}
        <div role="status" aria-label="Loading quota" class="@container flex flex-col gap-6">
          <div class="grid grid-cols-3 gap-2 md:gap-4">
            {#each [0, 1, 2] as i (i)}<Skeleton class="h-24 rounded-2xl" />{/each}
          </div>
          <div class="grid gap-4 @2xl:grid-cols-2" data-testid="quota-card-grid">
            {#each [0, 1, 2, 3, 4, 5] as i (i)}<Skeleton class="h-44 rounded-2xl" />{/each}
          </div>
        </div>
      {:else if error && !providers.length}
        <ErrorState title="Could not load quota" detail={error} onRetry={retryLoad} />
      {:else if !providers.length}
        <EmptyState icon={icons.quota} title="No quota sources yet" description="Connect a provider to see its usage here.">
          {#snippet action()}
            <a href="/providers" class={buttonVariants({ variant: 'outline', size: 'sm' })}><icons.provider />Open Providers</a>
          {/snippet}
        </EmptyState>
      {:else}
        <div class="grid grid-cols-3 gap-2 md:gap-4" data-testid="quota-summary">
          <StatCard
            label="Peak use"
            value={summary.peak ?? '–'}
            unit={summary.peak === null ? '' : '%'}
            caption={summary.peakLabel || 'no data yet'}
            tone={summary.peak === null ? null : STAT_TONE[thresholdClass(summary.peak)]}
            icon={icons.quota}
          />
          <StatCard label="Connected" value={summary.connected} unit={`/${summary.total}`} caption="accounts" icon={icons.provider} />
          <StatCard
            label="Attention"
            value={summary.attention}
            caption={summary.attention ? 'need a look' : 'all clear'}
            tone={summary.attention ? 'needs-you' : 'done'}
            icon={TriangleAlert}
          />
        </div>

        <Section title="Providers" icon={icons.provider}>
        <div class="@container">
        <div class="grid gap-4 @2xl:grid-cols-2" data-testid="quota-card-grid">
          {#each providers as p (p.key)}
            {@const state = quotaState(p)}
            {@const ProviderIcon = providerGlyph(p.key)}
            {@const windows = p.windows ?? []}
            <article class="animate-rise flex min-w-0 flex-col gap-4 rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="quota-card" data-key={p.key} data-state={state}>
              <div class="flex items-center gap-3">
                <span class="grid size-10 shrink-0 place-items-center rounded-xl bg-muted text-fg-2" aria-hidden="true"><ProviderIcon class="size-5" /></span>
                <div class="min-w-0 flex-1">
                  <h3 class="truncate text-title">{p.label}</h3>
                  <p class="truncate text-caption text-fg-3" data-testid="quota-updated">{quotaUpdatedLine(p, now)}</p>
                </div>
                <IconButton
                  icon={icons.refresh}
                  label={`Refresh ${p.label}`}
                  spin={busy[p.key] || refreshingAll}
                  disabled={busy[p.key] || refreshingAll}
                  aria-busy={Boolean(busy[p.key] || refreshingAll)}
                  onclick={() => runRefresh(p.key)}
                />
              </div>

              <div class="-mt-1 flex flex-wrap items-center gap-1.5">
                <Chip tone="outline" data-testid="quota-plan">{quotaPlan(p)}</Chip>
                {#if state === 'stale'}
                  <Chip tone="needs-you" icon={Clock3} data-testid="quota-state">stale</Chip>
                {:else if state === 'expired'}
                  <Chip tone="needs-you" icon={LockKeyhole} data-testid="quota-state">login expired</Chip>
                {:else if state === 'not_connected'}
                  <Chip tone="paused" dot data-testid="quota-state">not connected</Chip>
                {:else if state === 'error'}
                  <Chip tone="failed" dot data-testid="quota-state">error</Chip>
                {/if}
              </div>

              {#if state === 'not_connected'}
                <p class="text-caption text-fg-3" data-testid="quota-not-connected">{quotaHint(p)}</p>
                <a href={providerRowHref(p.key)} onclick={(event) => navigateToProvider(event, p.key)} class={`${buttonVariants({ variant: 'outline', size: 'sm' })} w-fit`}>
                  <icons.provider />Connect in Providers
                </a>
              {:else}
                {#if state === 'expired'}
                  <div class="flex items-start gap-2 rounded-xl bg-status-needs-you/10 p-3 text-caption text-status-needs-you" data-testid="quota-expired-callout">
                    <TriangleAlert class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
                    <div class="flex min-w-0 flex-col gap-0.5">
                      {#if p.hint}<span data-testid="quota-hint">{p.hint}</span>{/if}
                      <span>{p.key === 'claude' ? 'Host login expired, so quota cannot refresh. Run ' : `${p.label} login expired, so quota cannot refresh.`}{#if p.key === 'claude'}<code class="font-mono">claude auth login</code>, or{/if}</span>
                      <a class="inline-flex w-fit items-center underline max-md:min-h-11" href="/providers">Reconnect in Providers</a>
                    </div>
                  </div>
                {/if}

                {#if state === 'error' && (p.error || cardError[p.key])}
                  <p role="alert" class="text-caption text-status-failed">{cardError[p.key] || p.error}</p>
                {:else if cardError[p.key]}
                  <p role="alert" class="text-caption text-status-failed">{cardError[p.key]}</p>
                {/if}

                {#if windows.length}
                  <div class={`grid gap-x-4 gap-y-5 ${windows.length === 2 ? 'grid-cols-2' : windows.length >= 3 ? 'sm:grid-cols-3' : ''}`} data-testid="quota-windows" data-dimmed={state === 'expired' ? 'true' : undefined}>
                    {#each windows as w, i (`${w.label}-${i}`)}
                      {@const label = wordLabel(w.label)}
                      {@const tone = thresholdClass(w.pct)}
                      {@const countdown = resetCountdown(w.resets_at, now)}
                      <div class="flex min-w-0 flex-col gap-2" data-testid="quota-window">
                        <div class="flex min-w-0 flex-col">
                          <span class={`text-numeric tabular-nums ${state === 'expired' ? 'text-fg-3' : PCT_TEXT[tone]}`} data-testid="quota-pct">{w.pct}%</span>
                          <span class="mt-1 truncate text-body text-fg-2">{label}</span>
                        </div>
                        <div
                          class={`h-1.5 overflow-hidden rounded-full bg-muted ${state === 'expired' ? 'opacity-40' : ''}`}
                          role="progressbar"
                          aria-label={`${p.label} ${label}`}
                          aria-valuenow={w.pct}
                          aria-valuemin="0"
                          aria-valuemax="100"
                        >
                          <div class={`motion-emphasis h-full rounded-full transition-[width] ${BAR[tone]}`} style={`width: ${Math.min(100, Math.max(0, Number(w.pct)))}%`}></div>
                        </div>
                        <div class="flex min-w-0 flex-col text-caption text-fg-3">
                          {#if countdown}<span>{countdown === 'now' ? 'resets now' : `resets in ${countdown}`}</span>{/if}
                          {#if w.detail}<span class="break-words">{w.detail}</span>{/if}
                        </div>
                      </div>
                    {/each}
                  </div>
                {:else if state === 'error' && !p.error && !cardError[p.key]}
                  <p class="text-caption text-fg-3">{p.error || 'No quota data available.'}</p>
                {/if}
              {/if}
            </article>
          {/each}
        </div>
        </div>
        </Section>
      {/if}
    </div>
  </div>
</div>
