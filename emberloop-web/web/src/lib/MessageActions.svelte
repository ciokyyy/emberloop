<!-- Per-message actions. Desktop: the trigger sits in the hover toolbar and
     opens an anchored icon toolbar with tooltips. Phone: the trigger stays
     visible and the same group opens as a bottom sheet with 44 px targets. -->
<script>
  import { Ellipsis } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import IconButton from './ui/icon-button.svelte';
  import Tooltip from './ui/tooltip.svelte';
  import { icons } from './icons.js';
  import { cn } from './ui/cn.js';
  import { copyText, dispatchComposerEdit, markdownToPlainText, shareMessage } from './composer.js';
  import { editQueueItem, removeQueueItem } from './api.js';
  import ErrorState from './ui/error-state.svelte';
  import { holdBack } from './overlayBack.js';
  import { isPhone } from './viewport.js';

  let {
    block,
    sessionId = null,
    canEdit = false,
    queuedItem = null,
    open = false,
    onOpenChange = () => {},
    onQueueChange = () => {},
    align = 'end',
  } = $props();

  let busy = $state(false);
  let rootEl = $state(null);
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

  // System Back closes the open action sheet on phones; the desktop popover
  // keeps the router's Back behaviour.
  $effect(() => {
    if (!open || !isPhone()) return;
    return holdBack(() => changeOpen(false));
  });
</script>

{#snippet action(icon, label, run, destructive = false, disabled = false)}
  {@const Icon = icon}
  <Tooltip text={label}>
    {#snippet trigger(props)}
      <Button
        {...props}
        variant="ghost"
        size="icon-sm"
        aria-label={label}
        disabled={disabled}
        onclick={run}
        class={cn(
          'max-md:h-auto max-md:w-full max-md:min-h-11 max-md:flex-col max-md:gap-1 max-md:rounded-lg max-md:px-2 max-md:py-2 md:size-9',
          destructive && 'text-destructive hover:bg-destructive/10 hover:text-destructive',
        )}
      >
        <Icon aria-hidden="true" />
        <span class="hidden text-caption max-md:block">{label}</span>
      </Button>
    {/snippet}
  </Tooltip>
{/snippet}

<div bind:this={rootEl} class={cn('relative flex flex-col', align === 'start' ? 'items-start' : 'items-end')} data-testid="message-actions">
  <IconButton
    icon={Ellipsis}
    label={`Message actions for ${messageLabel}`}
    aria-haspopup="true"
    aria-expanded={open}
    onclick={() => changeOpen(!open)}
    class="[@media(hover:hover)]:md:opacity-0 [@media(hover:hover)]:md:group-focus-within:opacity-100 [@media(hover:hover)]:md:group-hover:opacity-100 [@media(hover:hover)]:md:focus-visible:opacity-100"
  />
  {#if open}
    <div class="pointer-events-none fixed inset-0 z-40 bg-black/40 md:hidden" aria-hidden="true"></div>
    <div
      role="group"
      aria-label={`Actions for ${messageLabel}`}
      data-testid="message-action-menu"
      class={cn(
        'z-50 rounded-xl border bg-surface-2 p-1.5 text-fg-1 shadow-elev-2',
        'max-md:fixed max-md:inset-x-0 max-md:bottom-[var(--kb-inset)] max-md:z-[60] max-md:rounded-b-none max-md:rounded-t-2xl max-md:p-3 max-md:pb-[max(0.75rem,var(--safe-bottom))] max-md:shadow-elev-3',
        'md:absolute md:top-full md:mt-1.5 md:w-max',
        align === 'start' ? 'md:left-0' : 'md:right-0',
      )}
    >
      <div class="mb-2 flex items-center gap-2 border-b pb-2 md:hidden">
        <span class="min-w-0 flex-1 truncate text-title">{messageLabel}</span>
        <IconButton icon={icons.dismiss} label="Close actions" onclick={() => changeOpen(false)} />
      </div>
      <div class="flex flex-wrap items-center gap-1 max-md:grid max-md:grid-cols-2 max-md:gap-2">
        {@render action(icons.copy, 'Copy text', () => copy('text'))}
        {@render action(icons.copy, 'Copy Markdown', () => copy('markdown'))}
        {@render action(icons.share, 'Share', share)}
        {#if canEdit && !queuedItem}
          {@render action(icons.edit, 'Edit and resend', editAndResend)}
        {/if}
        {#if queuedItem}
          {@render action(icons.edit, 'Edit', beginQueueEdit)}
          {@render action(icons.cancel, 'Cancel', cancelQueued, true, busy)}
        {/if}
      </div>
    </div>
  {/if}
  {#if editing}
    <form class="mt-1 flex w-full max-w-xl flex-col gap-2 rounded-xl border bg-surface-2 p-2 shadow-elev-2" onsubmit={saveQueueEdit} data-testid="queued-message-edit">
      <label class="flex flex-col gap-1 text-caption text-fg-2">
        Edit queued message
        <textarea bind:value={queueDraft} rows="3" class="min-h-20 w-full resize-y rounded-lg border bg-surface-0 px-3 py-2 text-body text-fg-1 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40"></textarea>
      </label>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="ghost" size="sm" disabled={busy} onclick={() => (editing = false)}>Keep queued</Button>
        <Button type="submit" size="sm" disabled={busy}>Save</Button>
      </div>
    </form>
  {/if}
  {#if error}<ErrorState compact title={error} class="mt-1" />{/if}
  {#if notice}<p role="status" class="mt-1 text-right text-caption text-fg-3">{notice}</p>{/if}
</div>
