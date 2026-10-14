<script>
  import { onMount } from 'svelte';
  import { Gauge, Plug } from '@lucide/svelte';
  import { highestPercent, thresholdClass } from './quota.js';
  import { quotaStore } from './quotaStore.js';

  let { onNavigate = () => {}, store = quotaStore, inDrawer = false, current = 'quota' } = $props();
  let providers = $state([]);

  const DOT = { calm: 'bg-green-500', warm: 'bg-amber-500', hot: 'bg-red-500' };
  const PCT = { calm: 'text-muted-foreground', warm: 'text-amber-300', hot: 'text-red-400' };

  onMount(() => {
    const unsubscribe = store.subscribe((value) => (providers = value));
    void store.load().catch(() => {});
    return unsubscribe;
  });

  function openPath(event, path) {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    onNavigate(path);
  }

</script>

<div class="animate-rise flex min-h-0 flex-1 flex-col gap-3" data-testid="quota-sidebar">
  <div class="px-2 pt-1.5 pb-1">
    <h1 class="text-base font-semibold tracking-tight">Quota and Providers</h1>
  </div>
  <nav class={`flex min-h-0 flex-1 flex-col gap-3 ${inDrawer ? 'overflow-visible' : 'overflow-auto'}`} aria-label="Quota and Providers navigation">
    <div class="flex flex-col gap-0.5">
      <a
        href="/quota"
        aria-current={current === 'quota' ? 'page' : undefined}
        onclick={(event) => openPath(event, '/quota')}
        class={`flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm max-md:min-h-11 ${current === 'quota' ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}`}
      >
        <Gauge class="size-4 shrink-0" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">Quota</span>
      </a>
      <a
        href="/providers"
        aria-current={current === 'providers' ? 'page' : undefined}
        onclick={(event) => openPath(event, '/providers')}
        class={`flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm max-md:min-h-11 ${current === 'providers' ? 'bg-accent text-foreground' : 'text-muted-foreground hover:bg-accent hover:text-foreground'}`}
      >
        <Plug class="size-4 shrink-0" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate">Providers</span>
      </a>
    </div>

    <section class="flex flex-col gap-0.5" aria-labelledby="quota-live-status">
      <h2 id="quota-live-status" class="px-2 pb-1 text-xs font-medium text-muted-foreground">Live status</h2>
      <ul class="flex flex-col gap-0.5" aria-live="polite">
        {#each providers as provider (provider.key)}
          {@const percent = highestPercent(provider)}
          {@const tone = percent === null ? null : thresholdClass(percent)}
          <li class="flex min-w-0 items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm" data-testid="quota-status-row" data-key={provider.key}>
            <span
              class={`size-2 shrink-0 rounded-full ${tone === null ? 'bg-input' : DOT[tone]}`}
              aria-hidden="true"
              data-testid="quota-status-dot"
            ></span>
            <span class="min-w-0 flex-1 truncate text-muted-foreground">{provider.label ?? provider.key}</span>
            <span class={`shrink-0 text-xs tabular-nums ${tone === null ? 'text-muted-foreground' : PCT[tone]}`} data-testid="quota-status-percent">
              {percent === null ? '—' : `${percent}%`}
            </span>
          </li>
        {/each}
      </ul>
    </section>
  </nav>
</div>
