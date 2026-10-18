<!-- The one status glyph in the chat header. Reconnecting wins, then the working
     Session (pulsing dot, not a spinner: the composer owns the spinner), else the live dot. Colour never stands alone: the word is the
     tooltip and the accessible name. -->
<script>
  import { CircleAlert } from '@lucide/svelte';
  import Tooltip from './ui/tooltip.svelte';
  import { statusLabels } from './icons.js';

  let { connected = true, status = 'idle' } = $props();
  const state = $derived(!connected ? 'reconnecting' : status === 'working' ? 'working' : 'live');
  const word = $derived(state === 'reconnecting' ? 'Reconnecting…' : state === 'working' ? statusLabels.working : 'Live');
</script>

<Tooltip text={word} side="bottom">
  {#snippet trigger(props)}
    <span
      {...props}
      class="grid size-6 shrink-0 place-items-center"
      role="status"
      aria-label={word}
      data-state={state}
      data-testid={state === 'reconnecting' ? 'reconnect' : 'live'}
    >
      {#if state === 'reconnecting'}
        <CircleAlert class="size-4 text-status-needs-you motion-safe:animate-pulse" aria-hidden="true" />
      {:else if state === 'working'}
        <span class="size-2 rounded-full bg-status-working motion-safe:animate-pulse" aria-hidden="true"></span>
      {:else}
        <span class="size-2 rounded-full bg-status-done" aria-hidden="true"></span>
      {/if}
    </span>
  {/snippet}
</Tooltip>
