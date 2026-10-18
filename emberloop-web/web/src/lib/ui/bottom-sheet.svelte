<!-- Bottom sheet on phone, centred dialog on desktop. A secondary editor opens
     here instead of a side panel: `title` names it, `description` is an
     sr-only summary, `children` is the body and `footer` holds actions. -->
<script>
  import { Dialog } from 'bits-ui';
  import IconButton from './icon-button.svelte';
  import { icons } from '../icons.js';
  import { cn } from './cn.js';
  import { holdBack } from '../overlayBack.js';

  let { open = $bindable(false), title, description = '', class: className = '', children, footer = null, ...rest } = $props();

  // System Back closes the sheet, and the entry it holds pops again when the
  // sheet closes from its own UI.
  $effect(() => {
    if (!open) return;
    return holdBack(() => (open = false));
  });
</script>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="motion-emphasis fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0" />
    <Dialog.Content
      class={cn(
        'motion-emphasis fixed z-50 flex max-h-[85dvh] flex-col gap-4 overflow-y-auto border bg-surface-2 p-4 text-fg-1 shadow-elev-3 outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 max-md:inset-x-0 max-md:bottom-[var(--kb-inset)] max-md:max-h-[calc(var(--app-height,100dvh)-3rem)] max-md:rounded-t-2xl max-md:pb-[max(1rem,var(--safe-bottom))] max-md:data-[state=closed]:slide-out-to-bottom max-md:data-[state=open]:slide-in-from-bottom md:top-1/2 md:left-1/2 md:w-full md:max-w-md md:-translate-x-1/2 md:-translate-y-1/2 md:rounded-2xl',
        className,
      )}
      {...rest}
    >
      <div class="flex items-center gap-2">
        <Dialog.Title class="min-w-0 flex-1 truncate text-title">{title}</Dialog.Title>
        <Dialog.Close>
          {#snippet child({ props })}
            <IconButton {...props} icon={icons.dismiss} label="Close" />
          {/snippet}
        </Dialog.Close>
      </div>
      <Dialog.Description class="sr-only">{description || title}</Dialog.Description>
      {@render children?.()}
      {#if footer}<div class="flex items-center gap-2 border-t pt-3">{@render footer()}</div>{/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
