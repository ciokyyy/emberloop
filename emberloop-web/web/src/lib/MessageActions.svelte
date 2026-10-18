<script>
  import Button from './ui/button.svelte';
  import { copyText, dispatchComposerEdit, markdownToPlainText, shareMessage } from './composer.js';
  import { editQueueItem, removeQueueItem } from './api.js';
  import ErrorState from './ui/error-state.svelte';

  let {
    block,
    sessionId = null,
    canEdit = false,
    queuedItem = null,
    open = false,
    onOpenChange = () => {},
    onQueueChange = () => {},
  } = $props();

  let busy = $state(false);
  let rootEl = null;
  let editing = $state(false);
  let queueDraft = $state('');
  let error = $state('');
  let notice = $state('');
  const text = $derived(String(queuedItem?.text ?? block?.body ?? ''));
  const messageLabel = $derived(block?.title || (queuedItem ? 'queued message' : 'message'));

  function changeOpen(value) {
    onOpenChange(value);
    if (!value) error = '';
  }

  async function copy(format) {
    error = '';
    notice = '';
    try {
      await copyText(format === 'markdown' ? text : markdownToPlainText(text));
      notice = format === 'markdown' ? 'Copied as Markdown.' : 'Copied as text.';
      changeOpen(false);
    } catch (e) {
      error = e?.message || 'Could not copy this message.';
    }
  }

  async function share() {
    error = '';
    notice = '';
    try {
      const result = await shareMessage(markdownToPlainText(text));
      notice = result === 'shared' ? 'Shared.' : 'Copied for sharing.';
      changeOpen(false);
    } catch (e) {
      if (e?.name === 'AbortError') {
        changeOpen(false);
        return;
      }
      error = e?.message || 'Could not share this message.';
    }
  }

  function editAndResend() {
    if (!sessionId) {
      error = 'This message is not attached to an open Session.';
      return;
    }
    dispatchComposerEdit({
      sessionId,
      text,
      imageIds: block?.imageIds ?? [],
    });
    notice = '';
    changeOpen(false);
  }

  function beginQueueEdit() {
    queueDraft = String(queuedItem?.text ?? '');
    error = '';
    editing = true;
    changeOpen(false);
  }

  async function saveQueueEdit(event) {
    event.preventDefault();
    if (!sessionId || !queuedItem?.id) return;
    busy = true;
    error = '';
    try {
      await editQueueItem(sessionId, queuedItem.id, queueDraft);
      onQueueChange({ type: 'edit', id: queuedItem.id, text: queueDraft });
      notice = 'Queued message updated.';
      editing = false;
    } catch (e) {
      onQueueChange({ type: 'refresh' });
      error = e?.message || 'Could not edit this queued message.';
    } finally {
      busy = false;
    }
  }

  async function cancelQueued() {
    if (!sessionId || !queuedItem?.id) return;
    busy = true;
    error = '';
    try {
      await removeQueueItem(sessionId, queuedItem.id);
      onQueueChange({ type: 'remove', id: queuedItem.id });
      notice = 'Queued message canceled.';
      editing = false;
      changeOpen(false);
    } catch (e) {
      onQueueChange({ type: 'refresh' });
      error = e?.message || 'Could not cancel this queued message.';
    } finally {
      busy = false;
    }
  }
  $effect(() => {
    if (!open || typeof window === 'undefined') return;
    const onKey = (event) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        changeOpen(false);
        rootEl?.querySelector('button[aria-haspopup]')?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  });

</script>

<div bind:this={rootEl} class="mt-1 flex flex-col items-end" data-testid="message-actions">
  <Button
    variant="ghost"
    size="sm"
    aria-label={`Message actions for ${messageLabel}`}
    aria-haspopup="true"
    aria-expanded={open}
    title="Message actions"
    onclick={() => changeOpen(!open)}
    class="md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100"
  >
    Actions
  </Button>
  {#if open}
    <div class="flex flex-wrap justify-end gap-1 rounded-xl border bg-popover p-2 shadow-elev-2" role="group" aria-label={`Actions for ${messageLabel}`} data-testid="message-action-menu">
      <Button variant="ghost" size="sm" onclick={() => copy('text')}>Copy text</Button>
      <Button variant="ghost" size="sm" onclick={() => copy('markdown')}>Copy Markdown</Button>
      <Button variant="ghost" size="sm" onclick={share}>Share</Button>
      {#if canEdit && !queuedItem}
        <Button variant="ghost" size="sm" onclick={editAndResend}>Edit and resend</Button>
      {/if}
      {#if queuedItem}
        <Button variant="ghost" size="sm" onclick={beginQueueEdit}>Edit</Button>
        <Button size="sm" variant="destructive" disabled={busy} onclick={cancelQueued}>Cancel</Button>
      {/if}
    </div>
  {/if}
  {#if editing}
    <form class="mt-1 flex w-full max-w-xl flex-col gap-2 rounded-xl border bg-card p-2" onsubmit={saveQueueEdit} data-testid="queued-message-edit">
      <label class="flex flex-col gap-1 text-xs text-muted-foreground">
        Edit queued message
        <textarea bind:value={queueDraft} rows="3" class="min-h-20 w-full resize-y rounded-lg border bg-background px-3 py-2 text-sm text-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40"></textarea>
      </label>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" disabled={busy} onclick={() => (editing = false)}>Keep queued</Button>
        <Button type="submit" size="sm" disabled={busy}>Save</Button>
      </div>
    </form>
  {/if}
  {#if error}<ErrorState compact title={error} class="mt-1" />{/if}
  {#if notice}<p role="status" class="mt-1 text-right text-xs text-muted-foreground">{notice}</p>{/if}
</div>
