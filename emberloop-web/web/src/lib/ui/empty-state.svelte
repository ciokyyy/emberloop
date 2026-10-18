<!-- Empty pattern: an icon, one line, an optional second line, one action.
     `compact` is the same pattern as one row, for a small region inside a
     page (a card list, a strip, a popover) instead of a whole page. -->
<script>
  import { Inbox } from '@lucide/svelte';
  import { cn } from './cn.js';

  let { icon: Icon = Inbox, title, description = '', action = null, compact = false, class: className = '', ...rest } = $props();
</script>

{#if compact}
  <div class={cn('flex flex-wrap items-center gap-2.5 rounded-xl border border-dashed px-3 py-2.5 text-left', className)} data-empty-state data-compact {...rest}>
    <span class="grid size-7 shrink-0 place-items-center rounded-full bg-muted text-fg-3" aria-hidden="true"><Icon class="size-3.5" /></span>
    <div class="flex min-w-0 flex-1 flex-col">
      <p class="text-body text-fg-1">{title}</p>
      {#if description}<p class="text-caption text-fg-3">{description}</p>{/if}
    </div>
    {#if action}<div class="shrink-0">{@render action()}</div>{/if}
  </div>
{:else}
  <div class={cn('flex flex-col items-center gap-3 rounded-2xl border border-dashed px-6 py-10 text-center', className)} data-empty-state {...rest}>
    <span class="grid size-11 place-items-center rounded-full bg-muted text-fg-3" aria-hidden="true"><Icon class="size-5" /></span>
    <div class="flex flex-col gap-1">
      <p class="text-title text-fg-1">{title}</p>
      {#if description}<p class="text-caption text-fg-3">{description}</p>{/if}
    </div>
    {#if action}<div class="mt-1">{@render action()}</div>{/if}
  </div>
{/if}
