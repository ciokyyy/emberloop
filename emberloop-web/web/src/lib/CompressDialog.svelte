<!-- Compress dialog: optional focus instructions for the summary. -->
<script>
  import Button from './ui/button.svelte';
  import Dialog from './ui/dialog.svelte';
  import ErrorState from './ui/error-state.svelte';
  import FloatingInput from './ui/floating-input.svelte';

  let { open = false, busy = false, error = '', usage = null, onClose = () => {}, onCompress = () => {} } = $props();
  let instructions = $state('');

  $effect(() => {
    if (open) instructions = '';
  });

  const fmt = (n) => (n >= 1000 ? `${(n / 1000).toFixed(n >= 100000 ? 0 : 1)}k` : String(n));

  function submit(ev) {
    ev.preventDefault();
    onCompress(instructions.trim());
  }
</script>

<Dialog {open} onOpenChange={(v) => !v && onClose()} title="Compress context" description="Older history becomes a summary; recent turns stay as they are. The Session keeps its id.">
  <form onsubmit={submit} class="flex flex-col gap-4">
    {#if usage}
      <p class="text-sm text-muted-foreground">Now {fmt(usage.used)} of {fmt(usage.size)} tokens.</p>
    {/if}
    <FloatingInput id="compress-focus" label="Keep in focus (optional)" bind:value={instructions} placeholder="file names, decisions, open bugs" autofocus />
    {#if error}
      <ErrorState compact title={error} />
    {/if}
    <div class="mt-1 flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={onClose}>Cancel</Button>
      <Button type="submit" disabled={busy}>{busy ? 'Starting…' : 'Compress'}</Button>
    </div>
  </form>
</Dialog>
