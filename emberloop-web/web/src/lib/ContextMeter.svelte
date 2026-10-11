<!-- Composer footer: model name plus a context-window ring. -->
<script>
  import { Shrink } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  // onCompress is set for Sessions that can compress. busy disables it. compressNote, when set, disables it too and gives the reason.
  let { model = '', usage = null, onCompress = null, busy = false, compressing = false, compressNote = '' } = $props();
  const R = 7;
  const C = 2 * Math.PI * R;
  const pct = $derived(usage ? Math.min(100, Math.round((usage.used / usage.size) * 100)) : 0);
  const fmt = (n) => (n >= 1000 ? `${(n / 1000).toFixed(n >= 100000 ? 0 : 1)}k` : String(n));
  const tone = $derived(pct >= 90 ? 'text-destructive' : pct >= 70 ? 'text-amber-300' : 'text-foreground');
</script>

<div class="flex items-center justify-between gap-3 px-3 text-xs text-muted-foreground" data-testid="context-meter">
  <span class="truncate">{model}</span>
  {#if onCompress}
    <Button variant="ghost" size="sm" class="ml-auto h-6 gap-1 px-2 text-xs" disabled={busy || compressing || !!compressNote} onclick={onCompress} title={compressNote || (busy ? 'Compress when the Session is idle' : 'Summarize older history to free context')}>
      <Shrink class="size-3.5" />{compressing ? 'Compressing…' : 'Compress'}
    </Button>
    {#if compressNote}<span class="truncate text-amber-300">{compressNote}</span>{/if}
  {/if}
  {#if usage}
    <span class="flex items-center gap-1.5 {tone}" title="{usage.used} of {usage.size} tokens">
      <svg viewBox="0 0 20 20" class="size-4 -rotate-90" aria-hidden="true">
        <circle cx="10" cy="10" r={R} fill="none" stroke="currentColor" stroke-opacity="0.2" stroke-width="2.5" />
        <circle
          cx="10" cy="10" r={R} fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"
          stroke-dasharray={C} stroke-dashoffset={C * (1 - pct / 100)}
          style="transition: stroke-dashoffset 0.6s cubic-bezier(0.22, 1, 0.36, 1)"
        />
      </svg>
      {fmt(usage.used)} / {fmt(usage.size)} · {pct}%
    </span>
  {:else}
    <span>Context: no usage yet</span>
  {/if}
</div>
