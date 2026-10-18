<!-- Project-scoped or global Run Ticket page: the worker transcript as the
     main column with review rounds woven in, plus the facts panel (agents,
     acceptance, gate, follow-ups). Realtime comes from the Run SSE snapshot
     and the worker Session stream; nothing here polls. Reviewers and scouts
     open read-only in a drawer with no composer. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import { Check, Circle, CircleAlert, CircleCheck, Pause, Play, Trash2 } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Chip from './ui/chip.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import IconButton from './ui/icon-button.svelte';
  import InfoHint from './ui/info-hint.svelte';
  import PageHeader from './ui/page-header.svelte';
  import Segmented from './ui/segmented.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import { icons } from './icons.js';
  import { createPhone } from './phone.svelte.js';
  import { people, personFor } from './people.svelte.js';
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
    nextStep,
    ticketTypeOf,
    blockersLabel,
  } from './ticketPage.js';
  import { buildPath } from './router.js';

  import FloatingInput from './ui/floating-input.svelte';

  let { runId = '', ticketId = '', projectId = null, onNavigate = null, onOpenSession = null } = $props();

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

  // Phone: Status, Transcript and Facts are tabs; desktop shows all three.
  const phone = createPhone();
  let tab = $state('status');
  const show = (name) => !phone.value || tab === name;
  const ticket = $derived(findTicket(run, ticketId));
  const step = $derived(nextStep(ticket, run));
  // The owner shows only on a multi-User host.
  const ownerName = $derived(people.list.length > 1 ? (personFor(run?.owner)?.username ?? '') : '');
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
    if (runId) {
      onNavigate?.(projectId == null
        ? buildPath({ name: 'run', runId })
        : buildPath({ name: 'project-run', projectId, runId }));
    } else {
      onNavigate?.(projectId == null ? '/runs' : buildPath({ name: 'project-runs', projectId }));
    }
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
<div class="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-6 md:px-6" data-testid="ticket-page">
  <div class="flex flex-col gap-1">
    <PageHeader
      title={ticket ? `${ticket.num ? `#${ticket.num} ` : ''}${ticket.title || ticket.id}` : `Ticket ${ticketId}`}
      titleAttrs={{ 'data-testid': 'ticket-title' }}
      icon={icons.ticket}
      onBack={goBack}
      backLabel="Run"
      class="animate-rise"
    />
    {#if ticket}
      <p class="truncate text-caption text-fg-3 max-md:pl-1">
        {ticket.id}{ticket.branch ? ` · ${ticket.branch}` : ''}{ticket.tip ? ` · tip ${shortTip(ticket.tip)}` : ''}
      </p>
    {/if}
  </div>

  {#if runError}
    <ErrorState title="Could not load the Run" detail={runError} onRetry={loadRun} />
  {/if}

  {#if !run && !runError}
    <div role="status" aria-label="Loading the ticket" class="flex flex-col gap-3">
      <Skeleton class="h-40 rounded-2xl" />
      <Skeleton class="h-14 rounded-2xl" />
    </div>
  {:else if run && !ticket}
    <EmptyState icon={icons.ticket} title={`Ticket ${ticketId} is not on this Run`}>
      {#snippet action()}
        <Button variant="secondary" onclick={goBack}>Back to the Run</Button>
      {/snippet}
    </EmptyState>
  {:else if ticket}
    <!-- Lead: status and the next action. Everything else sits below or behind a tab. -->
    <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="ticket-status">
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
        <Chip tone={step.tone} dot>{step.label}</Chip>
        {#if ticket.stateSince}
          <span class="text-caption tabular-nums text-fg-3" title={ticket.stateSince}>{sinceLabel(ticket.stateSince)}</span>
        {/if}
        {#if runStreamDown || wStreamDown}
          <span role="status" class="ml-auto flex items-center gap-1.5 text-caption text-fg-3" data-testid="stream-reconnecting">
            <span class="size-1.5 rounded-full bg-status-needs-you motion-safe:animate-pulse"></span>Reconnecting to live updates…
          </span>
        {/if}
      </div>
      {#if ticket.needsYou}
        <p role="alert" class="line-clamp-3 text-body text-status-needs-you" data-testid="needs-you">{ticket.needsYou}</p>
      {:else}
        <p class="text-body text-fg-1" data-testid="next-step">{step.next}</p>
        {#if ticket.lastRefusal}
          <p class="line-clamp-2 text-caption text-fg-3" data-testid="last-refusal">{ticket.lastRefusal}</p>
        {/if}
      {/if}
      <div class="flex flex-wrap items-center gap-1.5" data-testid="ticket-chips">
        <Chip tone={ticketTypeOf(ticket) === 'design' ? 'neutral' : 'outline'} icon={icons.ticket} title="Ticket type">{ticketTypeOf(ticket)}</Chip>
        {#if ticket.blockers?.length}
          <Chip tone="outline" title={`Blocked by ${ticket.blockers.join(', ')}`}>{blockersLabel(ticket.blockers)}</Chip>
        {/if}
        {#if ticket.warnings?.length}
          <Chip tone="needs-you" icon={CircleAlert}>{ticket.warnings.length} {ticket.warnings.length === 1 ? 'warning' : 'warnings'}</Chip>
          <InfoHint label="Chain warnings">
            <ul class="flex list-inside list-disc flex-col gap-1">{#each ticket.warnings as warning}<li>{warning}</li>{/each}</ul>
          </InfoHint>
        {/if}
        {#if ownerName}<Chip tone="outline" icon={icons.session} title="Run owner">{ownerName}</Chip>{/if}
      </div>

      {#if ticketActionable}
        <div class="flex flex-wrap items-center gap-2 border-t pt-3" data-testid="ticket-actions">
          {#if steerOpen}
            <form
              class="flex min-w-0 flex-1 flex-wrap items-end gap-2"
              onsubmit={(e) => {
                e.preventDefault();
                doSteer();
              }}
            >
              <FloatingInput class="min-w-0 flex-1 basis-60" label="Steer the worker" bind:value={steerText} placeholder="What should the worker do differently?" />
              <Button type="submit" disabled={acting || !steerText.trim()}>Send steer</Button>
              <Button type="button" variant="ghost" onclick={() => (steerOpen = false)}>Cancel</Button>
            </form>
          {:else}
            <Button variant={step.action === 'steer' ? 'default' : 'outline'} disabled={acting || !ticket?.workerSession} onclick={() => (steerOpen = true)}><icons.steer aria-hidden="true" />Steer</Button>
            {#if step.action === 'unpark'}
              <Button disabled={acting} onclick={doPark}><Play aria-hidden="true" />Unpark</Button>
            {/if}
            <div class="ml-auto flex items-center gap-1">
              <IconButton variant="outline" icon={icons.retry} label="Retry" disabled={acting} onclick={doRetry} />
              {#if step.action !== 'unpark'}
                <IconButton variant="outline" icon={Pause} label="Park" disabled={acting} onclick={doPark} />
              {/if}
              {#if confirmDiscard}
                <Button variant="destructive" disabled={acting} onclick={doDiscard}>Confirm discard</Button>
                <Button variant="ghost" onclick={() => (confirmDiscard = false)}>Cancel</Button>
              {:else}
                <IconButton variant="outline" icon={Trash2} label="Discard" disabled={acting} onclick={() => (confirmDiscard = true)} />
              {/if}
            </div>
          {/if}
        </div>
        {#if actionError}
          <ErrorState compact title={actionError} />
        {/if}
      {/if}
    </section>

    {#if phone.value}
      <Segmented mode="tabs" label="Ticket sections" panel="ticket-tabpanel" value={tab} onchange={(next) => (tab = next)} items={[{ value: 'status', label: 'Status' }, { value: 'transcript', label: 'Transcript' }, { value: 'facts', label: 'Facts' }]} />
    {/if}

    <!-- State track: the real row states. -->
    {#if show('status')}
      <div class="flex flex-wrap items-center gap-1.5 text-caption" data-testid="state-track" aria-label="Ticket state">
        {#each track.steps as s, i (s.id)}
          {#if i > 0}<span class="text-fg-4" aria-hidden="true">›</span>{/if}
          {#if s.state === 'current'}
            {@const label = s.id === 'in-review' && roundNumber(ticket) > 0 ? `in review · round ${roundNumber(ticket)}` : s.id === 'in-review' ? 'in review' : s.label}
            <span
              class="rounded-full px-3 py-1 font-medium {track.alert?.kind === 'needs-you'
                ? 'bg-status-needs-you/12 text-status-needs-you'
                : ticket.state === 'merged'
                  ? 'bg-status-done/12 text-status-done'
                  : 'bg-status-working/12 text-status-working'}"
            >
              {label}
            </span>
          {:else if s.state === 'done'}
            <span class="flex items-center gap-1 px-2 py-1 text-fg-3"><Check class="size-3" aria-hidden="true" />{s.label}</span>
          {:else}
            <span class="px-2 py-1 text-fg-3">{s.label}</span>
          {/if}
        {/each}
        {#if track.alert}
          <Chip tone={track.alert.kind === 'needs-you' ? 'needs-you' : 'neutral'} class="ml-auto">{track.alert.label}</Chip>
        {/if}
      </div>
    {/if}

    <div id={phone.value ? 'ticket-tabpanel' : undefined} class="flex min-h-0 flex-col gap-4 lg:flex-row" role={phone.value ? 'tabpanel' : undefined} aria-label={phone.value ? tab : undefined}>
      <!-- Main column: worker transcript with review rounds woven in. -->
      <div class="flex min-w-0 flex-1 flex-col gap-4">
        {#if show('status') && ticket.body}
          {@const briefLines = ticket.body.split('\n')}
          {@const briefShown = briefOpen || briefLines.length <= 6 ? ticket.body : briefLines.slice(0, 6).join('\n')}
          <div class="rounded-2xl border bg-card/60 px-4 py-3 text-caption text-fg-3" data-testid="ticket-brief">
            <span class="font-medium text-fg-1">Brief</span>
            <span class="mx-1" aria-hidden="true">·</span>
            <Markdown text={briefShown} />
            {#if briefLines.length > 6}
              <Button variant="ghost" size="sm" class="mt-1 h-auto p-0 text-caption max-md:min-h-11 max-md:px-2" onclick={() => (briefOpen = !briefOpen)}>
                {briefOpen ? 'Show less' : `Show ${briefLines.length - 6} more lines`}
              </Button>
            {/if}
          </div>
        {/if}

        {#if show('transcript') && ticket.workerSession}
          <div class="overflow-hidden rounded-2xl border bg-card shadow-elev-1" data-testid="worker-transcript">
            <div class="flex items-center gap-2 border-b bg-sidebar/60 px-4 py-2 text-caption font-medium text-fg-3">
              <icons.session class="size-3.5" aria-hidden="true" />Worker transcript
            </div>
            <div class="flex h-[60dvh] min-h-0 flex-col md:h-[50vh]">
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
        {:else if show('transcript') && phone.value}
          <EmptyState icon={icons.session} title="No worker yet" description="The transcript appears once the worker starts." />
        {/if}

        <!-- Review rounds, oldest first: a divider plus two cards each. -->
        {#if show('status')}
        {#each ticket.rounds ?? [] as round (round.n)}
          {@const cards = reviewCards(round)}
          <div class="flex items-center gap-3" data-testid="round-divider">
            <div class="h-px flex-1 bg-border"></div>
            <span class="rounded-full bg-muted px-3 py-0.5 text-caption tabular-nums text-fg-3">
              review round {round.n} · tip {shortTip(round.tip)}
            </span>
            <div class="h-px flex-1 bg-border"></div>
          </div>
          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2" data-testid="round-cards">
            {#each cards as card (card.type)}
              {@const agent = agentByRole[card.type]}
              {@const accent = harnessStyle(agent?.harness ?? '').accent}
              <div class="animate-rise overflow-hidden rounded-2xl border bg-card shadow-elev-1" data-testid="review-card">
                <div class="flex items-center gap-2 px-3.5 py-2.5">
                  <span class="size-2 shrink-0 rounded-full" style="background: {accent}"></span>
                  <span class="text-body font-medium">{card.short}</span>
                  {#if card.verdict === 'pass' && card.held}
                    <Chip tone="done">pass held</Chip>
                  {:else if card.verdict === 'pass'}
                    <Chip tone="done">pass</Chip>
                  {:else if card.verdict === 'changes'}
                    <Chip tone="needs-you">changes</Chip>
                  {:else if card.verdict === 'pending'}
                    <Chip tone="working" dot>re-judging</Chip>
                  {:else}
                    <Chip>{card.verdict}</Chip>
                  {/if}
                  {#if card.durationMs}
                    <span class="ml-auto text-caption tabular-nums text-fg-3">{Math.round(card.durationMs / 1000)}s</span>
                  {/if}
                </div>
                {#if card.verdict === 'pass' && card.held}
                  <div class="border-t bg-background/40 p-3 text-caption text-fg-3">
                    not re-run: pass still covers the tip
                  </div>
                {:else if (card.findings ?? []).length}
                  <ul class="flex flex-col gap-2 border-t bg-background/40 p-3 text-caption">
                    {#each card.findings as f, j (f.file + ':' + f.line + ':' + f.problem + ':' + j)}
                      <li class="[overflow-wrap:anywhere]">
                        {#if f.severity === 'minor'}
                          <span class="rounded bg-muted px-1 text-fg-3">minor</span>
                        {:else}
                          <span class="rounded bg-status-failed/12 px-1 text-status-failed">{f.severity}</span>
                        {/if}
                        <span class="font-mono"> {f.file}:{f.line}</span>
                        <span class="text-fg-3"> {f.problem}</span>
                      </li>
                    {/each}
                  </ul>
                {:else if card.verdict === 'pass'}
                  <EmptyState compact icon={icons.report} title="No findings" description="This review passed without notes." class="border-0" />
                {/if}
              </div>
            {/each}
          </div>
          {#if (round.sentFindings ?? []).length}
            <div class="animate-rise rounded-2xl border bg-card px-4 py-3 text-caption shadow-elev-1" data-testid="sent-findings-card">
              <div class="mb-2 text-body font-medium">Review findings sent to worker</div>
              <ul class="flex flex-col gap-2">
                {#each round.sentFindings as finding (finding.findingId)}
                  <li class="flex flex-col gap-1 border-t pt-2 first:border-0 first:pt-0">
                    <div class="flex flex-wrap items-center gap-1.5">
                      <Chip tone={finding.blocking ? 'failed' : 'neutral'}>{finding.blocking ? 'blocking' : 'non-blocking'}</Chip>
                      <span class="text-fg-3">{finding.reviewer}</span>
                      <code class="rounded bg-muted px-1 [overflow-wrap:anywhere]">{finding.findingId}</code>
                      <span class="font-mono [overflow-wrap:anywhere]">{finding.file}:{finding.line}</span>
                      <span class="text-fg-3">{finding.severity}</span>
                    </div>
                    <p class="text-fg-3 [overflow-wrap:anywhere]">{finding.problem}</p>
                    {#if finding.disposition}
                      <p data-testid="finding-disposition"><span class="font-medium text-fg-1">Disposition:</span> {finding.disposition.disposition}{#if finding.disposition.reason} — {finding.disposition.reason}{/if}{#if finding.disposition.ticket} · Ticket {finding.disposition.ticket}{/if}</p>
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
                <span class="rounded-full bg-muted px-3 py-0.5 text-caption text-fg-3">findings sent to worker</span>
                <div class="h-px flex-1 bg-border"></div>
              </div>
            {/if}
            <div class="animate-rise rounded-2xl border bg-card px-4 py-3 text-caption shadow-elev-1" data-testid="declined-card">
              <div class="mb-1 text-body font-medium">Declined with reason</div>
              {#each round.declined as d, k (d + ':' + k)}
                <p class="text-fg-3">{d}</p>
              {/each}
            </div>
          {/if}
        {/each}
        {/if}

        {#if show('transcript') && ticket.workerSession}
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

      <!-- Facts: agents, acceptance, gate, follow-ups. -->
      {#if show('facts')}
      <aside class="flex w-full shrink-0 flex-col gap-3 text-body lg:w-80" data-testid="ticket-panel">
        <div class="rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="agents">
          <div class="mb-2 flex items-center gap-1">
            <h3 class="text-title text-fg-1">Agents</h3>
            <InfoHint label="About agents">Reviewers and scouts open read-only.</InfoHint>
          </div>
          <div class="flex flex-col gap-0.5">
            {#each agents as a (a.sessionId)}
              {@const accent = harnessStyle(a.harness ?? '').accent}
              <button
                type="button"
                onclick={() => openAgent(a)}
                class="motion-standard flex min-h-9 w-full items-center gap-2 rounded-lg px-2 py-1.5 text-left text-body outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11"
              >
                <span class="size-2 shrink-0 rounded-full" style="background: {accent}" title={a.harness || 'agent'}></span>
                <span class="min-w-0 flex-1 truncate font-medium">{a.name || a.sessionId.slice(0, 8)}</span>
                <Chip>{agentRoleLabel(a.role)}</Chip>
              </button>
            {/each}
          </div>
        </div>

        <div class="rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="acceptance">
          <h3 class="mb-2 text-title text-fg-1 tabular-nums">Acceptance · {progress.done} of {progress.total}</h3>
          {#if boxes.length}
            <ul class="flex flex-col gap-1.5 text-body">
              {#each boxes as b, i (b.text + ':' + i)}
                <li class="flex gap-2 {b.checked ? 'text-fg-1' : 'text-fg-3'}">
                  {#if b.checked}<CircleCheck class="mt-0.5 size-4 shrink-0 text-status-done" role="img" aria-label="Done" />{:else}<Circle class="mt-0.5 size-4 shrink-0 text-fg-4" role="img" aria-label="Open" />{/if}<span class="min-w-0 [overflow-wrap:anywhere] [&_code]:rounded [&_code]:bg-muted [&_code]:px-1 [&_code]:font-mono [&_code]:text-[0.85em]">{@html inlineMd(b.text)}</span>
                </li>
              {/each}
            </ul>
          {:else}
            <EmptyState compact icon={icons.ticket} title="No checkboxes on this ticket" class="border-0 px-0" />
          {/if}
        </div>

        <div class="rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="gate">
          <h3 class="mb-2 text-title text-fg-1">Gate</h3>
          {#if gate.length}
            <ul class="flex flex-col gap-1.5 text-body">
              {#each gate as g (g.type)}
                <li class="flex items-center gap-2">
                  <span>{g.short}</span>
                  {#if g.verdict === 'pass' && g.fresh}
                    <Chip tone="done">pass</Chip>
                  {:else if g.verdict === 'pass'}
                    <Chip tone="done">pass</Chip><span class="text-caption text-fg-3">· holds</span>
                  {:else if g.verdict === 'changes'}
                    <Chip tone="needs-you">changes</Chip>
                  {:else}
                    <Chip tone="needs-you">pending{#if ticket.tip} @ {shortTip(ticket.tip)}{/if}</Chip>
                  {/if}
                </li>
              {/each}
            </ul>
          {:else}
            <EmptyState compact icon={icons.subagent} title="No reviewers required" class="border-0 px-0" />
          {/if}
        </div>

        <div class="rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="followups">
          <h3 class="mb-2 text-title text-fg-1">Follow-ups from worker</h3>
          {#if ticket.followups}
            <p class="whitespace-pre-wrap text-body [overflow-wrap:anywhere]">{ticket.followups}</p>
          {:else}
            <EmptyState compact icon={icons.subagent} title="No follow-ups yet" class="border-0 px-0" />
          {/if}
        </div>
      </aside>
      {/if}
    </div>
  {/if}
</div>
</div>

<!-- Read-only drawer for a reviewer or scout: transcript only, no composer. -->
{#if drawer}
  <div class="fixed inset-0 z-50" data-testid="agent-drawer">
    <button type="button" aria-label="Close transcript" class="motion-emphasis absolute inset-0 animate-in fade-in-0 bg-black/40 backdrop-blur-sm" onclick={closeDrawer}></button>
    <div class="motion-emphasis absolute inset-y-0 right-0 flex w-full max-w-xl animate-in fade-in-0 slide-in-from-right-4 flex-col border-l bg-sidebar pb-[env(safe-area-inset-bottom)] pt-[env(safe-area-inset-top)] shadow-elev-3">
      <div class="flex items-center gap-2 border-b p-3">
        <span class="size-2 shrink-0 rounded-full" style="background: {harnessStyle(drawer.harness ?? '').accent}"></span>
        <span class="min-w-0 flex-1 truncate text-title text-fg-1">{drawer.name || drawer.sessionId.slice(0, 8)}</span>
        <Chip>{agentRoleLabel(drawer.role)}</Chip>
        <IconButton icon={icons.dismiss} label="Close" onclick={closeDrawer} />
      </div>
      <p class="border-b px-4 py-2 text-caption text-status-needs-you">Read-only transcript. Reviewers cannot be steered.</p>
      {#if dError}
        <ErrorState compact title={dError} class="m-3" />
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
