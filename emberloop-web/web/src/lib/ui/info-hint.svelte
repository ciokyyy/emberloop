<!-- Tap-to-reveal help. Keeps long explanations off the page: a small info
     button opens a popover. Works with touch, mouse and keyboard. -->
<script>
  import { Info } from '@lucide/svelte';
  import Popover from './popover.svelte';
  import { cn } from './cn.js';

  let { label = 'More info', class: className = '', children } = $props();
</script>

<Popover
  aria-label={label}
  class={cn(
    'motion-standard inline-flex size-6 shrink-0 items-center justify-center rounded-full text-fg-3 outline-none hover:bg-muted hover:text-fg-1 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:size-11',
    className,
  )}
  contentClass="w-72 p-3 text-caption text-fg-2"
>
  {#snippet trigger()}
    <Info class="size-4" aria-hidden="true" />
  {/snippet}
  {@render children?.()}
</Popover>
