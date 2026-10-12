<!-- shadcn-svelte Popover on bits-ui: trigger plus a blurred rounded-xl panel. -->
<script>
  import { Popover } from 'bits-ui';
  import { cn } from './cn.js';

  let {
    open = $bindable(false),
    onOpenChange,
    disabled = false,
    sideOffset = 6,
    class: className = '',
    contentClass = '',
    trigger,
    children,
    ...rest
  } = $props();
</script>

<Popover.Root bind:open {onOpenChange}>
  <Popover.Trigger {disabled} class={className} {...rest}>
    {@render trigger?.()}
  </Popover.Trigger>
  <Popover.Portal>
    <Popover.Content
      {sideOffset}
      class={cn(
        'z-[60] max-w-[calc(100vw-1rem)] overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-xl outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
        contentClass,
      )}
    >
      {@render children?.()}
    </Popover.Content>
  </Popover.Portal>
</Popover.Root>
