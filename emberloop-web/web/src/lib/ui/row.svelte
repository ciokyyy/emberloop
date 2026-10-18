<!-- List row, one structure for Sessions, Runs and Projects: status glyph (or
     leading icon tile), title with optional chips, one line of meta, unread
     dot and trailing content. Renders a link when `href` is set, a button when
     `onclick` is set, otherwise a static row. 56 px tall under md.
     `status` is a status token or `idle`; the glyph is decorative and an
     sr-only status word follows it unless `statusWord` is false (the meta
     already names the status). `attention` gives the needs-you tint. `current` marks
     the open row (aria-current); `selected` marks a toggled row. -->
<script>
  import { Circle } from '@lucide/svelte';
  import { cn } from './cn.js';
  import { DOT } from './chip.svelte';
  import { statusIcons, statusLabels } from '../icons.js';

  const GLYPH_TONE = {
    working: 'text-status-working',
    'needs-you': 'text-status-needs-you',
    done: 'text-status-done',
    failed: 'text-status-failed',
    paused: 'text-status-paused',
    idle: 'text-fg-4',
  };

  let {
    title,
    meta = '',
    icon: Icon = null,
    status = null,
    unread = false,
    statusWord = true,
    attention = false,
    current = false,
    href = null,
    onclick = null,
    selected = false,
    badges = null,
    trailing = null,
    class: className = '',
    ...rest
  } = $props();
  const interactive = $derived(Boolean(href || onclick));
  const Glyph = $derived(status === 'idle' ? Circle : status && DOT[status] ? statusIcons[status] : null);
  const cls = $derived(
    cn(
      'flex min-h-12 w-full min-w-0 items-center gap-3 rounded-xl px-3 py-2 text-left max-md:min-h-14',
      interactive && 'motion-standard cursor-pointer outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40',
      (selected || current) && 'bg-surface-3',
      attention && 'border border-status-needs-you/30 bg-status-needs-you/10 hover:bg-status-needs-you/15',
      className,
    ),
  );
</script>

{#snippet body()}
  {#if Glyph}
    <span class={cn('grid size-5 shrink-0 place-items-center', GLYPH_TONE[status])} data-status={status} aria-hidden="true">
      <Glyph class={cn('size-4', status === 'working' && 'motion-safe:animate-pulse')} />
    </span>
    {#if statusWord}<span class="sr-only">{status === 'idle' ? 'Idle' : statusLabels[status]}.</span>{/if}
  {/if}
  {#if Icon}
    <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-fg-2" aria-hidden="true"><Icon class="size-4" /></span>
  {/if}
  <span class="flex min-w-0 flex-1 flex-col">
    <span class="flex min-w-0 items-center gap-1.5">
      <span class="min-w-0 truncate text-body text-fg-1">{title}</span>
      {#if badges}{@render badges()}{/if}
    </span>
    {#if meta}<span class="truncate text-caption text-fg-3">{meta}</span>{/if}
  </span>
  {#if unread}
    <span class="size-2 shrink-0 rounded-full bg-status-working" data-unread aria-hidden="true"></span>
    <span class="sr-only">Unread.</span>
  {/if}
  {#if trailing}<span class="flex shrink-0 items-center gap-2">{@render trailing()}</span>{/if}
{/snippet}

{#if href}
  <a {href} {onclick} class={cls} aria-current={current ? 'page' : selected ? 'true' : undefined} {...rest}>{@render body()}</a>
{:else if onclick}
  <button type="button" {onclick} class={cls} aria-current={current ? 'true' : undefined} aria-pressed={!current && selected ? 'true' : undefined} {...rest}>{@render body()}</button>
{:else}
  <div class={cls} {...rest}>{@render body()}</div>
{/if}
