<!-- Transcript card: one visual block per grouped Event run. -->
<script>
  import Markdown from './Markdown.svelte';
  import GroupCard from './GroupCard.svelte';
  import ToolStrip from './ToolStrip.svelte';
  import { CheckCircle2, XCircle, Loader, ChevronRight, Brain, Bot, Shrink, ShieldAlert, Terminal } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import { decideSudo, imageSrc } from './api.js';
  import VoiceBubble from './VoiceBubble.svelte';
  import { isAudioId, audioMeta } from './voiceMeta.js';
  let { block, sessionId = null, sessions = [], onOpenSession = () => {} } = $props();
  let sudoPassword = $state('');
  let sudoBusy = $state(false);
  let sudoError = $state('');
  const sudo = $derived(block.sudo ?? {});
  async function decide(action) {
    sudoBusy = true;
    sudoError = '';
    try {
      await decideSudo(sessionId, sudo.id, action, action === 'approve' ? sudoPassword : '');
      sudoPassword = '';
    } catch (e) {
      sudoError = e.message;
    } finally {
      sudoBusy = false;
    }
  }
  // One-line hint of what the tool was asked: first line of the input.
  const firstReply = $derived(String(block.members?.[0]?.reply ?? block.body ?? '').replace(/\s+/g, ' ').slice(0, 110));
  const preview = $derived(String(block.input ?? '').replace(/\s+/g, ' ').trim().slice(0, 120));
  const fmtTokens = (n) => (n >= 1000 ? `${(n / 1000).toFixed(n >= 100000 ? 0 : 1)}k` : String(n ?? 0));
  const compaction = $derived(block.compaction ?? {});
  const saved = $derived(compaction.tokensBefore > 0 && compaction.tokensAfter > 0 ? Math.max(0, Math.round((1 - compaction.tokensAfter / compaction.tokensBefore) * 100)) : null);
  // Voice recordings ride the image ids ("aud_" prefix): they show as a voice
  // bubble whose transcript is the message text; the rest stay pictures.
  const voiceIds = $derived((block.imageIds ?? []).filter(isAudioId));
  const pictureIds = $derived((block.imageIds ?? []).filter((i) => !isAudioId(i)));
  let voiceMeta = $state({});
  $effect(() => {
    for (const vid of voiceIds) {
      if (!sessionId || voiceMeta[vid]) continue;
      audioMeta(sessionId, vid).then((m) => {
        if (m) voiceMeta = { ...voiceMeta, [vid]: m };
      });
    }
  });
  const diffLines = $derived(block.card === 'diff' ? String(block.body ?? '').split('\n') : []);
</script>

{#if block.card === 'user'}
  <div class="flex justify-end {block.noAnim ? '' : 'animate-rise'}" data-card="user" data-swap={block.noAnim ? 'true' : undefined}>
    <div class="max-w-[80%] rounded-3xl bg-muted px-4 py-2.5 text-[15px] leading-relaxed whitespace-pre-wrap [overflow-wrap:anywhere] text-foreground {block.pending === 'sending' ? 'opacity-60' : ''}">
      {#if block.title && block.title !== 'You'}
        <div class="mb-0.5 text-[11px] font-medium uppercase tracking-wide opacity-80">{block.title}</div>
      {/if}
      {#if !voiceIds.length}{block.body}{/if}
      {#each voiceIds as vid (vid)}
        <div class="w-64 max-w-full text-left" data-testid="voice-message">
          <VoiceBubble
            src={sessionId ? imageSrc(sessionId, vid) : ''}
            peaks={voiceMeta[vid]?.peaks ?? []}
            durationMs={voiceMeta[vid]?.durationMs ?? 0}
            transcript={block.body}
          />
        </div>
      {/each}
      {#if pictureIds.length}
        <div class="mt-2 flex flex-wrap justify-end gap-1.5">
          {#each pictureIds as imgId (imgId)}
            {#if sessionId}
              <img src={imageSrc(sessionId, imgId)} alt="Attached image" class="h-28 w-28 rounded-xl border object-cover" loading="lazy" />
            {:else}
              <span class="rounded-lg bg-background/60 px-2 py-1 font-mono text-[11px] text-muted-foreground">image</span>
            {/if}
          {/each}
        </div>
      {/if}
      {#if block.pending === 'sending'}
        <div class="mt-0.5 text-[11px] text-muted-foreground">Sending…</div>
      {/if}
    </div>
  </div>
{:else if block.card === 'sudo'}
  <div class="animate-rise overflow-hidden rounded-2xl border border-amber-500/40 bg-card text-card-foreground" data-card="sudo" data-state={sudo.state}>
    <div class="flex items-center gap-2.5 px-4 py-2.5 text-sm">
      <ShieldAlert class="size-4 shrink-0 text-amber-400" />
      <span class="font-medium">Sudo request</span>
      <span class="ml-auto rounded-full bg-muted px-2 py-0.5 text-[11px] uppercase tracking-wide text-muted-foreground">
        {sudo.state === 'pending' ? 'waiting for you' : sudo.state}
      </span>
    </div>
    <div class="border-t bg-background/40 p-3.5">
      <div class="mb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Command{#if sudo.cwd}<span class="ml-2 font-mono normal-case">in {sudo.cwd}</span>{/if}</div>
      <pre class="max-h-56 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-sm leading-relaxed whitespace-pre-wrap" data-testid="sudo-command">{sudo.command}</pre>
      {#if sudo.state === 'pending'}
        <div class="mt-3 flex flex-wrap items-center gap-2">
          <Input type="password" autocomplete="off" bind:value={sudoPassword} placeholder="sudo password (optional)" aria-label="sudo password" class="h-9 max-w-64 flex-1" />
          <Button size="default" disabled={sudoBusy} onclick={() => decide('approve')}>Approve</Button>
          <Button size="default" variant="outline" disabled={sudoBusy} onclick={() => decide('deny')}>Deny</Button>
        </div>
        <p class="mt-1.5 text-xs text-muted-foreground">Without a password, sudo must already be passwordless. The password is used once and never stored.</p>
      {/if}
      {#if sudoError}<p role="alert" class="mt-2 text-xs text-destructive">{sudoError}</p>{/if}
    </div>
  </div>
{:else if block.card === 'sudoresult'}
  <details class="group animate-rise overflow-hidden rounded-2xl border border-dashed bg-card text-card-foreground" data-card="sudoresult" open>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Terminal class="size-4 shrink-0 text-muted-foreground" />
      <span class="font-medium">Sudo {sudo.status}</span>
      {#if sudo.status !== 'denied'}<span class="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">exit {sudo.exit}</span>{/if}
      <span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{sudo.command}</span>
      <ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      <pre class="max-h-40 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-sm whitespace-pre-wrap">{sudo.command}</pre>
      {#if sudo.output}<pre class="max-h-96 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{sudo.output}</pre>{/if}
    </div>
  </details>
{:else if block.card === 'compaction'}
  <details class="group animate-rise overflow-hidden rounded-2xl border bg-card/60 text-card-foreground" data-card="compaction" data-state={compaction.state}>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      {#if compaction.state === 'running'}
        <Loader class="size-4 animate-spin text-muted-foreground" />
        <span class="font-medium">Compressing context…</span>
      {:else if compaction.state === 'failed'}
        <XCircle class="size-4 text-destructive" />
        <span class="font-medium">Compression failed</span>
        <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{compaction.error ?? ''}</span>
      {:else}
        <Shrink class="size-4 text-green-400" />
        <span class="font-medium">Context compressed</span>
        {#if compaction.reason && compaction.reason !== 'manual'}<span class="rounded-full bg-muted px-2 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">auto</span>{/if}
        <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {fmtTokens(compaction.tokensBefore)} → {fmtTokens(compaction.tokensAfter)} tokens{#if saved !== null} · −{saved}%{/if}
        </span>
        {#if block.body}<ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />{/if}
      {/if}
    </summary>
    {#if block.body && compaction.state === 'done'}
      <div class="border-t px-4 py-3 text-sm leading-6 text-muted-foreground">
        <div class="mb-1 text-[11px] font-medium uppercase tracking-wide">Summary kept in context</div>
        <Markdown text={block.body} />
      </div>
    {/if}
  </details>
{:else if block.card === 'status'}
  <div class="flex animate-rise justify-center" data-card="status">
    <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">Session {block.title}</span>
  </div>
{:else if block.card === 'toolstrip'}
  <ToolStrip block={block} />
{:else if block.card === 'tool'}
  <details class="group animate-rise overflow-hidden rounded-2xl border bg-card text-card-foreground" data-card="tool">
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <span class="flex size-5 shrink-0 items-center justify-center">
        {#if block.status === 'failed'}
          <XCircle class="size-4 text-destructive" />
        {:else if block.status === 'completed' || block.status === 'complete'}
          <CheckCircle2 class="size-4 text-green-400" />
        {:else}
          <Loader class="size-4 animate-spin text-muted-foreground" />
        {/if}
      </span>
      <span class="min-w-0 truncate font-medium">{block.title}</span>
      {#if block.server}
        <span class="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">{block.server}</span>
      {/if}
      {#if preview}
        <span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{preview}</span>
      {:else}
        <span class="flex-1"></span>
      {/if}
      <ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      {#if block.input}
        <div>
          <div class="mb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Input</div>
          <pre class="max-h-56 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{block.input}</pre>
        </div>
      {/if}
      {#if block.output}
        <div>
          <div class="mb-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">Output</div>
          <pre class="max-h-72 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{block.output}</pre>
        </div>
      {/if}
      {#if !block.input && !block.output}
        <pre class="max-h-72 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs whitespace-pre-wrap">{block.body || 'No content.'}</pre>
      {/if}
    </div>
  </details>
{:else if block.card === 'report'}
  <details class="group animate-rise overflow-hidden rounded-2xl border border-dashed bg-card text-card-foreground" data-card="report">
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Bot class="size-4 shrink-0 text-muted-foreground" />
      <span class="font-medium">{block.title}</span>
      <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">{firstReply}</span>
      <ChevronRight class="size-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      {#each block.members?.length ? block.members : [{ id: '', kind: 'agent', reply: block.body }] as m}
        <div>
          <div class="mb-1 flex items-center gap-2 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
            {m.kind}{#if m.id}<span class="font-mono normal-case">{m.id}</span>{/if}
          </div>
          <div class="max-h-96 overflow-auto rounded-xl bg-muted/60 p-3"><Markdown text={m.reply} /></div>
        </div>
      {/each}
    </div>
  </details>
{:else if block.card === 'thinking'}
  <details class="group animate-rise rounded-2xl border bg-card text-card-foreground" data-card="thinking" open={!block.collapsed}>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Brain class="size-4" /><span class="flex-1">{block.title}</span>
      <ChevronRight class="size-4 transition-transform duration-200 group-open:rotate-90" />
    </summary>
    <pre class="max-h-64 overflow-auto border-t bg-background/40 p-3.5 font-mono text-xs whitespace-pre-wrap">{block.body}</pre>
  </details>
{:else if block.card === 'diff'}
  <div class="animate-rise overflow-hidden rounded-2xl border bg-card text-card-foreground" data-card="diff">
    <div class="border-b px-4 py-2 text-sm font-medium">Diff</div>
    <pre class="max-h-80 overflow-auto px-4 py-2 font-mono text-xs leading-relaxed">{#each diffLines as line}<span
        class:diff-add={line.startsWith('+') && !line.startsWith('+++')}
        class:diff-del={line.startsWith('-') && !line.startsWith('---')}>{line}
</span>{/each}</pre>
  </div>
{:else if block.card === 'permission'}
  <div class="animate-rise rounded-2xl border border-green-500/30 bg-card px-4 py-2.5 text-sm" data-card="permission">
    <span class="mr-2 font-medium">✓ {block.title}</span><span class="text-muted-foreground">{block.body}</span>
  </div>
{:else if block.card === 'error'}
  <div class="animate-rise rounded-2xl border border-destructive/40 bg-destructive/10 px-4 py-2.5 text-sm" role="alert" data-card="error">
    <span class="font-medium">Error: </span>{block.body}
  </div>
{:else if block.card === 'group'}
  <GroupCard block={block} {sessions} {onOpenSession} />
{:else if block.card === 'subagent'}
  <div class="animate-rise ml-6 rounded-2xl border border-dashed bg-card px-4 py-2.5 text-sm" data-card="subagent" title={block.childId ?? block.body}>
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    {#if block.childId}
      {@const short = String(block.childId ?? '').slice(0, 8)}
      <span class="font-mono text-[11px] text-muted-foreground/60">{short}</span>
    {:else}
      <Markdown text={block.body} />
    {/if}
  </div>
{:else}
  <div class="animate-rise px-1 py-1 text-[15px] leading-7" data-card={block.card}>
    <div class="mb-1 text-xs font-medium text-muted-foreground">{block.title}</div>
    <Markdown text={block.body} fade={block.card === 'assistant'} />
  </div>
{/if}
