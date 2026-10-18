<!-- Transcript card: one visual block per grouped Event run. -->
<script>
  import { onDestroy } from 'svelte';
  import MessageActions from './MessageActions.svelte';
  import Markdown from './Markdown.svelte';
  import GroupCard from './GroupCard.svelte';
  import ToolStrip from './ToolStrip.svelte';
  import { Ban, CheckCircle2, XCircle, Loader, ChevronRight, Brain, Shrink, ShieldAlert, Terminal, CreditCard, Clock } from '@lucide/svelte';
  import { getCurrentUserId, decideSudo, decideProfileQueue, compressSession, imageSrc, promoteSessionRule } from './api.js';
  import Button from './ui/button.svelte';
  import IconButton from './ui/icon-button.svelte';
  import { icons } from './icons.js';
  import ErrorState from './ui/error-state.svelte';
  import Input from './ui/input.svelte';
  import VoiceBubble from './VoiceBubble.svelte';
  import { isAudioId, audioMeta } from './voiceMeta.js';
  import Badge from './ui/badge.svelte';
  import Chip from './ui/chip.svelte';
  import UserAvatar from './UserAvatar.svelte';
  import { people, personFor } from './people.svelte.js';
  import { ownerTone } from './owners.js';
  import { appendText } from './appendText.js';
  let { block, sessionId = null, sessions = [], onOpenSession = () => {}, onDismissReport = () => {}, queuedItem = null, cancelled = false, onQueueChange = () => {} } = $props();
  let actionsOpen = $state(false);
  let longPressTimer = 0;
  let longPressY = 0;
  let longPressTriggered = false;
  const ownUserMessage = $derived(!cancelled && !block.pending && block.card === 'user' && block.sender === 'user' && !!block.senderId && block.senderId === getCurrentUserId());
  const messageText = $derived(String(queuedItem?.text ?? block.body ?? ''));
  const messageImageIds = $derived(queuedItem?.imageIds ?? block.imageIds ?? []);
  function clearActionLongPress() {
    clearTimeout(longPressTimer);
    longPressTimer = 0;
  }
  function startActionLongPress(event) {
    if (!queuedItem && block.sender !== 'user' && !['assistant', 'report'].includes(block.card)) return;
    if (event.pointerType !== 'touch' && event.pointerType !== 'pen') return;
    if (event.target?.closest?.('[data-testid="message-actions"]')) return;
    longPressY = event.clientY ?? 0;
    longPressTriggered = false;
    longPressTimer = setTimeout(() => {
      actionsOpen = true;
      longPressTriggered = true;
    }, 500);
  }
  function moveActionLongPress(event) {
    if (longPressTimer && Math.abs((event.clientY ?? longPressY) - longPressY) > 10) clearActionLongPress();
  }
  function stopActionLongPress() {
    clearActionLongPress();
  }
  function suppressContextMenu(event) {
    if (longPressTriggered) event.preventDefault();
  }
  onDestroy(clearActionLongPress);
  // A live thinking block grows per delta: its <pre> appends text only, and
  // the details height animation is off until it settles, so the mount-only
  // rise animation is never re-triggered by streaming.
  const thinkingLive = $derived(block.card === 'thinking' && !!block.live);
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
  let profileBusy = $state(false);
  let profileError = $state('');
  async function decideProfile(status) {
    profileBusy = true; profileError = '';
    try { await decideProfileQueue(block.profilePatch.id, status); }
    catch (e) { profileError = e.message; }
    finally { profileBusy = false; }
  }
  let compressBusy = $state(false);
  let compressError = $state('');
  // Retry reruns the whole compress of this card's Session.
  async function retryCompress() {
    compressBusy = true;
    compressError = '';
    try {
      await compressSession(sessionId);
    } catch (e) {
      compressError = e.message;
    } finally {
      compressBusy = false;
    }
  }
  // Promote an injected Rule to project knowledge from the chat: the Session
  // route files the promotion against this Session's worktree, and the
  // promotion queue approves the diff.
  const injectedRules = $derived((block.rules ?? []).filter((r) => r && typeof r.id === 'string' && r.id));
  let promoteBusy = $state('');
  let promoteError = $state('');
  let promotedIds = $state([]);
  const shortId = (id) => String(id).slice(0, 8);
  async function promoteRule(rule) {
    promoteBusy = rule.id;
    promoteError = '';
    try {
      await promoteSessionRule(sessionId, rule.id);
      if (!promotedIds.includes(rule.id)) promotedIds = [...promotedIds, rule.id];
    } catch (e) {
      promoteError = e.message;
    } finally {
      promoteBusy = '';
    }
  }
  // One-line hint of what the tool was asked: first line of the input.
  const firstReply = $derived(String(block.members?.[0]?.reply ?? block.body ?? '').replace(/\s+/g, ' ').slice(0, 110));
  const reportCounts = $derived(block.report?.counts ?? {});
  const reportCountLabel = $derived([
    `${reportCounts.total ?? block.members?.length ?? 0} agent${(reportCounts.total ?? block.members?.length ?? 0) === 1 ? '' : 's'}`,
    reportCounts.workers ? `${reportCounts.workers} worker${reportCounts.workers === 1 ? '' : 's'}` : '',
    reportCounts.scouts ? `${reportCounts.scouts} scout${reportCounts.scouts === 1 ? '' : 's'}` : '',
    reportCounts.failed ? `${reportCounts.failed} failed` : '',
  ].filter(Boolean).join(' · '));
  const preview = $derived(String(block.input ?? '').replace(/\s+/g, ' ').trim().slice(0, 120));
  const fmtTokens = (n) => (n >= 1000 ? `${(n / 1000).toFixed(n >= 100000 ? 0 : 1)}k` : String(n ?? 0));
  const compaction = $derived(block.compaction ?? {});
  const saved = $derived(compaction.tokensBefore > 0 && compaction.tokensAfter > 0 ? Math.max(0, Math.round((1 - compaction.tokensAfter / compaction.tokensBefore) * 100)) : null);
  // Voice recordings ride the image ids ("aud_" prefix): they show as a voice
  // bubble whose transcript is the message text; the rest stay pictures.
  const voiceIds = $derived(messageImageIds.filter(isAudioId));
  const pictureIds = $derived(messageImageIds.filter((i) => !isAudioId(i)));
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
<div role="group" aria-label={queuedItem ? `Queued ${block.title}` : cancelled ? `Canceled ${block.title || 'User'} message` : `${block.title || 'User'} message`} class="group relative flex flex-col items-end" data-card="user" data-state={queuedItem ? 'queued' : cancelled ? 'canceled' : block.pending ? 'pending' : block.sender === 'user' ? 'delivered' : undefined} data-swap={block.noAnim ? 'true' : undefined} onpointerdown={startActionLongPress} onpointermove={moveActionLongPress} onpointerup={stopActionLongPress} onpointercancel={stopActionLongPress} onpointerleave={stopActionLongPress} oncontextmenu={suppressContextMenu}>
  <div class="flex w-full flex-col items-end {block.noAnim ? '' : 'animate-rise'}">
    {#if block.sender === 'user' && block.senderId}
      {@const who = personFor(block.senderId)}
      <Badge variant="outline" class="mb-1 py-0 pr-2.5 pl-1" data-testid="sender-badge">
        <UserAvatar name={who?.username ?? block.senderId} tone={ownerTone(people.list, block.senderId)} size="sm" />
        <span class="max-w-40 truncate">{who?.username ?? block.senderId.slice(0, 8)}</span>
      </Badge>
    {/if}
    {#if queuedItem}
      <div class="mb-1 flex items-center gap-1.5 pr-1.5 text-caption font-medium text-status-working" data-testid="queued-message-marker">
        <Clock class="size-3.5 shrink-0" aria-hidden="true" />
        <span>Queued</span>
        {#if block.title}<span class="text-fg-3">· {block.title}</span>{/if}
      </div>
    {:else if cancelled}
      <div class="mb-1 flex items-center gap-1.5 pr-1.5 text-caption text-fg-3">
        <Ban class="size-3.5 shrink-0" aria-hidden="true" />
        <span data-testid="canceled-message">Canceled</span>
      </div>
    {/if}
    <div class="max-w-[85%] rounded-2xl rounded-br-md border px-3.5 py-2.5 text-body whitespace-pre-wrap [overflow-wrap:anywhere] sm:max-w-[75%] {queuedItem ? 'border-dashed border-status-working/40 bg-transparent text-fg-2' : cancelled ? 'border-dashed border-border bg-transparent text-fg-3' : 'border-border/60 bg-surface-2 text-fg-1 shadow-elev-1'} {block.pending === 'sending' ? 'opacity-60' : ''}">
      {#if block.title && block.title !== 'You' && !queuedItem}
        <div class="mb-1 text-caption font-medium tracking-wide text-fg-3 uppercase">{block.title}</div>
      {/if}
      {#if !voiceIds.length}{messageText}{/if}
      {#each voiceIds as vid (vid)}
        <div class="w-64 max-w-full text-left" data-testid="voice-message">
          <VoiceBubble
            src={sessionId ? imageSrc(sessionId, vid) : ''}
            peaks={voiceMeta[vid]?.peaks ?? []}
            durationMs={voiceMeta[vid]?.durationMs ?? 0}
            transcript={messageText}
          />
        </div>
      {/each}
      {#if pictureIds.length}
        <div class="mt-2 flex flex-wrap justify-end gap-1.5">
          {#each pictureIds as imgId (imgId)}
            {#if sessionId}
              <img src={imageSrc(sessionId, imgId)} alt="Attachment" class="h-28 w-28 rounded-xl border object-cover" loading="lazy" />
            {:else}
              <span class="rounded-lg bg-background/60 px-2 py-1 font-mono text-caption text-fg-3">image</span>
            {/if}
          {/each}
        </div>
      {/if}
      {#if block.pending === 'sending'}
        <div class="mt-1 text-caption text-fg-3">Sending…</div>
      {/if}
    </div>
  </div>
  {#if block.sender === 'user' || queuedItem}
    <MessageActions block={block} {sessionId} canEdit={ownUserMessage} {queuedItem} open={actionsOpen} onOpenChange={(value) => (actionsOpen = value)} {onQueueChange} />
  {/if}
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
      {#if sudoError}<ErrorState compact title={sudoError} class="mt-2" />{/if}
    </div>
  </div>
{:else if block.card === 'profilePatch'}
  <div class="animate-rise rounded-2xl border border-primary/30 bg-card p-4" data-card="profile-patch" data-state={block.profilePatch?.state ?? 'pending'}>
    <div class="text-sm font-semibold">Profile update proposed</div>
    {#if block.body}<p class="mt-1 text-sm text-muted-foreground">{block.body}</p>{/if}
    <pre class="mt-2 whitespace-pre-wrap text-xs">{JSON.stringify(block.profilePatch?.ops ?? [], null, 2)}</pre>
    {#if !block.profilePatch?.state || block.profilePatch.state === 'pending'}
      <div class="mt-3 flex gap-2"><Button disabled={profileBusy} onclick={() => decideProfile('approved')}>Approve</Button><Button variant="outline" disabled={profileBusy} onclick={() => decideProfile('rejected')}>Reject</Button></div>
    {:else}<div class="mt-2 text-xs text-muted-foreground">{block.profilePatch.state}</div>{/if}
    {#if profileError}<ErrorState compact title={profileError} class="mt-2" />{/if}
  </div>
{:else if block.card === 'sudoresult'}
  <details class="group animate-rise overflow-hidden rounded-2xl border border-dashed bg-card text-card-foreground" data-card="sudoresult" open>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Terminal class="size-4 shrink-0 text-muted-foreground" />
      <span class="font-medium">Sudo {sudo.status}</span>
      {#if sudo.status !== 'denied'}<span class="rounded-full bg-muted px-2 py-0.5 text-[11px] text-muted-foreground">exit {sudo.exit}</span>{/if}
      <span class="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{sudo.command}</span>
      <ChevronRight class="size-4 shrink-0 text-muted-foreground motion-standard transition-transform group-open:rotate-90" />
    </summary>
    <div class="flex flex-col gap-3 border-t bg-background/40 p-3.5">
      <pre class="max-h-40 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-sm whitespace-pre-wrap">{sudo.command}</pre>
      {#if sudo.output}<pre class="max-h-96 overflow-auto rounded-xl bg-muted/60 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap">{sudo.output}</pre>{/if}
    </div>
  </details>
{:else if block.card === 'compressBlocked'}
  <div class="animate-rise overflow-hidden rounded-2xl border border-amber-500/40 bg-card text-card-foreground" data-card="compressBlocked">
    <div class="flex flex-wrap items-center gap-2.5 px-4 py-2.5 text-sm">
      <CreditCard class="size-4 shrink-0 text-amber-400" />
      <span class="font-medium">{block.title}</span>
      <span class="min-w-0 flex-1 truncate text-xs text-muted-foreground" title={block.body}>{block.body}</span>
      <IconButton icon={icons.retry} label="Retry" disabled={compressBusy} onclick={retryCompress} />
    </div>
    {#if compressError}<ErrorState compact title={compressError} class="mx-4 mb-2.5" />{/if}
  </div>
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
        {#if block.body}<ChevronRight class="size-4 shrink-0 text-muted-foreground motion-standard transition-transform group-open:rotate-90" />{/if}
      {/if}
    </summary>
    {#if block.body && compaction.state === 'done'}
      <div class="border-t px-4 py-3 text-sm leading-6 text-muted-foreground">
        <div class="mb-1 text-[11px] font-medium uppercase tracking-wide">Summary kept in context</div>
        <Markdown text={block.body} />
      </div>
    {/if}
  </details>
{:else if block.card === 'divider'}
  <div class="flex animate-rise items-center gap-3 text-xs text-muted-foreground" data-card="divider">
    <span class="h-px flex-1 bg-border"></span>
    <span>{block.title}{#if block.body} · {block.body}{/if}</span>
    <span class="h-px flex-1 bg-border"></span>
  </div>
  {#if injectedRules.length > 0 && sessionId}
    <div class="flex flex-wrap items-center justify-center gap-1.5" data-testid="promote-rules">
      {#each injectedRules as rule (rule.id)}
        {@const done = promotedIds.includes(rule.id)}
        <Button
          variant="ghost"
          size="sm"
          disabled={promoteBusy === rule.id || done}
          aria-label={done ? `Promotion proposed for rule ${shortId(rule.id)}` : `Promote rule ${shortId(rule.id)} to project knowledge`}
          title={done ? 'Promotion proposed; approve its diff in the promotion queue' : 'Propose this Rule as an edit to the nearest AGENTS.md'}
          onclick={() => promoteRule(rule)}
        >
          {promoteBusy === rule.id ? 'Proposing…' : done ? `Proposed ${shortId(rule.id)}` : `Promote ${shortId(rule.id)}`}
        </Button>
      {/each}
    </div>
    {#if promoteError}
      <ErrorState compact title={promoteError} />
    {/if}
  {/if}
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
      <ChevronRight class="size-4 shrink-0 text-muted-foreground motion-standard transition-transform group-open:rotate-90" />
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
  {@const ReportIcon = icons.report}
  <div role="group" aria-label="Subagent report" class="group relative w-full" data-card-wrapper="report" onpointerdown={startActionLongPress} onpointermove={moveActionLongPress} onpointerup={stopActionLongPress} onpointercancel={stopActionLongPress} onpointerleave={stopActionLongPress} oncontextmenu={suppressContextMenu}>
    <div class="flex animate-rise items-start gap-1 rounded-xl border border-border/60 bg-surface-1 p-1.5 text-card-foreground shadow-elev-1">
      <details class="group/report min-w-0 flex-1 overflow-hidden rounded-lg" data-card="report" data-report-seq={block.report?.seq} open={!block.collapsed}>
        <summary class="flex min-h-11 cursor-pointer list-none items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-surface-3/60 [&::-webkit-details-marker]:hidden">
          <span class="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-fg-2" aria-hidden="true">
            <ReportIcon class="size-4" />
          </span>
          <span class="min-w-0 flex-1">
            <span class="block min-w-0 truncate text-body font-medium">{block.report ? block.report.title : block.title}</span>
            <span class="mt-0.5 flex min-w-0 flex-col text-caption text-fg-3 sm:flex-row sm:items-center sm:gap-1.5">
              {#if block.report}<span class="min-w-0 truncate sm:shrink-0">Subagents {block.report.groupName} finished</span>{/if}
              {#if firstReply}<span class="hidden shrink-0 text-fg-4 sm:inline" aria-hidden="true">·</span><span class="min-w-0 truncate">{firstReply}</span>{/if}
            </span>
          </span>
          <Chip tone="outline" class="hidden shrink-0 sm:inline-flex">{reportCountLabel}</Chip>
          <ChevronRight class="size-4 shrink-0 text-fg-3 motion-standard transition-transform group-open/report:rotate-90" aria-hidden="true" />
        </summary>
        <div class="flex flex-col gap-3 border-t border-border/60 p-3">
          {#each block.members?.length ? block.members : [{ id: '', kind: 'agent', reply: block.body }] as m}
            <div>
              <div class="mb-1 flex items-center gap-2 text-caption font-medium tracking-wide text-fg-3 uppercase">
                {m.kind}{#if m.id}<span class="font-mono normal-case">{m.id}</span>{/if}
              </div>
              <div class="max-h-96 overflow-auto rounded-lg bg-surface-0 p-3"><Markdown text={m.reply} /></div>
            </div>
          {/each}
        </div>
      </details>
      {#if block.report}
        <IconButton icon={icons.dismiss} label="Dismiss report" class="mt-1 shrink-0" onclick={() => onDismissReport(block.report.seq)} />
      {/if}
    </div>
    <MessageActions {block} {sessionId} open={actionsOpen} onOpenChange={(value) => (actionsOpen = value)} align="start" />
  </div>
{:else if block.card === 'thinking'}
  <details class="group animate-rise rounded-2xl border bg-card text-card-foreground {thinkingLive ? 'think-live' : ''}" data-card="thinking" data-live={thinkingLive ? 'true' : undefined} open={!block.collapsed}>
    <summary class="flex cursor-pointer list-none items-center gap-2.5 px-3.5 py-2.5 text-sm text-muted-foreground transition-colors hover:bg-muted/50 [&::-webkit-details-marker]:hidden">
      <Brain class="size-4" /><span class="flex-1">{block.title}</span>
      <ChevronRight class="size-4 motion-standard transition-transform group-open:rotate-90" />
    </summary>
    <pre class="max-h-64 overflow-auto border-t bg-background/40 p-3.5 font-mono text-xs whitespace-pre-wrap" use:appendText={block.body}></pre>
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
  <ErrorState compact title={block.body} class="animate-rise" data-card="error" />
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
  <div role="group" aria-label={block.title || 'Message'} class="group relative flex w-full flex-col items-start" data-card={block.card} data-state={block.card === 'assistant' ? 'delivered' : undefined} onpointerdown={startActionLongPress} onpointermove={moveActionLongPress} onpointerup={stopActionLongPress} onpointercancel={stopActionLongPress} onpointerleave={stopActionLongPress} oncontextmenu={suppressContextMenu}>
    <div class="flex w-full animate-rise flex-col items-start gap-1 py-1">
      {#if block.title}<div class="text-caption font-medium text-fg-3">{block.title}</div>{/if}
      <div class="w-full min-w-0 text-body text-fg-1">
        <Markdown text={messageText} fade={block.card === 'assistant'} />
      </div>
    </div>
    {#if block.card === 'assistant'}<MessageActions {block} {sessionId} open={actionsOpen} onOpenChange={(value) => (actionsOpen = value)} align="start" />{/if}
  </div>
{/if}
<style>
  details.think-live::details-content {
    transition: none;
  }
</style>
