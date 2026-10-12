<!-- shadcn-svelte Sheet on bits-ui Dialog: slides in from the left. -->
<script>
  import { Dialog } from 'bits-ui';
  import { cn } from './cn.js';

  let { open = $bindable(false), title = '', class: className = '', children } = $props();
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay
      class="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0"
    />
    <Dialog.Content
      class={cn(
        'fixed inset-y-0 left-0 z-50 flex w-80 max-w-[88vw] flex-col gap-2 overflow-hidden border-r bg-sidebar p-3 pt-[max(0.75rem,env(safe-area-inset-top))] pb-[max(0.75rem,env(safe-area-inset-bottom))] outline-none duration-300 data-[state=closed]:animate-out data-[state=closed]:slide-out-to-left data-[state=open]:animate-in data-[state=open]:slide-in-from-left',
        className,
      )}
    >
      <Dialog.Title class="sr-only">{title}</Dialog.Title>
      {@render children?.()}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
