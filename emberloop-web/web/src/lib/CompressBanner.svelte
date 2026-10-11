<!-- Banner for a Session whose Compress stopped on a credit or auth failure. -->
<script>
  import Button from './ui/button.svelte';
  import { compressSession, dismissCompress } from './api.js';
  let { sessionId, onChangeModel = () => {}, onDismissed = () => {} } = $props();
  let busy = $state(false);
  let error = $state('');
  // run takes one request: the buttons stay off while it runs, and its refusal shows in place.
  async function run(request) {
    busy = true;
    error = '';
    try {
      await request();
    } catch (err) {
      error = err.message;
    } finally {
      busy = false;
    }
  }
  const retry = () => run(() => compressSession(sessionId));
  const dismiss = () =>
    run(async () => {
      await dismissCompress(sessionId);
      onDismissed();
    });
</script>

<div class="animate-rise flex flex-wrap items-center gap-2 rounded-2xl border border-amber-500/40 bg-card px-4 py-2.5 text-sm" role="status" data-testid="compress-banner">
  <span class="font-medium">compress needs pig credits</span>
  <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">Messages wait until compress succeeds or this banner is dismissed.</span>
  <Button size="sm" disabled={busy} onclick={retry}>Retry</Button>
  <Button variant="outline" size="sm" onclick={() => onChangeModel()}>Change compress model</Button>
  <Button variant="ghost" size="sm" disabled={busy} onclick={dismiss}>Dismiss</Button>
  {#if error}<p role="alert" class="w-full text-xs text-destructive">{error}</p>{/if}
</div>
