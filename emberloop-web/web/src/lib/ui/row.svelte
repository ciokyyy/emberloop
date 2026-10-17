<!-- List row: leading icon or status dot, title, one line of meta, trailing
     content. Renders a link when `href` is set, a button when `onclick` is
     set, otherwise a static row. 44 px tall under md. -->
<script>
  import { cn } from './cn.js';
  import { DOT } from './chip.svelte';

  let {
    title,
    meta = '',
    icon: Icon = null,
    status = null,
    href = null,
    onclick = null,
    selected = false,
    trailing = null,
    class: className = '',
    ...rest
  } = $props();
  const interactive = $derived(Boolean(href || onclick));
  const cls = $derived(
    cn(
      'flex min-h-12 w-full min-w-0 items-center gap-3 rounded-xl px-3 py-2 text-left max-md:min-h-14',
      interactive && 'motion-standard cursor-pointer outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40',
      selected && 'bg-surface-3',
      className,
    ),
  );
</script>

{#snippet body()}
  {#if status && DOT[status]}
    <span class={cn('size-2 shrink-0 rounded-full', DOT[status])} data-status={status} aria-hidden="true"></span>
  {/if}
  {#if Icon}
    <span class="grid size-8 shrink-0 place-items-center rounded-lg bg-muted text-fg-2" aria-hidden="true"><Icon class="size-4" /></span>
  {/if}
  <span class="flex min-w-0 flex-1 flex-col">
    <span class="truncate text-body text-fg-1">{title}</span>
    {#if meta}<span class="truncate text-caption text-fg-3">{meta}</span>{/if}
  </span>
  {#if trailing}<span class="flex shrink-0 items-center gap-2">{@render trailing()}</span>{/if}
{/snippet}

{#if href}
  <a {href} {onclick} class={cls} aria-current={selected ? 'true' : undefined} {...rest}>{@render body()}</a>
{:else if onclick}
  <button type="button" {onclick} class={cls} aria-pressed={selected ? 'true' : undefined} {...rest}>{@render body()}</button>
{:else}
  <div class={cls} {...rest}>{@render body()}</div>
{/if}
