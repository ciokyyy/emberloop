<!-- Segmented control. `mode="toggle"` is a pressed-button group (a view switch);
     `mode="tabs"` is a tablist (arrow keys move, Home and End jump) that points
     every tab at its `panel` element. Segments are 44 px under md. Each item is
     `{ value, label, icon?, count? }`. -->
<script>
  import { cn } from './cn.js';

  let { items = [], value = '', label = '', mode = 'toggle', panel = '', onchange = null, class: className = '', ...rest } = $props();

  const tabs = $derived(mode === 'tabs');

  function pick(next) {
    if (next !== value) onchange?.(next);
  }

  function onKey(event) {
    if (!tabs) return;
    const index = items.findIndex((item) => item.value === value);
    let next = -1;
    if (event.key === 'ArrowRight') next = (index + 1) % items.length;
    else if (event.key === 'ArrowLeft') next = (index - 1 + items.length) % items.length;
    else if (event.key === 'Home') next = 0;
    else if (event.key === 'End') next = items.length - 1;
    if (next < 0) return;
    event.preventDefault();
    pick(items[next].value);
    event.currentTarget.querySelectorAll('[role="tab"]')[next]?.focus();
  }
</script>

<!-- svelte-ignore a11y_interactive_supports_focus -->
<div
  class={cn('flex min-w-0 rounded-full bg-muted p-0.5', className)}
  role={tabs ? 'tablist' : 'group'}
  aria-label={label || undefined}
  onkeydown={onKey}
  data-segmented
  {...rest}
>
  {#each items as item (item.value)}
    {@const active = item.value === value}
    <button
      type="button"
      role={tabs ? 'tab' : undefined}
      aria-selected={tabs ? active : undefined}
      aria-pressed={tabs ? undefined : active}
      tabindex={tabs && !active ? -1 : undefined}
      aria-controls={tabs && panel ? panel : undefined}
      data-value={item.value}
      class={cn(
        'motion-standard inline-flex min-h-8 min-w-0 flex-1 items-center justify-center gap-1.5 rounded-full px-3 text-caption font-medium outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 max-md:text-body [&_svg]:size-3.5 [&_svg]:shrink-0',
        active ? 'bg-surface-3 text-fg-1 shadow-elev-1' : 'text-fg-3 hover:text-fg-1',
      )}
      onclick={() => pick(item.value)}
    >
      {#if item.icon}<item.icon aria-hidden="true" />{/if}
      <span class="truncate">{item.label}</span>
      {#if item.count != null}<span class="tabular-nums text-fg-3">{item.count}</span>{/if}
    </button>
  {/each}
</div>
