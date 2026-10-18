<!-- Usage windows of one quota account, percent first: a big tabular percent,
     a bar and the reset countdown per window. Shared by the Quota and
     Providers cards so both read the same. Every bar is driven only by the
     Daemon's pct. -->
<script>
  import { resetCountdown, thresholdClass, wordLabel } from './quota.js';

  // windows: [{ label, pct, resets_at, detail }]; name prefixes each bar's
  // accessible name; dimmed greys the numbers for an expired login.
  let { windows = [], name = '', now = Date.now(), dimmed = false } = $props();

  const BAR = { calm: 'bg-status-done', warm: 'bg-status-needs-you', hot: 'bg-status-failed' };
  const PCT_TEXT = { calm: 'text-fg-1', warm: 'text-status-needs-you', hot: 'text-status-failed' };
</script>

{#if windows.length}
  <div class={`grid gap-x-4 gap-y-5 ${windows.length === 2 ? 'grid-cols-2' : windows.length >= 3 ? 'sm:grid-cols-3' : ''}`} data-testid="quota-windows" data-dimmed={dimmed ? 'true' : undefined}>
    {#each windows as w, i (`${w.label}-${i}`)}
      {@const label = wordLabel(w.label)}
      {@const tone = thresholdClass(w.pct)}
      {@const countdown = resetCountdown(w.resets_at, now)}
      <div class="flex min-w-0 flex-col gap-2" data-testid="quota-window">
        <div class="flex min-w-0 flex-col">
          <span class={`text-numeric tabular-nums ${dimmed ? 'text-fg-3' : PCT_TEXT[tone]}`} data-testid="quota-pct">{w.pct}%</span>
          <span class="mt-1 truncate text-body text-fg-2">{label}</span>
        </div>
        <div
          class={`h-1.5 overflow-hidden rounded-full bg-muted ${dimmed ? 'opacity-40' : ''}`}
          role="progressbar"
          aria-label={`${name} ${label}`.trim()}
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
{/if}
