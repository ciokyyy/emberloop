<!-- shadcn-svelte-idiom Slider on bits-ui. Single value 0-100 by default. -->
<script>
  import { Slider } from 'bits-ui';
  import { cn } from './cn.js';

  let {
    value = $bindable(0),
    min = 0,
    max = 100,
    step = 1,
    disabled = false,
    label = '',
    class: className = '',
    onValueChange,
    onValueCommit,
    ...rest
  } = $props();
</script>

<Slider.Root
  type="single"
  bind:value
  {min}
  {max}
  {step}
  {disabled}
  {onValueChange}
  {onValueCommit}
  class={cn('relative flex w-full touch-none items-center select-none data-[disabled]:opacity-50', className)}
  {...rest}
>
  {#snippet children({ thumbs })}
    <span class="relative h-1.5 w-full grow overflow-hidden rounded-full bg-primary/20">
      <Slider.Range class="absolute h-full bg-primary" />
    </span>
    {#each thumbs as thumb (thumb)}
      <Slider.Thumb
        index={thumb}
        aria-label={label || undefined}
        class="block size-4 shrink-0 rounded-full border border-primary/50 bg-background shadow-sm ring-ring/30 transition-[color,box-shadow] outline-none hover:cursor-grab focus-visible:ring-[3px] disabled:pointer-events-none disabled:opacity-50"
      />
    {/each}
  {/snippet}
</Slider.Root>
