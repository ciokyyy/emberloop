<script>
  import { cn } from './cn.js';

  let { class: className = '', value = $bindable(''), ...rest } = $props();
</script>

<input
  bind:value
  class={cn(
    'flex h-11 w-full rounded-xl border border-input bg-transparent px-3.5 text-sm shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 disabled:opacity-50',
    className,
  )}
  {...rest}
/>
