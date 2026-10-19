<!-- Combobox picker (shadcn-svelte combobox on Popover + Command, virtualized).
     items: [{ value, label, detail? }]. The trigger shows the label with the
     detail (pig: provider) muted beside it. Public API matches the plain
     bits-ui Select. -->
<script>
  import { ChevronDown } from '@lucide/svelte';
  import Popover from './popover.svelte';
  import CommandList from './command.svelte';
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

  let open = $state(false);

  const selected = $derived(items.find((i) => i.value === value));

  function pick(v) {
    value = v;
    onValueChange?.(v);
    open = false;
  }
</script>

<Popover
  bind:open
  {disabled}
  sideOffset={6}
  data-placeholder={selected ? undefined : ''}
  class={cn(
    'flex min-h-11 w-full items-center justify-between gap-2 rounded-xl border border-input bg-transparent px-3.5 py-1.5 text-left text-base leading-snug md:text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 disabled:opacity-50 data-[placeholder]:text-muted-foreground',
    className,
  )}
  contentClass="w-(--bits-popover-anchor-width) min-w-40"
  {...rest}
>
  {#snippet trigger()}
    <span class="min-w-0 flex-1 break-words line-clamp-2" title={selected ? `${selected.value}` : undefined}>{selected?.label ?? placeholder}{#if selected?.detail}<span class="text-muted-foreground"> · {selected.detail}</span>{/if}</span>
    <ChevronDown class="size-4 shrink-0 text-muted-foreground" />
  {/snippet}
  {#key open}
    <CommandList
      {items}
      selectedValue={value}
      onSelect={pick}
      label="Search options"
    />
  {/key}
</Popover>
