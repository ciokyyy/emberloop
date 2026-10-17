<!-- Icon-only button. `label` is required: it is the accessible name and the
     tooltip. Touch size comes from Button (44 px under md). -->
<script>
  import Button from './button.svelte';
  import Tooltip from './tooltip.svelte';
  import { cn } from './cn.js';

  let {
    icon: Icon,
    label,
    variant = 'ghost',
    size = 'icon-sm',
    spin = false,
    tooltip = true,
    side = 'top',
    class: className = '',
    ...rest
  } = $props();
</script>

{#snippet button(extra = {})}
  <Button {variant} {size} aria-label={label} {...extra} {...rest} class={cn(extra.class, className)}>
    {#if Icon}<Icon class={spin ? 'animate-spin' : ''} aria-hidden="true" />{/if}
  </Button>
{/snippet}

{#if tooltip}
  <Tooltip text={label} {side}>
    {#snippet trigger(props)}
      {@render button(props)}
    {/snippet}
  </Tooltip>
{:else}
  {@render button()}
{/if}
