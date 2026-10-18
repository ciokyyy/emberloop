<!-- Headline number card: label, big tabular value with optional unit, one
     caption. `tone` colours the value with a status token. -->
<script>
  import { cn } from './cn.js';

  const TONE = {
    working: 'text-status-working',
    'needs-you': 'text-status-needs-you',
    done: 'text-status-done',
    failed: 'text-status-failed',
    paused: 'text-status-paused',
  };

  let { label, value, unit = '', caption = '', tone = null, icon: Icon = null, class: className = '', ...rest } = $props();
</script>

<div class={cn('flex min-w-0 flex-col gap-1.5 rounded-2xl border bg-card p-4 shadow-elev-1 max-sm:p-3', className)} data-stat-card {...rest}>
  <div class="flex min-w-0 items-center gap-1.5 text-caption text-fg-3">
    {#if Icon}<Icon class="size-3.5 shrink-0" aria-hidden="true" />{/if}
    <span class="truncate">{label}</span>
  </div>
  <div class={cn('flex flex-wrap items-baseline gap-x-0.5', tone ? TONE[tone] : 'text-fg-1')}>
    <span class="text-numeric tabular-nums max-sm:text-[1.5rem]" data-stat-value>{value}</span>
    {#if unit}<span class="text-title tabular-nums opacity-70">{unit}</span>{/if}
  </div>
  {#if caption}<div class="truncate text-caption text-fg-3">{caption}</div>{/if}
</div>
