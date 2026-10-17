<!-- Grouped surface for settings and list content: one rounded card, rows
     separated by hairlines (`divided`) or free content with padding
     (`padded`). Pair with SettingRow; the card style matches Quota cards. -->
<script>
  import { cn } from './cn.js';

  let { divided = true, padded = false, tag = 'div', class: className = '', children, ...rest } = $props();
</script>

<svelte:element
  this={tag}
  class={cn(
    'flex min-w-0 flex-col overflow-hidden rounded-2xl border bg-card shadow-elev-1',
    divided && 'divide-y divide-border',
    padded && 'gap-3 p-4',
    className,
  )}
  data-panel
  {...rest}
>
  {@render children?.()}
</svelte:element>
