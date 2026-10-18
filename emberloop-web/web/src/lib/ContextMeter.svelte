<!-- Context-window ring for the chat header. A tap opens a detail sheet: model,
     tokens and Compress. Bottom sheet on phone, anchored panel on desktop. -->
<script>
  import { Dialog } from 'bits-ui';
  import { Loader } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import IconButton from './ui/icon-button.svelte';
  import Tooltip from './ui/tooltip.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import { icons } from './icons.js';
  import { holdBack } from './overlayBack.js';
  import { isPhone } from './viewport.js';

  // onCompress is set for Sessions that can compress. busy disables it. compressNote, when set, disables it too and gives the reason.
  // `open` is bindable so the phone Session menu can open the same sheet.
  let { model = '', usage = null, onCompress = null, busy = false, compressing = false, compressNote = '', open = $bindable(false) } = $props();

  // System Back closes the detail sheet on phones; the desktop panel keeps the
  // router's Back behaviour.
  $effect(() => {
    if (!open || !isPhone()) return;
    return holdBack(() => (open = false));
  });
  const R = 7;
  const C = 2 * Math.PI * R;
  const pct = $derived(usage ? Math.min(100, Math.round((usage.used / usage.size) * 100)) : 0);
  const fmt = (n) => (n >= 1e6 ? `${+(n / 1e6).toFixed(1)}M` : n >= 1000 ? `${+(n / 1000).toFixed(n >= 1e5 ? 0 : 1)}k` : String(n));
  const tone = $derived(pct >= 90 ? 'text-status-failed' : pct >= 70 ? 'text-status-needs-you' : 'text-fg-1');
  const name = $derived(usage ? `Context ${pct}% used` : 'Context: no usage yet');
  const compressOff = $derived(busy || compressing || !!compressNote);
  const compressHint = $derived(compressNote || (compressing ? 'Compressing…' : busy ? 'Compress when the Session is idle' : 'Summarize older history to free context'));
</script>

{#snippet ring(size)}
  <svg viewBox="0 0 20 20" class="{size} -rotate-90" aria-hidden="true">
    <circle cx="10" cy="10" r={R} fill="none" stroke="currentColor" stroke-opacity="0.2" stroke-width="2.5" />
    {#if usage}
      <circle
        cx="10" cy="10" r={R} fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"
        stroke-dasharray={C} stroke-dashoffset={C * (1 - pct / 100)}
        class="motion-emphasis transition-[stroke-dashoffset]"
      />
    {/if}
  </svg>
{/snippet}

<Tooltip text={name} side="bottom">
  {#snippet trigger(props)}
    <Button {...props} variant="ghost" size="icon-sm" aria-label={name} aria-haspopup="dialog" data-testid="context-meter" class="{tone} [&_svg]:size-5" onclick={() => (open = true)}>
      {@render ring('size-5')}
    </Button>
  {/snippet}
</Tooltip>

<Dialog.Root bind:open>
  <Dialog.Portal>
    <Dialog.Overlay class="motion-emphasis fixed inset-0 z-50 bg-black/50 backdrop-blur-sm data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 md:bg-transparent md:backdrop-blur-none" />
    <Dialog.Content
      data-testid="context-sheet"
      class="motion-emphasis fixed z-50 flex flex-col gap-4 border bg-surface-2 p-4 text-fg-1 shadow-elev-3 outline-none data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:animate-in data-[state=open]:fade-in-0 max-md:inset-x-0 max-md:bottom-[var(--kb-inset)] max-md:rounded-t-2xl max-md:pb-[max(1rem,var(--safe-bottom))] max-md:data-[state=closed]:slide-out-to-bottom max-md:data-[state=open]:slide-in-from-bottom md:right-6 md:top-16 md:w-80 md:rounded-2xl"
    >
      <div class="flex items-center gap-2">
        <Dialog.Title class="min-w-0 flex-1 truncate text-title">Context window</Dialog.Title>
        <Dialog.Close>
          {#snippet child({ props })}
            <IconButton {...props} icon={icons.dismiss} label="Close" />
          {/snippet}
        </Dialog.Close>
      </div>
      <Dialog.Description class="sr-only">Model, tokens used and Compress for this Session.</Dialog.Description>
      <div class="flex items-center gap-4 {tone}">
        {@render ring('size-14')}
        <div class="flex min-w-0 flex-col gap-1">
          {#if usage}
            <span class="text-numeric" data-testid="context-pct">{pct}%</span>
            <span class="truncate text-caption tabular-nums text-fg-3" data-testid="context-tokens">{fmt(usage.used)} of {fmt(usage.size)} tokens</span>
          {:else}
            <EmptyState compact icon={icons.quota} title="No usage yet" />
          {/if}
        </div>
      </div>
      <div class="flex min-h-11 items-center gap-3 border-t pt-3 text-body">
        <icons.model class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate" data-testid="context-model">{model || 'Unknown model'}</span>
      </div>
      {#if onCompress}
        <div class="flex min-h-11 items-center gap-3 border-t pt-3">
          <p class="min-w-0 flex-1 text-caption {compressNote ? 'text-status-needs-you' : 'text-fg-3'}" data-testid="compress-hint">{compressHint}</p>
          <IconButton
            icon={compressing ? Loader : icons.compress}
            spin={compressing}
            label="Compress"
            variant="outline"
            disabled={compressOff}
            onclick={() => {
              open = false;
              onCompress();
            }}
          />
        </div>
      {/if}
    </Dialog.Content>
  </Dialog.Portal>
</Dialog.Root>
