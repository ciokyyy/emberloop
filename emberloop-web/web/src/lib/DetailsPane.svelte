<!-- Collapsible details pane: Session metadata and transcript stats. -->
<script>
  let { session = null, eventCount = 0, open = false, onToggle = () => {} } = $props();
</script>

{#if open && session}
  <aside aria-label="Session details" class="flex w-64 shrink-0 flex-col gap-3 overflow-auto rounded-2xl border bg-card p-4 text-sm shadow-sm">
    <div class="flex items-center justify-between">
      <h2 class="font-semibold">Details</h2>
      <button onclick={onToggle} aria-label="Close details" class="rounded-md px-2 py-1 text-muted-foreground hover:bg-muted">✕</button>
    </div>
    <dl class="flex flex-col gap-2">
      <div><dt class="text-xs text-muted-foreground">Harness</dt><dd class="font-medium">{session.harness}</dd></div>
      <div><dt class="text-xs text-muted-foreground">Model</dt><dd class="font-medium">{session.model}</dd></div>
      <div><dt class="text-xs text-muted-foreground">Working directory</dt><dd class="break-all font-mono text-xs">{session.workDir}</dd></div>
      <div><dt class="text-xs text-muted-foreground">Status</dt><dd class="font-medium">{session.status}</dd></div>
      <div><dt class="text-xs text-muted-foreground">Events</dt><dd class="font-medium">{eventCount}</dd></div>
      <div><dt class="text-xs text-muted-foreground">Session ID</dt><dd class="break-all font-mono text-xs">{session.id}</dd></div>
    </dl>
  </aside>
{/if}
