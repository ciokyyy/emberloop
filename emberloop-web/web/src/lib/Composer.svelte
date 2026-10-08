<!-- Floating composer. The mode badge always states what a send will do. -->
<script>
  import { MODES, availableModes, resolveComposerMode } from './composer.js';
  import Button from './ui/button.svelte';

  let { status = 'idle', busy = false, onSend = () => {} } = $props();
  let text = $state('');
  let selected = $state('steer');
  let sendError = $state('');

  const mode = $derived(resolveComposerMode({ status, selected }));
  const options = $derived(availableModes(status));
  const label = (id) => MODES.find((m) => m.id === id)?.label ?? id;

  async function send(ev) {
    ev.preventDefault();
    const t = text.trim();
    if (!t || busy) return;
    try {
      await onSend(t, mode);
      text = '';
      sendError = '';
    } catch {
      // Keep the draft: the prompt never left the browser.
      sendError = 'Send failed. Your text is kept above.';
    }
  }
</script>

<form onsubmit={send} class="rounded-2xl border bg-card p-2 shadow-lg" aria-label="composer">
  <div class="flex items-center gap-1 px-2 pt-1">
    {#each options as opt (opt.id)}
      {#if options.length > 1}
        <button
          type="button"
          aria-pressed={selected === opt.id}
          onclick={() => (selected = opt.id)}
          class="rounded-full px-2.5 py-0.5 text-xs font-medium"
          class:bg-primary={selected === opt.id}
          class:text-primary-foreground={selected === opt.id}
          class:text-muted-foreground={selected !== opt.id}
        >
          {opt.label}
        </button>
      {/if}
    {/each}
    <span class="ml-auto rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground" data-testid="composer-mode">
      Sends as {label(mode)}
    </span>
  </div>
  <div class="flex items-end gap-2 p-1">
    <textarea
      aria-label="message"
      rows="2"
      bind:value={text}
      placeholder={status === 'working' ? 'Steer or follow up…' : 'Send a prompt…'}
      class="max-h-40 flex-1 resize-none bg-transparent px-2 py-1 text-sm outline-none placeholder:text-muted-foreground"
    ></textarea>
    <Button type="submit" disabled={busy || !text.trim()}>Send</Button>
  </div>
  {#if sendError}
    <p role="alert" class="px-2 pb-1 text-xs text-red-600 dark:text-red-400">{sendError}</p>
  {/if}
</form>
