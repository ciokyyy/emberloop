<!-- Collapsible details pane: Session metadata and transcript stats. -->
<script>
  import { X } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import { modelLabel } from './harnesses.js';
  let { session = null, eventCount = 0, open = false, onToggle = () => {} } = $props();

  const rows = $derived(
    session
      ? [
          ['Harness', session.harness],
          ['Model', modelLabel(session.harness, session.model)],
          ['Working directory', session.workDir, true],
          ['Status', session.status],
          ['Events', eventCount],
          ['Session ID', session.id, true],
        ]
      : [],
  );
</script>

{#if open && session}
  <aside aria-label="Session details" class="animate-in fade-in-0 slide-in-from-right-4 duration-300 flex w-72 shrink-0 flex-col gap-4 overflow-auto rounded-2xl border bg-card p-4 text-sm">
    <div class="flex items-center justify-between">
      <h2 class="font-semibold">Details</h2>
      <Button variant="ghost" size="icon-sm" onclick={onToggle} aria-label="Close details"><X /></Button>
    </div>
    <dl class="flex flex-col gap-3">
      {#each rows as [k, v, mono]}
        <div>
          <dt class="text-xs text-muted-foreground">{k}</dt>
          <dd class="break-all {mono ? 'font-mono text-xs' : 'font-medium'}">{v}</dd>
        </div>
      {/each}
    </dl>
  </aside>
{/if}
