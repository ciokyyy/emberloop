<!-- Floating composer. The mode badge always states what a send will do. -->
<script>
  import { MODES, availableModes, resolveComposerMode } from './composer.js';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';

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

  function key(ev) {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) send(ev);
  }
</script>

<form
  onsubmit={send}
  class="animate-rise rounded-3xl border bg-muted/80 p-2 shadow-[0_8px_30px_rgb(0_0_0/0.12)] backdrop-blur-xl transition-shadow focus-within:shadow-[0_8px_40px_rgb(0_0_0/0.2)]"
  aria-label="composer"
>
  <div class="flex items-center gap-1 px-2 pt-1">
    {#if options.length > 1}
      <div class="flex gap-0.5 rounded-full bg-background/60 p-0.5">
        {#each options as opt (opt.id)}
          <button
            type="button"
            aria-pressed={selected === opt.id}
            onclick={() => (selected = opt.id)}
            class="rounded-full px-3 py-1 text-xs font-medium transition-colors {selected === opt.id
              ? 'bg-primary text-primary-foreground'
              : 'text-muted-foreground hover:text-foreground'}"
          >
            {opt.label}
          </button>
        {/each}
      </div>
    {/if}
    <Badge class="ml-auto bg-transparent" data-testid="composer-mode">Sends as {label(mode)}</Badge>
  </div>
  <div class="flex items-end gap-2 p-1">
    <textarea
      aria-label="message"
      rows="2"
      bind:value={text}
      onkeydown={key}
      placeholder={status === 'working' ? 'Steer or follow up…' : 'Send a prompt…'}
      class="max-h-40 flex-1 resize-none bg-transparent px-3 py-2 text-[15px] outline-none placeholder:text-muted-foreground"
    ></textarea>
    <Button type="submit" size="default" disabled={busy || !text.trim()}>Send</Button>
  </div>
  {#if sendError}
    <p role="alert" class="px-3 pb-1 text-xs text-destructive">{sendError}</p>
  {/if}
</form>
