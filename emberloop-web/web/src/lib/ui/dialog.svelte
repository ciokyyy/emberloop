<!-- shadcn-svelte Dialog on bits-ui: blurred overlay, centered card. -->
<script>
  import { Dialog } from 'bits-ui';
  import { X } from '@lucide/svelte';
  import { cn } from './cn.js';

  let { open = $bindable(false), title = '', description = '', class: className = '', children, onOpenChange } = $props();
</script>

<Dialog.Root bind:open {onOpenChange}>
  <Dialog.Portal>
    <Dialog.Overlay
      class="fixed inset-0 z-50 bg-black/40 backdrop-blur-sm data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0"
    />
    <Dialog.Content
      class={cn(
        'fixed top-1/2 left-1/2 z-50 w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 rounded-2xl border bg-popover p-6 text-popover-foreground shadow-2xl outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95',
        className,
      )}
    >
      <div class="mb-5 flex flex-col gap-1">
        <Dialog.Title class="text-lg font-semibold tracking-tight">{title}</Dialog.Title>
        {#if description}
          <Dialog.Description class="text-sm text-muted-foreground">{description}</Dialog.Description>
        {/if}
      </div>
      {@render children?.()}
      <Dialog.Close
        class="absolute top-4 right-4 flex size-8 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
        aria-label="Close"
      >
        <X class="size-4" />
      </Dialog.Close>
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
