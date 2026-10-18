<!-- Tooltip on bits-ui: wraps one trigger element. The trigger child receives
     the aria wiring; a tooltip never replaces an accessible name. -->
<script>
  import { Tooltip } from 'bits-ui';
  import { cn } from './cn.js';

  let { text = '', side = 'top', delayDuration = 400, class: className = '', trigger, ...rest } = $props();
</script>

<Tooltip.Provider {delayDuration} disableHoverableContent>
  <Tooltip.Root ignoreNonKeyboardFocus>
    <Tooltip.Trigger>
      {#snippet child({ props })}
        {@render trigger?.(props)}
      {/snippet}
    </Tooltip.Trigger>
    {#if text}
      <Tooltip.Portal>
        <Tooltip.Content
          {side}
          sideOffset={6}
          class={cn(
            'motion-instant z-[70] max-w-[min(18rem,calc(100vw-1rem))] rounded-lg border bg-surface-2 px-2.5 py-1.5 text-caption text-fg-1 shadow-elev-2 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=instant-open]:animate-in data-[state=instant-open]:fade-in-0',
            className,
          )}
          {...rest}
        >
          {text}
        </Tooltip.Content>
      </Tooltip.Portal>
    {/if}
  </Tooltip.Root>
</Tooltip.Provider>
