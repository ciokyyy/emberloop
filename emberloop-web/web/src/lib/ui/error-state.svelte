<!-- Error pattern: icon, one line, optional detail, one retry action. -->
<script>
  import { TriangleAlert } from '@lucide/svelte';
  import Button from './button.svelte';
  import { icons } from '../icons.js';
  import { cn } from './cn.js';

  let {
    title = 'Something went wrong',
    detail = '',
    onRetry = null,
    retryLabel = 'Try again',
    busy = false,
    class: className = '',
    ...rest
  } = $props();
</script>

<div role="alert" class={cn('flex flex-col items-center gap-3 rounded-2xl border border-status-failed/30 bg-status-failed/8 px-6 py-8 text-center', className)} data-error-state {...rest}>
  <span class="grid size-11 place-items-center rounded-full bg-status-failed/12 text-status-failed" aria-hidden="true"><TriangleAlert class="size-5" /></span>
  <div class="flex flex-col gap-1">
    <p class="text-title text-fg-1">{title}</p>
    {#if detail}<p class="text-caption break-words text-fg-3">{detail}</p>{/if}
  </div>
  {#if onRetry}
    <Button variant="outline" size="sm" onclick={onRetry} disabled={busy} aria-busy={busy}>
      <icons.retry class={busy ? 'animate-spin' : ''} aria-hidden="true" />{retryLabel}
    </Button>
  {/if}
</div>
