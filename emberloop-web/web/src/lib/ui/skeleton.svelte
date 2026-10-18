<!-- Loading placeholder block. Decorative: wrap a group in role="status" with a label. -->
<script>
  import { cn } from './cn.js';

  let { class: className = '', ...rest } = $props();
</script>

<div aria-hidden="true" class={cn('h-4 rounded-md bg-surface-3/70 motion-safe:animate-pulse', className)} {...rest}></div>
