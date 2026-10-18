<!-- Error pattern: icon, one line, optional detail, one retry action.
     `compact` is the same pattern as one row, for a failure beside content
     that is still on screen (a refresh that failed, one card that could not
     load) instead of a page that has nothing to show. The compact row keeps
     its markup on one line on purpose: the alert's text content is then the
     message alone, with no template whitespace. -->
<script>
  import { TriangleAlert } from '@lucide/svelte';
  import Button from './button.svelte';
  import IconButton from './icon-button.svelte';
  import { icons } from '../icons.js';
  import { cn } from './cn.js';

  let {
    title = 'Something went wrong',
    detail = '',
    onRetry = null,
    retryLabel = 'Try again',
    busy = false,
    compact = false,
    class: className = '',
    ...rest
  } = $props();
</script>

{#if compact}
  <!-- One line on purpose: the alert's textContent stays the message alone. -->
  <div role="alert" class={cn('flex flex-wrap items-center gap-2.5 rounded-xl border border-status-failed/30 bg-status-failed/8 px-3 py-2.5 text-left', className)} data-error-state data-compact {...rest}><span class="grid size-7 shrink-0 place-items-center rounded-full bg-status-failed/12 text-status-failed" aria-hidden="true"><TriangleAlert class="size-3.5" /></span><div class="flex min-w-0 flex-1 flex-col"><p class="text-body text-fg-1">{title}</p>{#if detail}<p class="text-caption break-words text-fg-3">{detail}</p>{/if}</div>{#if onRetry}<IconButton icon={icons.retry} label={retryLabel} spin={busy} disabled={busy} aria-busy={busy} class="shrink-0" onclick={onRetry} />{/if}</div>
{:else}
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
{/if}
