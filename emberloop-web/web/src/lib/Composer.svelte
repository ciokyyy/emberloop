<!-- Floating composer. The mode badge always states what a send will do. -->
<script>
  import { onDestroy } from 'svelte';
  import WorkingIndicator from './WorkingIndicator.svelte';
  import { MODES, availableModes, resolveComposerMode } from './composer.js';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import { Mic, Square, Loader, Paperclip, X } from '@lucide/svelte';
  import { sttSupport, startLiveRecording, transcribe, STT_MAX_BYTES } from './stt.js';
  import { readImages, toWire, IMAGE_TYPES } from './images.js';

  let { status = 'idle', busy = false, onSend = () => {}, onStop = null } = $props();
  let text = $state('');
  let selected = $state('steer');
  let sendError = $state('');
  // Attached images: { mime, data, name }. They ride with the next send.
  let images = $state([]);
  let imageNote = $state('');
  let picker;
  // Voice input: 'idle' | 'recording' | 'transcribing'. While recording the
  // mic streams segments: each closed segment is transcribed as it lands
  // and appended to the draft in seq order, so text appears while the user
  // still speaks. liveSegs previews them (pending dimmed); the flush index
  // holds back out-of-order answers so the draft stays ordered.
  let voice = $state('idle');
  let voiceNote = $state('');
  let liveSegs = $state([]);
  let voiceFormat = $state(true);
  let liveRec = null;
  let segFlush = 0;
  let segTasks = [];
  let voiceGotText = false;
  const support = sttSupport();

  onDestroy(() => {
    try {
      liveRec?.cancel();
    } catch {
      // Already stopped.
    }
  });
  const mode = $derived(resolveComposerMode({ status, selected }));
  const options = $derived(availableModes(status));
  const label = (id) => MODES.find((m) => m.id === id)?.label ?? id;
  const canSend = $derived(!busy && (text.trim() !== '' || images.length > 0));

  async function send(ev) {
    ev.preventDefault();
    const t = text.trim();
    const sent = images;
    if ((!t && sent.length === 0) || busy) return;
    // Clear at once: the transcript shows the message while it travels.
    // A failed send puts the draft and the images back.
    text = '';
    images = [];
    sendError = '';
    imageNote = '';
    try {
      await onSend(t, mode, toWire(sent));
    } catch {
      text = t;
      images = sent;
      sendError = 'Send failed. Your message is kept above.';
    }
  }

  async function addFiles(files) {
    const out = await readImages(files, images);
    images = out.images;
    imageNote = out.error;
  }
  function picked(ev) {
    addFiles([...ev.currentTarget.files]);
    ev.currentTarget.value = '';
  }
  function paste(ev) {
    const files = [...(ev.clipboardData?.files ?? [])].filter((f) => f.type.startsWith('image/'));
    if (files.length) {
      ev.preventDefault();
      addFiles(files);
    }
  }

  // Appends finalized segment text to the live draft. It reads the current
  // draft at flush time, so text typed mid-recording is kept, never clobbered.
  function liveAppend(out) {
    if (!out) return;
    text = text ? `${text} ${out}` : out;
    voiceGotText = true;
  }

  // Flushes finalized segments in seq order; a failed part is skipped so a
  // slow or failed segment never blocks later ones.
  function flushSegs() {
    while (segFlush < liveSegs.length && liveSegs[segFlush].state !== 'pending') {
      const s = liveSegs[segFlush];
      if (s.state === 'done') liveAppend(s.text);
      segFlush++;
    }
  }

  // One closed segment: transcribe it at once so answers land while the user
  // still speaks. Out-of-order answers wait in liveSegs for the flush.
  function onLiveSegment(blob, seq) {
    liveSegs[seq] = { seq, state: 'pending', text: '' };
    const task = (async () => {
      try {
        if (blob.size > STT_MAX_BYTES) {
          liveSegs[seq] = { seq, state: 'error', text: '' };
          voiceNote = 'A voice part topped the 25 MB cap and was skipped.';
          return;
        }
        const out = await transcribe(blob, { format: voiceFormat });
        liveSegs[seq] = { seq, state: 'done', text: out };
      } catch (e) {
        liveSegs[seq] = { seq, state: 'error', text: '' };
        voiceNote = `Voice part ${seq + 1} failed (${e.message}); the rest is kept.`;
      } finally {
        flushSegs();
      }
    })();
    segTasks.push(task);
  }

  // Mic button: first click records in live segments, second click stops,
  // flushes the tail and waits for every part. Nothing is sent until the
  // user sends it.
  async function toggleMic() {
    voiceNote = '';
    if (voice === 'recording') {
      const rec = liveRec;
      liveRec = null;
      voice = 'transcribing';
      try {
        await rec.stop();
        await Promise.allSettled(segTasks);
        flushSegs();
        if (!voiceGotText && !voiceNote) voiceNote = 'No speech heard.';
      } catch (e) {
        voiceNote = `Voice failed: ${e.message}`;
      } finally {
        voice = 'idle';
      }
      return;
    }
    if (voice !== 'idle') return;
    liveSegs = [];
    segFlush = 0;
    segTasks = [];
    voiceGotText = false;
    try {
      liveRec = await startLiveRecording({ onSegment: onLiveSegment });
      voice = 'recording';
    } catch {
      liveRec = null;
      voiceNote = 'Mic blocked or missing. Allow it in the browser.';
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
  {#if options.length <= 1}
    <WorkingIndicator {status} />
  {/if}
  {#if options.length > 1}
    <div class="flex items-center gap-1 px-2 pt-1">
      <div class="flex gap-0.5 rounded-full bg-background/60 p-0.5" role="group" aria-label="Send mode">
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
      <WorkingIndicator {status} class="ml-auto px-1" />
    </div>
  {/if}
  {#if images.length}
    <ul class="flex flex-wrap gap-2 px-3 pt-2" aria-label="attached images">
      {#each images as img, i (i)}
        <li class="relative">
          <img src={`data:${img.mime};base64,${img.data}`} alt={img.name} class="size-14 rounded-lg border object-cover" />
          <button
            type="button"
            aria-label={`remove ${img.name}`}
            onclick={() => (images = images.filter((_, j) => j !== i))}
            class="absolute -right-1.5 -top-1.5 rounded-full border bg-background p-0.5 text-muted-foreground hover:text-foreground"
          >
            <X class="size-3" />
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="flex items-end gap-2 p-1">
    <div class="relative min-w-0 flex-1">
      <textarea
        aria-label="message"
        rows="2"
        bind:value={text}
        onkeydown={key}
        onpaste={paste}
        placeholder={status === 'working' ? 'Steer or follow up…' : 'Send a prompt…'}
        class="max-h-40 w-full resize-none bg-transparent px-3 pb-5 pt-2 text-[15px] outline-none placeholder:text-muted-foreground"
      ></textarea>
      <Badge data-testid="composer-mode" aria-live="polite" variant="secondary" class="pointer-events-none absolute bottom-1 right-2 border-transparent bg-transparent px-1.5 py-0 text-[10px] font-normal text-muted-foreground/80 transition-colors {options.length > 1 ? '' : 'opacity-70'}">Sends as {label(mode)}</Badge>
    </div>
    <input
      bind:this={picker}
      type="file"
      accept={IMAGE_TYPES.join(',')}
      multiple
      hidden
      aria-label="image files"
      onchange={picked}
    />
    <Button type="button" variant="ghost" size="icon" aria-label="attach image" title="Attach image" onclick={() => picker.click()}>
      <Paperclip />
    </Button>
    {#if support.ok}
      <Button
        type="button"
        variant={voice === 'recording' ? 'destructive' : 'ghost'}
        size="icon"
        aria-label={voice === 'recording' ? 'stop recording' : 'record voice'}
        title="Voice input (live transcription via OpenRouter)"
        disabled={voice === 'transcribing'}
        onclick={toggleMic}
      >
        {#if voice === 'recording'}
          <Square class="animate-pulse" />
        {:else if voice === 'transcribing'}
          <Loader class="animate-spin" />
        {:else}
          <Mic />
        {/if}
      </Button>
    {/if}
    {#if onStop && status === 'working'}
      <Button type="button" variant="destructive" size="icon" aria-label="stop" title="Stop the running turn" onclick={onStop}>
        <Square />
      </Button>
    {/if}
    <Button type="submit" size="default" disabled={!canSend}>Send</Button>
  </div>
  {#if sendError}
    <p role="alert" class="px-3 pb-1 text-xs text-destructive">{sendError}</p>
  {/if}
  {#if imageNote}
    <p data-testid="image-note" class="px-3 pb-1 text-xs text-destructive">{imageNote}</p>
  {/if}
  {#if voice !== 'idle' && liveSegs.length}
    <p data-testid="voice-live" class="flex flex-wrap items-center gap-x-1 px-3 pb-1 text-xs text-muted-foreground">
      <span>Listening…</span>
      {#each liveSegs as s, i (i)}
        {#if s && s.state === 'pending'}
          <span class="animate-pulse opacity-50 transition-opacity duration-300">…</span>
        {:else if s && s.text}
          <span>{s.text}</span>
        {/if}
      {/each}
      <button
        type="button"
        aria-pressed={voiceFormat}
        title="Clean up each voice part (punctuation, filler words) with a small cloud model"
        onclick={() => (voiceFormat = !voiceFormat)}
        class="ml-auto rounded-full px-2 py-0.5 font-medium transition-colors duration-300 {voiceFormat
          ? 'text-foreground'
          : 'opacity-50 hover:opacity-80'}"
      >
        Cleanup {voiceFormat ? 'on' : 'off'}
      </button>
    </p>
  {/if}
  {#if voiceNote || !support.ok}
    <p data-testid="voice-note" class="px-3 pb-1 text-xs text-muted-foreground">{voiceNote || support.reason}</p>
  {/if}
</form>
