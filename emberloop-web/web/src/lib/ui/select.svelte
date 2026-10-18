<!-- shadcn-svelte Select on bits-ui. items: [{ value, label }]. -->
<script>
  import { Select } from 'bits-ui';
  import { Check, ChevronDown } from '@lucide/svelte';
  import { cn } from './cn.js';

  let {
    value = $bindable(''),
    items = [],
    placeholder = 'Select…',
    disabled = false,
    class: className = '',
    onValueChange,
    ...rest
  } = $props();

  const selected = $derived(items.find((i) => i.value === value));
</script>

<Select.Root type="single" bind:value {items} {disabled} {onValueChange}>
  <Select.Trigger
    class={cn(
      'flex h-11 w-full items-center justify-between gap-2 rounded-xl border border-input bg-transparent px-3.5 text-left text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 disabled:opacity-50 data-[placeholder]:text-muted-foreground',
      className,
    )}
    {...rest}
  >
    <span class="min-w-0 flex-1 truncate">{selected?.label ?? placeholder}</span>
    <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
  </Select.Trigger>
  <Select.Portal>
    <Select.Content
      sideOffset={6}
      class="motion-standard z-[60] max-h-72 w-(--bits-select-anchor-width) min-w-40 overflow-hidden rounded-xl border bg-popover text-popover-foreground shadow-xl outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95"
    >
      <Select.Viewport class="p-1.5">
        {#each items as item (item.value)}
          <Select.Item
            value={item.value}
            label={item.label}
            class="relative flex w-full cursor-default items-center rounded-lg py-2 pr-8 pl-3 text-sm outline-none select-none data-[highlighted]:bg-accent data-[disabled]:opacity-50"
          >
            {#snippet children({ selected })}
              <span class="truncate">{item.label}</span>
              {#if selected}
                <Check class="absolute right-2.5 size-4" />
              {/if}
            {/snippet}
          </Select.Item>
        {/each}
      </Select.Viewport>
    </Select.Content>
  </Select.Portal>
</Select.Root>
