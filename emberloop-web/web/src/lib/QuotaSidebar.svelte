<script>
  import { onMount } from 'svelte';
  import Row from './ui/row.svelte';
  import { icons } from './icons.js';
  import { highestPercent, thresholdClass } from './quota.js';
  import { quotaStore } from './quotaStore.js';

  let { onNavigate = () => {}, store = quotaStore, inDrawer = false, current = 'quota' } = $props();
  let providers = $state([]);

  // Calm, warm and hot quota tones read as done, needs-you and failed.
  const STATUS = { calm: 'done', warm: 'needs-you', hot: 'failed' };
  const PCT = { calm: 'text-fg-3', warm: 'text-status-needs-you', hot: 'text-status-failed' };

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
  <h2 class="px-2 pt-1 text-title text-fg-1">Quota and Providers</h2>
  <nav class={`flex min-h-0 flex-1 flex-col gap-3 ${inDrawer ? 'overflow-visible' : 'overflow-auto'}`} aria-label="Quota and Providers navigation">
    <div class="flex flex-col gap-0.5">
      <Row href="/quota" title="Quota" icon={icons.quota} current={current === 'quota'} onclick={(event) => openPath(event, '/quota')} />
      <Row href="/providers" title="Providers" icon={icons.provider} current={current === 'providers'} onclick={(event) => openPath(event, '/providers')} />
    </div>

    <section class="flex flex-col gap-0.5" aria-labelledby="quota-live-status">
      <h3 id="quota-live-status" class="px-2 pb-1 text-caption font-medium text-fg-3">Live status</h3>
      <ul class="flex flex-col gap-0.5" aria-live="polite">
        {#each providers as provider (provider.key)}
          {@const percent = highestPercent(provider)}
          {@const tone = percent === null ? null : thresholdClass(percent)}
          <li data-testid="quota-status-row" data-key={provider.key}>
            <Row title={provider.label ?? provider.key} status={tone === null ? 'idle' : STATUS[tone]} statusWord={false} class="min-h-11 max-md:min-h-11">
              {#snippet trailing()}
                <span class={`text-caption tabular-nums ${tone === null ? 'text-fg-3' : PCT[tone]}`} data-testid="quota-status-percent">
                  {percent === null ? '—' : `${percent}%`}
                </span>
              {/snippet}
            </Row>
          </li>
        {/each}
      </ul>
    </section>
  </nav>
</div>
