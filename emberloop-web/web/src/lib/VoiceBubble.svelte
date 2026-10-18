<!-- Voice message bubble: play, waveform with seek, speed, transcript. -->
<script>
  import { Play, Pause, FileText, X } from '@lucide/svelte';
  import { flatPeaks, fmtDuration } from './voiceMeta.js';

  let {
    src = '',
    peaks = [],
    durationMs = 0,
    transcript = '',
    onRemove = null,
    // The draft in the composer edits its transcript in the main textarea.
    transcriptEditable = false,
  } = $props();

  const bars = $derived(peaks?.length ? peaks : flatPeaks());
  let audio = $state();
  let playing = $state(false);
  let pos = $state(0); // seconds
  let length = $state(0); // seconds, from the element once known
  let rate = $state(1);
  let open = $state(false);

  const total = $derived(length > 0 ? length : durationMs / 1000);
  const frac = $derived(total > 0 ? Math.min(1, pos / total) : 0);

  function toggle() {
    if (!audio) return;
    if (audio.paused) void audio.play().catch(() => {});
    else audio.pause();
  }
  function seek(ev) {
    if (!audio || !(total > 0)) return;
    const r = ev.currentTarget.getBoundingClientRect();
    const f = Math.min(1, Math.max(0, (ev.clientX - r.left) / r.width));
    audio.currentTime = f * total;
    pos = audio.currentTime;
  }
  function cycleRate() {
    rate = rate === 1 ? 1.5 : rate === 1.5 ? 2 : 1;
    if (audio) audio.playbackRate = rate;
  }
</script>

<div class="flex w-64 max-w-full flex-col gap-1.5" data-testid="voice-bubble" title={transcript || undefined}>
  <div class="flex items-center gap-2">
    <button
      type="button"
      aria-label={playing ? 'pause voice message' : 'play voice message'}
      onclick={toggle}
      class="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground transition-transform hover:scale-105 active:scale-95 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
    >
      {#if playing}<Pause class="size-4" />{:else}<Play class="size-4" />{/if}
    </button>
    <!-- svelte-ignore a11y_click_events_have_key_events -->
    <div
      role="slider"
      tabindex="0"
      aria-label="seek voice message"
      aria-valuemin="0"
      aria-valuemax={Math.round(total)}
      aria-valuenow={Math.round(pos)}
      onclick={seek}
      onkeydown={(e) => {
        if (!audio || !(total > 0)) return;
        if (e.key === 'ArrowRight') audio.currentTime = Math.min(total, audio.currentTime + 5);
        if (e.key === 'ArrowLeft') audio.currentTime = Math.max(0, audio.currentTime - 5);
      }}
      class="flex h-8 min-w-0 flex-1 cursor-pointer items-center gap-px"
    >
      {#each bars as h, i (i)}
        <span
          class="w-[3px] shrink-0 rounded-full transition-colors motion-standard {i / bars.length < frac ? 'bg-primary' : 'bg-foreground/25'}"
          style="height: {Math.max(8, Math.min(100, h))}%"
        ></span>
      {/each}
    </div>
    <span class="w-9 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground">{fmtDuration((playing || pos > 0 ? pos : total) * 1000)}</span>
  </div>
  <div class="flex items-center gap-1 pl-10">
    <button
      type="button"
      aria-label="playback speed"
      onclick={cycleRate}
      class="rounded-full px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
    >{rate}x</button>
    <button
      type="button"
      aria-label="transcript"
      aria-pressed={open}
      onclick={() => (open = !open)}
      class="flex items-center gap-1 rounded-full px-1.5 py-0.5 text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
    ><FileText class="size-3" />Transcript</button>
    {#if onRemove}
      <button
        type="button"
        aria-label="discard recording"
        onclick={onRemove}
        class="ml-auto rounded-full p-1 text-muted-foreground transition-colors hover:text-foreground"
      ><X class="size-3.5" /></button>
    {/if}
  </div>
  {#if open}
    <p class="animate-rise text-sm leading-snug whitespace-pre-wrap [overflow-wrap:anywhere]" data-testid="voice-transcript">
      {#if transcript}{transcript}{:else if transcriptEditable}<span class="text-muted-foreground">Edit the transcript in the message box.</span>{:else}<span class="text-muted-foreground">No transcript.</span>{/if}
    </p>
  {/if}
  {#if src}
    <audio
      bind:this={audio}
      {src}
      preload="metadata"
      onplay={() => (playing = true)}
      onpause={() => (playing = false)}
      onended={() => {
        playing = false;
        pos = 0;
      }}
      ontimeupdate={() => (pos = audio.currentTime)}
      onloadedmetadata={() => {
        if (Number.isFinite(audio.duration)) length = audio.duration;
      }}
    ></audio>
  {/if}
</div>
