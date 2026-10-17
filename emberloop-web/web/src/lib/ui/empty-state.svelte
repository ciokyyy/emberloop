<!-- Empty pattern: an icon, one line, an optional second line, one action. -->
<script>
  import { Inbox } from '@lucide/svelte';
  import { cn } from './cn.js';

  let { icon: Icon = Inbox, title, description = '', action = null, class: className = '', ...rest } = $props();
</script>

<div class={cn('flex flex-col items-center gap-3 rounded-2xl border border-dashed px-6 py-10 text-center', className)} data-empty-state {...rest}>
  <span class="grid size-11 place-items-center rounded-full bg-muted text-fg-3" aria-hidden="true"><Icon class="size-5" /></span>
  <div class="flex flex-col gap-1">
    <p class="text-title text-fg-1">{title}</p>
    {#if description}<p class="text-caption text-fg-3">{description}</p>{/if}
  </div>
  {#if action}<div class="mt-1">{@render action()}</div>{/if}
</div>
