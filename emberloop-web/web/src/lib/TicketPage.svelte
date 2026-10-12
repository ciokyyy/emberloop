<!-- Ticket page at /runs/<runId>/<ticketId>: the worker transcript as the
     main column with review rounds woven in, plus the facts panel (agents,
     acceptance, gate, follow-ups). Realtime comes from the Run SSE snapshot
     and the worker Session stream; nothing here polls. Reviewers and scouts
     open read-only in a drawer with no composer. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import { ChevronLeft, Check, X } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import Transcript from './Transcript.svelte';
  import Composer from './Composer.svelte';
  import Markdown from './Markdown.svelte';
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { groupTranscript, withImageNote } from './events.js';
  import { harnessStyle } from './harnesses.js';
  import {
    getRun,
    openRunStream,
    getSession,
    openStream,
    postPrompt,
    postSteer,
    postFollowup,
    interruptSession,
    steerTicket,
    retryTicket,
    parkTicket,
    discardTicket,
  } from './api.js';
  import {
    findTicket,
    stateTrack,
    reviewCards,
    gateSummary,
    boxProgress,
    agentsOf,
    agentRoleLabel,
    shortTip,
    roundNumber,
    sinceLabel,
    dropPendingEcho,
    ticketFinished,
    runFinished,
  } from './ticketPage.js';

  import FloatingInput from './ui/floating-input.svelte';

  let { runId = '', ticketId = '', onNavigate = null, onOpenSession = null } = $props();

  const TAIL_TURNS = 6;

  let run = $state(null);
  let runError = $state('');
  let runSource = null;
  // Stream health: the EventSource error hook flips these until the next
  // snapshot lands, so the page can show a reconnecting indicator.
  let runStreamDown = $state(false);
  let wStreamDown = $state(false);
  // Generation tokens plus a disposed flag: a getSession resolving after
  // unmount or a {#key} change must never open a stream on stale state.
  let disposed = false;
  let rToken = 0;

  // Worker feed: tail snapshot plus the live Session stream, load-earlier on
  // scroll up, optimistic pending cards like the chat view.
  let wMeta = $state(null);
  let wEvents = $state([]);
  let wLastSeq = $state(0);
  let wHasMore = $state(false);
  let wFirstSeq = $state(0);
  let wLoadingEarlier = $state(false);
  let wSource = null;
  let wToken = 0;
  let loadedWorkerFor = '';
  let sending = $state(false);
  let pendingSends = $state([]);
  let pendingKey = 0;
  let briefOpen = $state(false);

  // Per-ticket actions (steer, retry, park, discard): one route each on
  // the Run, answered with the fresh snapshot. Discard asks twice.
  let acting = $state(false);
  let actionError = $state('');
  let steerOpen = $state(false);
  let steerText = $state('');
  let confirmDiscard = $state(false);

  // Read-only drawer feed for one reviewer or scout Session. No composer.
  let drawer = $state(null);
  let dEvents = $state([]);
  let dLastSeq = $state(0);
  let dHasMore = $state(false);
  let dFirstSeq = $state(0);
  let dLoadingEarlier = $state(false);
  let dError = $state('');
  let dSource = null;
  let dToken = 0;

  const ticket = $derived(findTicket(run, ticketId));
  const track = $derived(stateTrack(ticket));
  const gate = $derived(gateSummary(ticket));
  const agents = $derived(agentsOf(ticket));
  const boxes = $derived(ticket?.boxes ?? []);
  const progress = $derived(boxProgress(boxes));
  const workerStatus = $derived(wMeta?.status ?? 'idle');
  const agentByRole = $derived(Object.fromEntries((agents ?? []).map((a) => [a.role, a])));
  const wBlocks = $derived([
    ...groupTranscript(wEvents, {}),
    ...pendingSends.map((p) => ({
      key: `pending-${p.key}`,
      card: 'user',
      title: p.mode === 'steer' ? 'Steer' : p.mode === 'followup' ? 'Follow-up' : 'You',
      body: withImageNote(p.text, p.images),
      imageIds: p.imageIds ?? [],
      pending: p.state,
      events: [],
    })),
  ]);
  const dBlocks = $derived(groupTranscript(dEvents, {}));

  function closeRunStream() {
    if (runSource) runSource.close();
    runSource = null;
  }

  function closeWorkerStream() {
    if (wSource) wSource.close();
    wSource = null;
  }

  function closeDrawerStream() {
    if (dSource) dSource.close();
    dSource = null;
  }

  function runOrTicketDone(r) {
    return runFinished(r) || ticketFinished(findTicket(r, ticketId));
  }

  async function loadRun() {
    const token = ++rToken;
    runError = '';
    try {
      const body = await getRun(runId);
      if (disposed || token !== rToken) return;
      run = body.run ?? body;
      briefOpen = false;
      if (runOrTicketDone(run)) {
        closeRunStream();
        return;
      }
      subscribeRun();
    } catch {
      if (disposed || token !== rToken) return;
      runError = 'Could not load the Run.';
    }
  }

  // SSE only: the Daemon publishes a fresh snapshot on every change (box
  // tick, verdict, round append, state change, agent added). A finished
  // run/ticket closes the stream; the snapshot stays on screen.
  function subscribeRun() {
    closeRunStream();
    if (disposed) return;
    if (typeof EventSource === 'undefined') return;
    try {
      runSource = openRunStream(
        runId,
        (body) => {
          if (disposed) return;
          runStreamDown = false;
          run = body.run ?? body;
          if (runOrTicketDone(run)) closeRunStream();
        },
        { onError: () => (runStreamDown = true), onOpen: () => (runStreamDown = false) },
      );
    } catch {
      // The snapshot stays; the next navigation reloads.
    }
  }

  function subscribeWorker(id, token) {
    if (disposed || token !== wToken) return;
    if (typeof EventSource === 'undefined') return;
    try {
      wSource = openStream(
        id,
        wLastSeq,
        (e) => {
          if (disposed || token !== wToken) return;
          if (e.seq <= wLastSeq) return;
          wStreamDown = false;
          wLastSeq = e.seq;
          wEvents = [...wEvents, e];
          if (e.kind === 'prompt') pendingSends = dropPendingEcho(pendingSends, e);
          if (e.kind === 'status' && e.status) wMeta = { ...(wMeta ?? {}), status: e.status };
        },
        { onError: () => (wStreamDown = true), onOpen: () => (wStreamDown = false) },
      );
    } catch {
      // The tail snapshot stays.
    }
  }

  async function loadWorker(id) {
    const token = ++wToken;
    closeWorkerStream();
    wEvents = [];
    wLastSeq = 0;
    wMeta = null;
    wHasMore = false;
    wFirstSeq = 0;
    wLoadingEarlier = false;
    pendingSends = [];
    if (!id) return;
    try {
      const body = await getSession(id, { tail: TAIL_TURNS });
      if (disposed || token !== wToken) return;
      wMeta = body.session ?? null;
      wEvents = body.events ?? [];
      wHasMore = body.hasMore ?? false;
      wFirstSeq = body.firstSeq ?? 0;
      wLastSeq = wEvents.reduce((m, e) => Math.max(m, e.seq ?? 0), 0);
      subscribeWorker(id, token);
    } catch {
      // The rounds and panel still render without the worker transcript.
    }
  }

  async function loadWorkerEarlier() {
    const id = ticket?.workerSession;
    if (!id || !wHasMore || wLoadingEarlier) return;
    wLoadingEarlier = true;
    const first = wFirstSeq;
    try {
      const body = await getSession(id, { before: first, turns: TAIL_TURNS });
      if (disposed || id !== ticket?.workerSession) return;
      wEvents = [...(body.events ?? []), ...wEvents];
      wHasMore = body.hasMore ?? false;
      wFirstSeq = body.firstSeq ?? wFirstSeq;
    } catch {
      // The sentinel stays: retry on the next scroll up.
    } finally {
      // Always reset: the ticket may have moved on mid-fetch, and a stuck
      // flag would wedge the sentinel forever.
      wLoadingEarlier = false;
    }
  }

  // The worker Session starts once the Run snapshot names it; a new submit
  // cycle keeps the same Session, so reload only when the id changes.
  $effect(() => {
    const id = ticket?.workerSession ?? '';
    if (id !== loadedWorkerFor) {
      loadedWorkerFor = id;
      loadWorker(id);
    }
  });

  async function sendToWorker(text, mode, images = [], imageIds = []) {
    const id = ticket?.workerSession;
    if (!id || sending) return;
    sending = true;
    const key = ++pendingKey;
    pendingSends = [...pendingSends, { key, text, mode, images: images.length, imageIds, state: 'sending' }];
    try {
      let res;
      // Only the worker takes messages; reviewers refuse with 409.
      if (images.length || imageIds.length) res = await postPrompt(id, text, { mode, images, imageIds });
      else if (mode === 'steer') res = await postSteer(id, text);
      else if (mode === 'followup') res = await postFollowup(id, text);
      else res = await postPrompt(id, text);
      if (res?.queued && !res?.seq) pendingSends = pendingSends.filter((p) => p.key !== key);
      else pendingSends = pendingSends.map((p) => (p.key === key ? { ...p, state: 'sent' } : p));
    } catch (err) {
      pendingSends = pendingSends.filter((p) => p.key !== key);
      throw err;
    } finally {
      sending = false;
    }
  }

  // The ticket page owns its snapshot: every action answers the fresh Run,
  // applied here at once (the SSE would bring it a beat later).
  function applyActionSnapshot(body) {
    const next = body?.run ?? body;
    if (next?.id) run = next;
  }

  const ticketActionable = $derived(
    !!ticket && ticket.state !== 'merged' && ticket.state !== 'discarded' && !runFinished(run),
  );

  async function runTicketAction(fn, failed) {
    if (!runId || !ticketId || acting) return;
    acting = true;
    actionError = '';
    try {
      applyActionSnapshot(await fn(runId, ticketId));
    } catch {
      actionError = failed;
    } finally {
      acting = false;
    }
  }

  async function doSteer() {
    const text = steerText.trim();
    if (!text || acting) return;
    acting = true;
    actionError = '';
    try {
      applyActionSnapshot(await steerTicket(runId, ticketId, text));
      steerText = '';
      steerOpen = false;
    } catch {
      actionError = 'Could not steer the worker.';
    } finally {
      acting = false;
    }
  }

  function doRetry() {
    return runTicketAction((r, t) => retryTicket(r, t), 'Could not retry the ticket.');
  }

  function doPark() {
    // A parked ticket resumes through retry, which clears the park.
    if (ticket?.state === 'parked') return doRetry();
    return runTicketAction((r, t) => parkTicket(r, t), 'Could not park the ticket.');
  }

  function doDiscard() {
    confirmDiscard = false;
    return runTicketAction((r, t) => discardTicket(r, t), 'Could not discard the ticket.');
  }

  async function openDrawer(agent) {
    const token = ++dToken;
    closeDrawerStream();
    drawer = agent;
    dEvents = [];
    dLastSeq = 0;
    dHasMore = false;
    dFirstSeq = 0;
    dLoadingEarlier = false;
    dError = '';
    try {
      const body = await getSession(agent.sessionId, { tail: TAIL_TURNS });
      if (disposed || token !== dToken || drawer?.sessionId !== agent.sessionId) return;
      dEvents = body.events ?? [];
      dHasMore = body.hasMore ?? false;
      dFirstSeq = body.firstSeq ?? 0;
      dLastSeq = dEvents.reduce((m, e) => Math.max(m, e.seq ?? 0), 0);
      if (typeof EventSource === 'undefined') return;
      if (disposed || token !== dToken || drawer?.sessionId !== agent.sessionId) return;
      dSource = openStream(agent.sessionId, dLastSeq, (e) => {
        if (disposed || token !== dToken) return;
        if (e.seq <= dLastSeq || drawer?.sessionId !== agent.sessionId) return;
        dLastSeq = e.seq;
        dEvents = [...dEvents, e];
      });
    } catch {
      if (!disposed && token === dToken && drawer?.sessionId === agent.sessionId) dError = 'Could not load that transcript.';
    }
  }

  function closeDrawer() {
    dToken++;
    closeDrawerStream();
    drawer = null;
    dEvents = [];
  }

  async function loadDrawerEarlier() {
    const id = drawer?.sessionId;
    if (!id || !dHasMore || dLoadingEarlier) return;
    dLoadingEarlier = true;
    const first = dFirstSeq;
    try {
      const body = await getSession(id, { before: first, turns: TAIL_TURNS });
      if (disposed || id !== drawer?.sessionId) return;
      dEvents = [...(body.events ?? []), ...dEvents];
      dHasMore = body.hasMore ?? false;
      dFirstSeq = body.firstSeq ?? dFirstSeq;
    } catch {
      // The sentinel stays: retry on the next scroll up.
    } finally {
      // Always reset: the drawer may show another agent by now.
      dLoadingEarlier = false;
    }
  }

  function openAgent(a) {
    if (!a?.sessionId) return;
    if (a.role === 'worker') onOpenSession?.(a.sessionId);
    else openDrawer(a);
  }

  function goBack() {
    if (runId) onNavigate?.(`/runs/${runId}`);
    else onNavigate?.('/runs');
  }

  onMount(() => {
    disposed = false;
    loadRun();
  });

  onDestroy(() => {
    // Invalidate every in-flight fetch: its resolution must not touch state
    // or open a stream after unmount or a {#key} route change.
    disposed = true;
    rToken++;
    wToken++;
    dToken++;
    closeRunStream();
    closeWorkerStream();
    closeDrawerStream();
  });

  // Inline markdown (code spans, emphasis) for one-line text such as the
  // acceptance boxes; the output is sanitized before it reaches {@html}.
  function inlineMd(text) {
    return DOMPurify.sanitize(marked.parseInline(String(text ?? '')));
  }
</script>

<div class="min-h-0 flex-1 overflow-auto">
<div class="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6" data-testid="ticket-page">
  <div class="flex items-center gap-3">
    <Button variant="ghost" size="sm" onclick={goBack}><ChevronLeft />Run</Button>
    <div class="min-w-0 flex-col leading-tight">
      <h2 class="truncate text-lg font-semibold tracking-tight" data-testid="ticket-title">
        {#if ticket}{ticket.num ? `#${ticket.num} ` : ''}{ticket.title || ticket.id}{:else}Ticket {ticketId}{/if}
      </h2>
      {#if ticket}
        <p class="truncate text-xs text-muted-foreground">
          {ticket.id}{ticket.branch ? ` · ${ticket.branch}` : ''}{ticket.tip ? ` · tip ${shortTip(ticket.tip)}` : ''}
        </p>
      {/if}
    </div>
    {#if ticket?.stateSince}
      <span class="ml-auto shrink-0 text-xs text-muted-foreground" title={ticket.stateSince}>
        {track.currentLabel} · {sinceLabel(ticket.stateSince)}
      </span>
    {/if}
  </div>

  {#if runStreamDown || wStreamDown}
    <p role="status" class="flex items-center gap-2 rounded-xl border px-4 py-2 text-sm text-muted-foreground" data-testid="stream-reconnecting">
      <span class="size-1.5 animate-pulse rounded-full bg-amber-400"></span>Reconnecting to live updates…
    </p>
  {/if}
  {#if runError}
    <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{runError}</p>
  {/if}
  {#if ticket?.needsYou}
    <p role="alert" class="rounded-xl border border-amber-400/30 bg-amber-400/10 px-4 py-2 text-sm text-amber-200" data-testid="needs-you">
      {ticket.needsYou}
    </p>
  {:else if ticket?.lastRefusal}
    <p class="rounded-xl border px-4 py-2 text-sm text-muted-foreground" data-testid="last-refusal">{ticket.lastRefusal}</p>
  {/if}

  {#if ticketActionable}
    <div class="flex flex-wrap items-center gap-2" data-testid="ticket-actions">
      {#if steerOpen}
        <form
          class="flex min-w-0 flex-1 items-end gap-2"
          onsubmit={(e) => {
            e.preventDefault();
            doSteer();
          }}
        >
          <FloatingInput class="min-w-0 flex-1" label="Steer the worker" bind:value={steerText} placeholder="What should the worker do differently?" />
          <Button type="submit" size="sm" disabled={acting || !steerText.trim()}>Send steer</Button>
          <Button type="button" size="sm" variant="ghost" onclick={() => (steerOpen = false)}>Cancel</Button>
        </form>
      {:else}
        <Button size="sm" variant="outline" disabled={acting || !ticket?.workerSession} onclick={() => (steerOpen = true)}>Steer</Button>
        <Button size="sm" variant="outline" disabled={acting} onclick={doRetry}>Retry</Button>
        <Button size="sm" variant="outline" disabled={acting} onclick={doPark}>
          {ticket?.state === 'parked' ? 'Unpark' : 'Park'}
        </Button>
        {#if confirmDiscard}
          <span class="flex items-center gap-2">
            <Button size="sm" variant="destructive" disabled={acting} onclick={doDiscard}>Confirm discard</Button>
            <Button size="sm" variant="ghost" onclick={() => (confirmDiscard = false)}>Cancel</Button>
          </span>
        {:else}
          <Button size="sm" variant="ghost" disabled={acting} onclick={() => (confirmDiscard = true)}>Discard</Button>
        {/if}
      {/if}
    </div>
    {#if actionError}
      <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{actionError}</p>
    {/if}
  {/if}

  <!-- State track: the real row states. -->
  {#if ticket}
    <div class="flex flex-wrap items-center gap-1.5 text-xs" data-testid="state-track" aria-label="Ticket state">
      {#each track.steps as s, i (s.id)}
        {#if i > 0}<span class="text-muted-foreground" aria-hidden="true">›</span>{/if}
        {#if s.state === 'current'}
          {@const label = s.id === 'in-review' && roundNumber(ticket) > 0 ? `in review · round ${roundNumber(ticket)}` : s.id === 'in-review' ? 'in review' : s.label}
          <span
            class="rounded-full px-3 py-1 font-medium {track.alert?.kind === 'needs-you'
              ? 'bg-amber-500/15 text-amber-300'
              : ticket.state === 'merged'
                ? 'bg-green-500/15 text-green-300'
                : 'bg-blue-500/15 text-blue-300'}"
          >
            {label}
          </span>
        {:else if s.state === 'done'}
          <span class="flex items-center gap-1 px-2 py-1 text-muted-foreground"><Check class="size-3" />{s.label}</span>
        {:else}
          <span class="px-2 py-1 text-muted-foreground">{s.label}</span>
        {/if}
      {/each}
      {#if track.alert}
        <Badge variant={track.alert.kind === 'needs-you' ? 'warning' : 'secondary'} class="ml-auto">{track.alert.label}</Badge>
      {/if}
    </div>
  {/if}

  {#if !run && !runError}
    <p class="rounded-xl border px-4 py-2 text-sm text-muted-foreground">Loading the ticket…</p>
  {:else if run && !ticket}
    <div class="flex flex-col items-start gap-3">
      <p class="rounded-xl border px-4 py-2 text-sm text-muted-foreground">Ticket {ticketId} is not on this Run.</p>
      <Button variant="secondary" size="sm" onclick={goBack}>Back to the Run</Button>
    </div>
  {:else if ticket}
    <div class="flex min-h-0 flex-col gap-4 lg:flex-row">
      <!-- Main column: worker transcript with review rounds woven in. -->
      <div class="flex min-w-0 flex-1 flex-col gap-4">
        {#if ticket.body}
          {@const briefLines = ticket.body.split('\n')}
          {@const briefShown = briefOpen || briefLines.length <= 6 ? ticket.body : briefLines.slice(0, 6).join('\n')}
          <div class="rounded-2xl border bg-card/60 px-4 py-3 text-xs text-muted-foreground" data-testid="ticket-brief">
            <span class="font-medium text-foreground">Brief</span>
            <span class="mx-1" aria-hidden="true">·</span>
            <Markdown text={briefShown} />
            {#if briefLines.length > 6}
              <Button variant="ghost" size="sm" class="mt-1 h-auto p-0 text-xs max-md:min-h-11 max-md:px-2" onclick={() => (briefOpen = !briefOpen)}>
                {briefOpen ? 'Show less' : `Show ${briefLines.length - 6} more lines`}
              </Button>
            {/if}
          </div>
        {/if}

        {#if ticket.workerSession}
          <div class="overflow-hidden rounded-2xl border" data-testid="worker-transcript">
            <div class="border-b bg-sidebar/60 px-4 py-2 text-xs font-medium text-muted-foreground">
              Worker transcript
            </div>
            <div class="flex h-[50vh] min-h-0 flex-col">
              <Transcript
                blocks={wBlocks}
                hasMore={wHasMore}
                loadingEarlier={wLoadingEarlier}
                onLoadEarlier={loadWorkerEarlier}
                bottomPad={24}
                emptyHint="The worker has not written anything yet."
                sessionId={ticket.workerSession}
                sessions={[]}
              />
            </div>
          </div>
        {/if}

        <!-- Review rounds, oldest first: a divider plus two cards each. -->
        {#each ticket.rounds ?? [] as round (round.n)}
          {@const cards = reviewCards(round)}
          <div class="flex items-center gap-3" data-testid="round-divider">
            <div class="h-px flex-1 bg-border"></div>
            <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">
              review round {round.n} · tip {shortTip(round.tip)}
            </span>
            <div class="h-px flex-1 bg-border"></div>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2" data-testid="round-cards">
            {#each cards as card (card.type)}
              {@const agent = agentByRole[card.type]}
              {@const accent = harnessStyle(agent?.harness ?? '').accent}
              <div class="animate-rise overflow-hidden rounded-2xl border bg-card" data-testid="review-card">
                <div class="flex items-center gap-2 px-3.5 py-2.5">
                  <span class="size-2 shrink-0 rounded-full" style="background: {accent}"></span>
                  <span class="font-medium">{card.short}</span>
                  {#if card.verdict === 'pass' && card.held}
                    <span class="rounded-full bg-green-500/10 px-2 text-xs text-green-300">pass held</span>
                  {:else if card.verdict === 'pass'}
                    <span class="rounded-full bg-green-500/10 px-2 text-xs text-green-300">pass</span>
                  {:else if card.verdict === 'changes'}
                    <span class="rounded-full bg-amber-500/10 px-2 text-xs text-amber-300">changes</span>
                  {:else if card.verdict === 'pending'}
                    <span class="flex items-center gap-1.5 text-xs text-muted-foreground">
                      <span class="size-1.5 animate-pulse rounded-full bg-blue-500"></span>re-judging
                    </span>
                  {:else}
                    <span class="rounded-full bg-muted px-2 text-xs text-muted-foreground">{card.verdict}</span>
                  {/if}
                  {#if card.durationMs}
                    <span class="ml-auto text-xs text-muted-foreground">{Math.round(card.durationMs / 1000)}s</span>
                  {/if}
                </div>
                {#if card.verdict === 'pass' && card.held}
                  <div class="border-t bg-background/40 p-3 text-xs text-muted-foreground">
                    not re-run: pass still covers the tip
                  </div>
                {:else if (card.findings ?? []).length}
                  <ul class="space-y-1.5 border-t bg-background/40 p-3 text-xs">
                    {#each card.findings as f, j (f.file + ':' + f.line + ':' + f.problem + ':' + j)}
                      <li>
                        {#if f.severity === 'minor'}
                          <span class="rounded bg-muted px-1 text-muted-foreground">minor</span>
                        {:else}
                          <span class="rounded bg-red-500/10 px-1 text-red-300">{f.severity}</span>
                        {/if}
                        <span class="font-mono"> {f.file}:{f.line}</span>
                        <span class="text-muted-foreground"> {f.problem}</span>
                      </li>
                    {/each}
                  </ul>
                {:else if card.verdict === 'pass'}
                  <div class="border-t bg-background/40 p-3 text-xs text-muted-foreground">No findings.</div>
                {/if}
              </div>
            {/each}
          </div>
          {#if (round.sentFindings ?? []).length}
            <div class="animate-rise rounded-2xl border bg-card px-4 py-3 text-xs" data-testid="sent-findings-card">
              <div class="mb-2 font-medium">Review findings sent to worker</div>
              <ul class="space-y-2">
                {#each round.sentFindings as finding (finding.findingId)}
                  <li class="space-y-1 border-t pt-2 first:border-0 first:pt-0">
                    <div class="flex flex-wrap items-center gap-1.5">
                      <Badge variant={finding.blocking ? 'destructive' : 'secondary'}>{finding.blocking ? 'blocking' : 'non-blocking'}</Badge>
                      <span class="text-muted-foreground">{finding.reviewer}</span>
                      <code class="rounded bg-muted px-1">{finding.findingId}</code>
                      <span class="font-mono">{finding.file}:{finding.line}</span>
                      <span class="text-muted-foreground">{finding.severity}</span>
                    </div>
                    <p class="text-muted-foreground">{finding.problem}</p>
                    {#if finding.disposition}
                      <p data-testid="finding-disposition"><span class="font-medium text-foreground">Disposition:</span> {finding.disposition.disposition}{#if finding.disposition.reason} — {finding.disposition.reason}{/if}{#if finding.disposition.ticket} · Ticket {finding.disposition.ticket}{/if}</p>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
          {#if !(round.sentFindings ?? []).length && (round.declined ?? []).length}
            {#if round.sentToWorkerAt}
              <div class="flex items-center gap-3">
                <div class="h-px flex-1 bg-border"></div>
                <span class="rounded-full bg-muted px-3 py-0.5 text-xs text-muted-foreground">findings sent to worker</span>
                <div class="h-px flex-1 bg-border"></div>
              </div>
            {/if}
            <div class="animate-rise rounded-2xl border bg-card px-4 py-3 text-xs" data-testid="declined-card">
              <div class="mb-1 font-medium">Declined with reason</div>
              {#each round.declined as d, k (d + ':' + k)}
                <p class="text-muted-foreground">{d}</p>
              {/each}
            </div>
          {/if}
        {/each}

        {#if ticket.workerSession}
          <div class="pb-2" data-testid="worker-composer">
            <Composer
              status={workerStatus}
              busy={sending}
              sessionId={ticket.workerSession}
              onSend={sendToWorker}
              onStop={() => interruptSession(ticket.workerSession)}
            />
          </div>
        {/if}
      </div>

      <!-- Right panel: agents, acceptance, gate, follow-ups. -->
      <aside class="w-full shrink-0 space-y-3 text-xs lg:w-80" data-testid="ticket-panel">
        <div class="rounded-2xl border bg-card p-3" data-testid="agents">
          <div class="mb-2 text-muted-foreground">Agents</div>
          <div class="space-y-1">
            {#each agents as a (a.sessionId)}
              {@const accent = harnessStyle(a.harness ?? '').accent}
              <button
                type="button"
                onclick={() => openAgent(a)}
                class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
              >
                <span class="size-2 shrink-0 rounded-full" style="background: {accent}" title={a.harness || 'agent'}></span>
                <span class="min-w-0 flex-1 truncate font-medium">{a.name || a.sessionId.slice(0, 8)}</span>
                <span class="rounded-full bg-muted px-1.5 text-[10px] text-muted-foreground">{agentRoleLabel(a.role)}</span>
              </button>
            {/each}
          </div>
          <div class="mt-2 text-[11px] text-muted-foreground">Reviewers and scouts open read-only.</div>
        </div>

        <div class="rounded-2xl border bg-card p-3" data-testid="acceptance">
          <div class="mb-2 text-muted-foreground">Acceptance · {progress.done} of {progress.total}</div>
          {#if boxes.length}
            <ul class="space-y-1 text-sm">
              {#each boxes as b, i (b.text + ':' + i)}
                <li class="flex gap-2 {b.checked ? '' : 'text-muted-foreground'}">
                  {#if b.checked}<span class="text-green-400">✓</span>{:else}<span>○</span>{/if}<span class="min-w-0 [overflow-wrap:anywhere] [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.85em]">{@html inlineMd(b.text)}</span>
                </li>
              {/each}
            </ul>
          {:else}
            <p class="text-muted-foreground">No checkboxes on this ticket.</p>
          {/if}
        </div>

        <div class="rounded-2xl border bg-card p-3" data-testid="gate">
          <div class="mb-1 text-muted-foreground">Gate</div>
          {#if gate.length}
            <ul class="space-y-1 text-sm">
              {#each gate as g (g.type)}
                <li class="flex items-center gap-1.5">
                  <span>{g.short}</span>
                  {#if g.verdict === 'pass' && g.fresh}
                    <span class="text-green-400">pass</span>
                  {:else if g.verdict === 'pass'}
                    <span class="text-green-400">pass</span><span class="text-muted-foreground">· holds</span>
                  {:else if g.verdict === 'changes'}
                    <span class="text-amber-300">changes</span>
                  {:else}
                    <span class="text-amber-300">pending{#if ticket.tip} @ {shortTip(ticket.tip)}{/if}</span>
                  {/if}
                </li>
              {/each}
            </ul>
          {:else}
            <p class="text-muted-foreground">No reviewers required.</p>
          {/if}
        </div>

        <div class="rounded-2xl border bg-card p-3" data-testid="followups">
          <div class="mb-1 text-muted-foreground">Follow-ups from worker</div>
          {#if ticket.followups}
            <p class="text-sm whitespace-pre-wrap">{ticket.followups}</p>
          {:else}
            <p class="text-muted-foreground">None yet.</p>
          {/if}
        </div>
      </aside>
    </div>
  {/if}
</div>
</div>

<!-- Read-only drawer for a reviewer or scout: transcript only, no composer. -->
{#if drawer}
  <div class="fixed inset-0 z-50" data-testid="agent-drawer">
    <button type="button" aria-label="Close transcript" class="absolute inset-0 animate-in fade-in-0 bg-black/40 backdrop-blur-sm" onclick={closeDrawer}></button>
    <div class="absolute inset-y-0 right-0 flex w-full max-w-xl animate-in fade-in-0 slide-in-from-right-4 flex-col border-l bg-sidebar duration-300">
      <div class="flex items-center gap-2 border-b p-3">
        <span class="size-2 shrink-0 rounded-full" style="background: {harnessStyle(drawer.harness ?? '').accent}"></span>
        <span class="min-w-0 flex-1 truncate text-sm font-semibold">{drawer.name || drawer.sessionId.slice(0, 8)}</span>
        <Badge>{agentRoleLabel(drawer.role)}</Badge>
        <Button variant="ghost" size="icon-sm" onclick={closeDrawer} aria-label="Close"><X /></Button>
      </div>
      <p class="border-b px-4 py-2 text-xs text-amber-300">Read-only transcript. Reviewers cannot be steered.</p>
      {#if dError}
        <p role="alert" class="m-3 rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{dError}</p>
      {/if}
      <div class="flex min-h-0 flex-1 flex-col">
        <Transcript
          blocks={dBlocks}
          hasMore={dHasMore}
          loadingEarlier={dLoadingEarlier}
          onLoadEarlier={loadDrawerEarlier}
          bottomPad={16}
          emptyHint="No transcript yet."
          sessionId={drawer.sessionId}
          sessions={[]}
        />
      </div>
    </div>
  </div>
{/if}
