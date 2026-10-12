<!-- Floating composer. The send-mode buttons state what a send will do. -->
<script>
  import { onDestroy } from 'svelte';
  import WorkingIndicator from './WorkingIndicator.svelte';
  import { availableModes, resolveComposerMode } from './composer.js';
  import Button from './ui/button.svelte';
  import { BookmarkPlus, Mic, Square, Loader, Paperclip, X, Images } from '@lucide/svelte';
  import { sttSupport, startRecording, transcribe, STT_MAX_BYTES } from './stt.js';
  import VoiceBubble from './VoiceBubble.svelte';
  import { readVoicePrefs, onVoicePrefs } from './voicePrefs.js';
  import { fmtDuration, isAudioId } from './voiceMeta.js';
  import { readImages, toWire, IMAGE_TYPES } from './images.js';
  import { listImages, imageSrc } from './api.js';
  import { soundPlayer } from './sound.js';

  // Voice ticks: a quiet sound when recording starts and stops. Message
  // sends stay silent.
  const sounds = soundPlayer();
  let voiceSeq = 0;

  let { status = 'idle', busy = false, sessionId = '', onSend = () => {}, onStop = null } = $props();

  // A fresh key per tick: start and stop never dedup each other. The
  // throttle still applies, so a stop inside 400ms of its start stays silent.
  const tick = (phase) => sounds?.playEvent('voice', { key: `voice:${sessionId || 'draft'}:${phase}:${++voiceSeq}` });
  let text = $state('');
  let selected = $state('steer');
  let sendError = $state('');
  // Attached images: { mime, data, name }. They ride with the next send.
  let images = $state([]);
  let imageNote = $state('');
  // Stored references: { id, name, url } into the Session's image gallery.
  // They ride the next send as stable ids; the agent reads their files.
  let refs = $state([]);
  let gallery = $state(false);
  let galleryImages = $state([]);
  let galleryLoadedFor = '';
  let lastSession = '';
  let picker;
  let box;
  // Voice input: 'idle' | 'recording' | 'transcribing'. A recording is one
  // take. On stop it uploads for transcription and the text lands in the
  // draft textarea, where the user edits it before sending. The recording
  // rides the send as a voice message; the model reads only the text.
  // On/off and cleanup live in Settings (voicePrefs.js).
  let voice = $state('idle');
  let voiceNote = $state('');
  let voicePrefs = $state(readVoicePrefs());
  const offPrefs = onVoicePrefs((p) => (voicePrefs = p));
  // Recent input levels (0..1) for the live waveform, and elapsed seconds.
  let levels = $state([]);
  let elapsedMs = $state(0);
  // The finished recording awaiting send: { blob, url, durationMs, peaks }.
  let audio = $state(null);
  let rec = null;
  let clock = null;
  let levelTick = 0;
  const support = sttSupport();

  function stopClock() {
    clearInterval(clock);
    clock = null;
  }
  function dropAudio() {
    if (audio?.url) {
      try {
        URL.revokeObjectURL(audio.url);
      } catch {
        // Nothing to revoke.
      }
    }
    audio = null;
  }

  onDestroy(() => {
    offPrefs();
    stopClock();
    try {
      rec?.cancel();
    } catch {
      // Already stopped.
    }
    dropAudio();
  });
  const mode = $derived(resolveComposerMode({ status, selected }));
  const workingModes = availableModes('working');
  const headerOpen = $derived(availableModes(status).length > 1);
  const canSend = $derived(!busy && voice !== 'transcribing' && (text.trim() !== '' || images.length > 0 || refs.length > 0));

  // A new Session starts with an empty tray: refs point at another
  // Session's gallery and would send unknown ids.
  $effect(() => {
    if (sessionId !== lastSession) {
      lastSession = sessionId;
      refs = [];
      gallery = false;
      galleryLoadedFor = '';
    }
  });

  // The gallery lists every image attached in this Session. Clicking one
  // inserts a reference into the tray, so a later message can re-reference
  // it by stable id without re-uploading bytes.
  async function toggleGallery() {
    gallery = !gallery;
    if (gallery && sessionId && galleryLoadedFor !== sessionId) {
      galleryLoadedFor = sessionId;
      try {
        const body = await listImages(sessionId);
        if (sessionId === galleryLoadedFor) galleryImages = (body.images ?? []).filter((i) => !isAudioId(i.id));
      } catch {
        galleryImages = [];
      }
    }
  }

  function referenceImage(img) {
    if (!img?.id || refs.some((r) => r.id === img.id)) return;
    refs = [...refs, { id: img.id, name: img.name ?? 'image', url: img.url || (sessionId ? imageSrc(sessionId, img.id) : '') }];
  }

  async function send(ev) {
    ev.preventDefault();
    const t = text.trim();
    const sent = images;
    const sentRefs = refs;
    const sentAudio = audio;
    if ((!t && sent.length === 0 && sentRefs.length === 0) || busy) return;
    // Clear at once: the transcript shows the message while it travels.
    // A failed send puts the draft, the images and the refs back.
    text = '';
    images = [];
    refs = [];
    audio = null;
    sendError = '';
    imageNote = '';
    try {
      const ids = sentRefs.map((r) => r.id);
      if (sentAudio) await onSend(t, mode, toWire(sent), ids, { blob: sentAudio.blob, durationMs: sentAudio.durationMs, peaks: sentAudio.peaks });
      else await onSend(t, mode, toWire(sent), ids);
      if (sentAudio?.url) URL.revokeObjectURL(sentAudio.url);
    } catch {
      text = t;
      images = sent;
      refs = sentRefs;
      audio = sentAudio;
      sendError = 'Send failed. Your message is kept above.';
    }
  }

  // Remember button: a structural /remember action that stores the draft as
  // a Rule directly after grounding. A draft sends at once with the prefix;
  // an empty draft gains the prefix for editing; a prefixed draft toggles
  // back to a normal message. Pasted "remember:" text never matches: only
  // the slash prefix on the first line acts.
  function remember(ev) {
    if (busy || voice === 'transcribing') return;
    if (text.startsWith('/remember ')) {
      text = text.slice('/remember '.length);
      box?.focus();
      return;
    }
    const t = text.trim();
    if ((!t || t === '/remember') && images.length === 0 && refs.length === 0) {
      text = '/remember ';
      box?.focus();
      return;
    }
    text = '/remember ' + t;
    send(ev);
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

  // Mic button: first click records, second click stops, uploads the take
  // for transcription and puts the text in the draft. Nothing is sent until
  // the user sends it.
  async function toggleMic() {
    if (voice === 'recording') return finishRecording();
    if (voice !== 'idle') return;
    voiceNote = '';
    levels = [];
    elapsedMs = 0;
    levelTick = 0;
    try {
      rec = await startRecording({
        onLevel: (v) => {
          if (++levelTick % 2 === 0) levels = [...levels.slice(-39), v];
        },
        onEnded: () => {
          if (voice === 'recording') {
            voiceNote = 'Recording stopped early. Kept what was captured.';
            void finishRecording();
          }
        },
      });
      voice = 'recording';
      const t0 = Date.now();
      clock = setInterval(() => (elapsedMs = Date.now() - t0), 250);
      tick('start');
    } catch {
      rec = null;
      voiceNote = 'Mic blocked or missing. Allow it in the browser.';
    }
  }

  async function finishRecording() {
    const r = rec;
    rec = null;
    if (!r || voice !== 'recording') return;
    stopClock();
    voice = 'transcribing';
    tick('stop');
    try {
      const out = await r.stop();
      if (!out.blob.size) {
        voiceNote = 'No audio captured.';
        return;
      }
      dropAudio();
      audio = { ...out, url: URL.createObjectURL(out.blob) };
      if (out.blob.size > STT_MAX_BYTES) {
        voiceNote = 'Recording is over the 25 MB cap. Discard it and record less.';
        return;
      }
      const got = await transcribe(out.blob, { format: voicePrefs.cleanup });
      if (got) text = text ? `${text} ${got}` : got;
      else voiceNote = 'No speech heard. Type the text or discard the recording.';
    } catch (e) {
      voiceNote = `Transcription failed (${e.message}). Type the text or discard the recording.`;
    } finally {
      voice = 'idle';
    }
  }

  function cancelRecording() {
    stopClock();
    try {
      rec?.cancel();
    } catch {
      // Already stopped.
    }
    rec = null;
    voice = 'idle';
    tick('stop');
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
  <!-- The send-mode header stays mounted and animates open while working and
       shut when idle. It always lists the working modes, so the buttons do
       not change mid fade-out. -->
  <div
    class="grid transition-[grid-template-rows,opacity] duration-300 ease-[cubic-bezier(.22,1,.36,1)] motion-reduce:transition-none {headerOpen
      ? 'grid-rows-[1fr] opacity-100'
      : 'grid-rows-[0fr] opacity-0'}"
    aria-hidden={!headerOpen}
    inert={!headerOpen}
    data-testid="mode-header"
  >
    <div class="min-h-0 overflow-hidden">
      <div class="flex items-center gap-1 px-2 pt-1">
        <div class="flex gap-0.5 rounded-full bg-background/60 p-0.5" role="group" aria-label="Send mode">
          {#each workingModes as opt (opt.id)}
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
    </div>
  </div>
  {#if images.length || refs.length}
    <ul class="flex flex-wrap gap-2 px-3 pt-2" aria-label="attached images" data-testid="composer-tray">
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
      {#each refs as r (r.id)}
        <li class="relative" title={r.name}>
          <img src={r.url} alt={r.name} class="size-14 rounded-lg border object-cover" />
          <button
            type="button"
            aria-label={`remove ${r.name}`}
            onclick={() => (refs = refs.filter((x) => x.id !== r.id))}
            class="absolute -right-1.5 -top-1.5 rounded-full border bg-background p-0.5 text-muted-foreground hover:text-foreground"
          >
            <X class="size-3" />
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  {#if gallery}
    <div class="px-3 pt-1" data-testid="gallery">
      {#if galleryImages.length}
        <div class="grid grid-cols-6 gap-1.5">
          {#each galleryImages as img (img.id)}
            <button
              type="button"
              onclick={() => referenceImage(img)}
              title={`Reference ${img.name}`}
              class="overflow-hidden rounded-lg border transition-opacity hover:opacity-80 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
            >
              <img src={img.url || imageSrc(sessionId, img.id)} alt={img.name} class="aspect-square w-full object-cover" loading="lazy" />
            </button>
          {/each}
        </div>
      {:else}
        <p class="text-xs text-muted-foreground">No images attached in this Session yet.</p>
      {/if}
    </div>
  {/if}
  {#if voice === 'recording'}
    <div class="animate-rise flex items-center gap-2 px-3 pt-2" data-testid="voice-recorder">
      <span class="size-2 shrink-0 animate-pulse rounded-full bg-destructive" aria-hidden="true"></span>
      <span data-testid="voice-status" class="sr-only">Listening…</span>
      <span class="w-10 shrink-0 text-xs tabular-nums text-muted-foreground">{fmtDuration(elapsedMs)}</span>
      <div class="flex h-8 min-w-0 flex-1 items-center justify-end gap-px overflow-hidden" aria-hidden="true" data-testid="voice-wave">
        {#each levels as v, i (i)}
          <span class="w-[3px] shrink-0 rounded-full bg-primary/80" style="height: {Math.max(10, Math.round(v * 100))}%"></span>
        {/each}
      </div>
      <Button type="button" variant="ghost" size="icon" aria-label="cancel recording" title="Cancel recording" onclick={cancelRecording}>
        <X />
      </Button>
    </div>
  {/if}
  {#if audio}
    <div class="animate-rise flex flex-wrap items-center gap-3 px-3 pt-2" data-testid="voice-draft">
      <div class="rounded-2xl bg-background/60 p-2.5">
        <VoiceBubble src={audio.url} peaks={audio.peaks} durationMs={audio.durationMs} transcript={text} transcriptEditable onRemove={dropAudio} />
      </div>
      {#if voice === 'transcribing'}
        <span data-testid="voice-status" class="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Loader class="size-3.5 animate-spin" />Transcribing…
        </span>
      {/if}
    </div>
  {:else if voice === 'transcribing'}
    <div class="px-3 pt-2 text-xs text-muted-foreground" data-testid="voice-status">Transcribing…</div>
  {/if}
  <div class="flex flex-wrap items-end gap-1 p-1 sm:flex-nowrap sm:gap-2">
    <div class="relative min-w-0 basis-full sm:basis-0 sm:flex-1">
      <textarea
        bind:this={box}
        aria-label="message"
        name="message"
        rows="2"
        bind:value={text}
        onkeydown={key}
        onpaste={paste}
        placeholder={status === 'working' ? 'Steer or follow up…' : 'Send a prompt…'}
        class="max-h-40 w-full resize-none bg-transparent px-3 pb-2 pt-2 text-[15px] outline-none placeholder:text-muted-foreground"
      ></textarea>
    </div>
    <input
      bind:this={picker}
      name="attachments"
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
    <Button type="button" variant="ghost" size="icon" aria-label="Session image gallery" title="Session image gallery" aria-pressed={gallery} disabled={!sessionId} onclick={toggleGallery}>
      <Images />
    </Button>
    {#if support.ok && voicePrefs.enabled}
      <Button
        type="button"
        variant={voice === 'recording' ? 'destructive' : 'ghost'}
        size="icon"
        aria-label={voice === 'recording' ? 'stop recording' : 'record voice'}
        title="Record a voice message"
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
    <Button type="button" variant="ghost" size="sm" aria-label="remember as rule" title="Remember as a Rule (sends /remember)" onclick={remember}><BookmarkPlus class="sm:hidden" /><span class="max-sm:sr-only">Remember</span></Button>
    <Button type="submit" size="default" class="ml-auto sm:ml-0" disabled={!canSend} title={canSend ? 'Send' : busy ? 'Sending the previous message…' : 'Type a message or attach an image'}>Send</Button>
  </div>
  {#if sendError}
    <p role="alert" class="px-3 pb-1 text-xs text-destructive">{sendError}</p>
  {/if}
  {#if imageNote}
    <p data-testid="image-note" class="px-3 pb-1 text-xs text-destructive">{imageNote}</p>
  {/if}
  {#if voiceNote || (voicePrefs.enabled && !support.ok)}
    <p data-testid="voice-note" class="px-3 pb-1 text-xs text-muted-foreground">{voiceNote || support.reason}</p>
  {/if}
</form>
