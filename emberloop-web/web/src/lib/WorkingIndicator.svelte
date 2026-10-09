<script>
  let { status = 'idle', label = 'Working…', class: cls = 'px-3 pt-1' } = $props();
</script>

{#if status === 'working'}
  <div data-testid="working-indicator" role="status" class="flex items-center gap-2 text-xs text-muted-foreground {cls}">
    <svg class="working-spinner size-3.5 animate-spin motion-reduce:animate-none" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle cx="12" cy="12" r="9" stroke="currentColor" stroke-opacity="0.25" stroke-width="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" stroke-width="3" stroke-linecap="round" />
    </svg>
    <span>{label}</span>
  </div>
{/if}
