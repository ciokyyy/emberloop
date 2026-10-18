<!-- Runs view: the main area owns the picked spec's ticket preview, the Run
     start button and the live Tickets as a dependency graph or table.
     Spec discovery (repo directory, spec list, Find specs form, preview
     pick) lives in the rail-2 RunsSidebar and shares this view's
     `runsDiscovery` store, so a sidebar pick repaints this area. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import { Dialog } from 'bits-ui';
  import { CircleAlert, List, ListChecks, SlidersHorizontal, Workflow } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Chip from './ui/chip.svelte';
  import IconButton from './ui/icon-button.svelte';
  import InfoHint from './ui/info-hint.svelte';
  import Segmented from './ui/segmented.svelte';
  import BottomSheet from './ui/bottom-sheet.svelte';
  import Row from './ui/row.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import PageHeader from './ui/page-header.svelte';
  import { icons } from './icons.js';
  import { createPhone } from './phone.svelte.js';
  import { people, personFor } from './people.svelte.js';
  import { ticketTypeOf, blockersLabel } from './ticketPage.js';
  import RunsGraph from './RunsGraph.svelte';
  import { startRun, getRun, openRunStream, pauseRun, resumeRun, pauseTicket as pauseRunTicket, resumeTicket as resumeRunTicket, switchRun as switchRunBackend, switchTicket as switchTicketBackend, rebaseRun, abandonRun, setRunTicketType } from './api.js';
  import RunSwitchPicker from './RunSwitchPicker.svelte';
  import { soundPlayer, runTransitions, seedRun } from './sound.js';
  import {
    discovery,
    initDiscoveryRepo,
    ensureDiscoveryContext,
  } from './runsDiscovery.svelte.js';
  import { buildPath } from './router.js';

  // Run sounds: a blocked cue when a ticket turns needs-you, a complete cue
  // when a ticket merges or the Run finishes. The first snapshot of a Run
  // seeds silently (opening replays history; history is not news), and a
  // push arriving over the service worker for the same key sounds once.
  const sounds = soundPlayer();
  let prevRun = null;
  function applyRun(next) {
    if (next?.tickets) runError = '';
    run = next;
    // A deep-linked Run seeds the sidebar spec picker from its own
    // repo/spec.
    void ensureDiscoveryContext(run);
    if (!next?.id) {
      // The Run cleared (new pick, route away): forget the snapshot so a
      // reopened Run seeds silently instead of replaying stale news.
      prevRun = null;
      return;
    }
    if (!prevRun || prevRun.id !== next.id) {
      prevRun = { id: next.id, snap: seedRun(next) };
      return;
    }
    const snap = seedRun(next);
    for (const t of runTransitions(prevRun.snap, next)) sounds?.playEvent(t.event, { key: t.key });
    prevRun = { id: next.id, snap };
  }

  const merged = $derived((run?.tickets ?? []).filter((t) => t.state === 'merged').length);
  // Chip tones: a Run or Ticket status maps onto the five status tokens.
  const runTone = (status) => (status === 'done' ? 'done' : status === 'failed' ? 'failed' : status === 'paused' ? 'paused' : status === 'needs-you' ? 'needs-you' : status === 'running' ? 'working' : 'neutral');
  const stateTone = (state) => (state === 'merged' ? 'done' : state === 'needs-you' ? 'needs-you' : state === 'paused' ? 'paused' : ['running', 'in-review', 'merging'].includes(state) ? 'working' : 'neutral');
  const reviewerLabel = (type) => (type === 'reviewer-spec' ? 'spec' : type === 'reviewer-standards' ? 'standards' : String(type));
  const chainRoles = [
    { role: 'worker', label: 'Worker' },
    { role: 'reviewer-spec', label: 'Spec review' },
    { role: 'reviewer-standards', label: 'Standards' },
  ];
  const activeChainEntries = (ticket) => chainRoles.flatMap((role) => {
    const entry = ticket.chain?.[role.role];
    return entry ? [{ ...role, ...entry }] : [];
  });
  const ticketFinished = (ticket) => ['merged', 'discarded', 'parked'].includes(ticket.state);
  const boxCount = (t) => {
    const boxes = t.boxes ?? [];
    return boxes.length ? `${boxes.filter((b) => b.checked).length}/${boxes.length}` : '—';
  };

  let { workDir = '', onOpenSession = null, runId = '', ticketId = '', projectId = null, onNavigate = null, onOpenMenu = null, scopeLabel = '' } = $props();

  const scopedProject = $derived(projectId != null && projectId !== 'none');

  const runPath = (id, childTicketId = '') => projectId == null
    ? buildPath(childTicketId ? { name: 'ticket', runId: id, ticketId: childTicketId } : { name: 'run', runId: id })
    : buildPath(childTicketId
      ? { name: 'project-ticket', projectId, runId: id, ticketId: childTicketId }
      : { name: 'project-run', projectId, runId: id });

  // The sidebar owns the repo default; the main area only fills a blank
  // store (a cold deep link with no session context), never clobbering a
  // typed repo.
  initDiscoveryRepo(workDir);
  let run = $state(null);
  let runError = $state('');
  let previewTypeOverrides = $state({});
  let runLoading = $state(false);
  let starting = $state(false);
  let acting = $state(false);
  let confirmAbandon = $state(false);
  // Tickets layout: the dependency graph is the desktop default. Phones list
  // the Tickets and open the graph full screen on request: the wide canvas
  // never sits inside the scrolling page.
  const phone = createPhone();
  let graphView = $state('graph');
  let graphFull = $state(false);
  let sheetTicketId = $state('');
  let sheetOpen = $state(false);
  // The owner shows only on a multi-User host.
  const ownerName = $derived(people.list.length > 1 ? (personFor(run?.owner)?.username ?? '') : '');
  const view = $derived(phone.value ? 'list' : graphView);
  const sheetTicket = $derived((run?.tickets ?? []).find((t) => t.id === sheetTicketId) ?? null);
  function pickView(next) {
    if (phone.value && next === 'graph') graphFull = true;
    else graphView = next;
  }
  // Tickets waving the needs-you banner: the driver marks them after the
  // same refusal three times, ten idle minutes or three stall nudges.
  const needsYouTickets = $derived((run?.tickets ?? []).filter((t) => t.state !== 'discarded' && t.state !== 'parked' && t.state !== 'merged' && (t.state === 'needs-you' || t.needsYou)));
  let runSource = null;
  // Stream health: the Run EventSource error hook flips this until the next
  // snapshot lands, so the view can show a reconnecting indicator.
  let runStreamDown = $state(false);
  // Generation token plus a disposed flag: a getRun resolving after unmount
  // or a fast runA -> runB navigation must never overwrite the Run or leave
  // a stale subscription behind.
  let openToken = 0;
  let disposed = false;

  function closeRunStream() {
    if (runSource) runSource.close();
    runSource = null;
  }

 	// Completed, failed and abandoned Runs stop changing; paused and needs-you
 	// Runs stay subscribed so Resume can wake the view.
 	function liveDone(r) {
 		return r && (r.status === 'done' || r.status === 'failed' || r.status === 'abandoned');
  }

  // A sidebar spec pick drops a live Run started from another spec: the
  // preview below replaces it. Seeded context (same spec) keeps the Run.
  let seenPick = discovery.pickNonce;
  $effect(() => {
    const nonce = discovery.pickNonce;
    if (nonce === seenPick) return;
    seenPick = nonce;
    if (discovery.selected && run && run.spec && discovery.selected !== run.spec) {
      openToken++;
      closeRunStream();
      run = null;
      runLoading = false;
      runError = '';
      runStreamDown = false;
      prevRun = null;
    }
  });

  async function refreshRun(token) {
    if (!run?.id) return false;
    const id = run.id;
    try {
      const body = await getRun(id);
      if (disposed || (token !== undefined && token !== openToken)) return false;
      if (run?.id !== id) return false;
      applyRun(body.run ?? body);
      if (liveDone(run)) closeRunStream();
      return true;
    } catch {
      // The view keeps its loading state; the caller reports a failed detail read.
      return false;
    }
  }

  // Deep links to `/runs/<runId>` or `/p/<projectId>/runs/<runId>` load that Run.
  // token keeps a fast runA -> runB navigation from letting A's late answer
  // overwrite B or double-subscribe the stream.
  async function openRun(id) {
    if (!id) return;
    confirmAbandon = false;
    const token = ++openToken;
    closeRunStream();
    runStreamDown = false;
    run = { id };
    runError = '';
    runLoading = true;
    const loaded = await refreshRun(token);
    if (disposed || token !== openToken) return;
    if (run?.id !== id) return;
    runLoading = false;
    if (!loaded && !run?.tickets) runError = 'Could not load Run details.';
    subscribeRun(id, token);
  }

  // Realtime from the Run SSE, no polling: the Daemon publishes a fresh
  // snapshot on every change (box tick, verdict, round, state, agent).
  function subscribeRun(id, token) {
    if (token !== undefined && token !== openToken) return;
    closeRunStream();
    if (disposed) return;
    if (typeof EventSource === 'undefined') return;
    try {
      runSource = openRunStream(
        id,
        (body) => {
          if (disposed) return;
          if (token !== undefined && token !== openToken) return;
          // A stale stream (runA after navigating to runB) never writes.
          if (run?.id && run.id !== id) return;
          runStreamDown = false;
          applyRun(body.run ?? body);
          if (liveDone(run)) closeRunStream();
        },
        { onError: () => (runStreamDown = true), onOpen: () => (runStreamDown = false) },
      );
    } catch {
      // The single refresh above still shows the current snapshot.
    }
  }

  onDestroy(() => {
    disposed = true;
    openToken++;
    closeRunStream();
  });


  async function start() {
    if (!discovery.selected || starting) return;
    const repo = scopedProject ? workDir : discovery.repo;
    if (!repo) return;
    starting = true;
    runError = '';
    try {
      const body = await startRun({ spec: discovery.selected, repo, ...(Object.keys(previewTypeOverrides).length ? { typeOverrides: previewTypeOverrides } : {}) });
      const startedRun = body.run ?? null;
      applyRun(startedRun);
      if (startedRun?.id) {
        onNavigate?.(runPath(startedRun.id));
        subscribeRun(startedRun.id);
      }
    } catch (e) {
      runError = e?.message?.includes('409') ? 'A run is already active for this spec.' : 'Could not start the Run.';
    } finally {
      starting = false;
    }
  }
  function previewType(ticket) {
    return previewTypeOverrides[ticket.id] || ticket.type || 'code';
  }

  function flipPreviewType(ticket) {
    const current = previewType(ticket);
    previewTypeOverrides = { ...previewTypeOverrides, [ticket.id]: current === 'design' ? 'code' : 'design' };
  }

  function clearPreviewType(ticket) {
    const { [ticket.id]: _removed, ...remaining } = previewTypeOverrides;
    previewTypeOverrides = remaining;
  }

  async function act(fn, failed) {
    if (!run?.id || acting) return;
    acting = true;
    runError = '';
    try {
      const body = await fn(run.id);
      applyRun(body.run ?? body);
      if (liveDone(run)) closeRunStream();
      else if (run?.id) subscribeRun(run.id);
    } catch (e) {
      if (e?.run) {
        applyRun(e.run);
        if (liveDone(run)) closeRunStream();
      }
      runError = failed;
    } finally {
      acting = false;
    }
  }

  async function actTicket(fn, ticket, failed) {
    if (!run?.id || acting) return;
    acting = true;
    runError = '';
    try {
      const body = await fn(run.id, ticket.id);
      applyRun(body.run ?? body);
      if (liveDone(run)) closeRunStream();
      else if (run?.id) subscribeRun(run.id);
    } catch (e) {
      if (e?.run) applyRun(e.run);
      runError = failed;
    } finally {
      acting = false;
    }
  }

  async function pauseTicketItem(ticket) {
    await actTicket(pauseRunTicket, ticket, 'Could not pause the Ticket.');
  }

  async function resumeTicketItem(ticket) {
    await actTicket(resumeRunTicket, ticket, 'Could not resume the Ticket.');
  }

  async function switchTicketItem(ticket, selection) {
    await actTicket((runID, ticketID) => switchTicketBackend(runID, ticketID, selection), ticket, 'Could not switch the Ticket Backend and model.');
  }

  async function changeTicketType(ticket, type) {
    if (!run?.id || ticket.workerSession || acting) return;
    acting = true;
    runError = '';
    try {
      const body = await setRunTicketType(run.id, ticket.id, type);
      applyRun(body.run ?? body);
    } catch (error) {
      runError = error?.message || 'Could not change Ticket type.';
    } finally {
      acting = false;
    }
  }
  async function switchRunSelection(selection) {
    await act((runID) => switchRunBackend(runID, selection), 'Could not switch the Run Backend and model.');
  }

  async function pause() {
    await act(pauseRun, 'Could not pause the Run.');
  }

  async function resume() {
    await act(resumeRun, 'Could not resume the Run.');
  }
  async function rebase() {
    await act(rebaseRun, 'Could not rebase the spec branch onto master.');
  }

  async function abandon() {
    confirmAbandon = false;
    await act(abandonRun, 'Could not abandon the Run.');
  }

  function clearRun() {
    openToken++;
    closeRunStream();
    run = null;
    runError = '';
    runLoading = false;
    runStreamDown = false;
    confirmAbandon = false;
    prevRun = null;
  }

  onMount(() => {
    if (ticketId && runId) return;
    if (runId) openRun(runId);
  });

  // Follow route changes without a remount: entering a ticket hands the Run
  // to the ticket page (which owns its own SSE stream, no polling), leaving
  // one seeds the spec context again through openRun.
  let routeSeen = '';
  $effect(() => {
    const key = `${projectId ?? ''}/${runId ?? ''}/${ticketId ?? ''}`;
    if (!routeSeen) {
      routeSeen = key;
      return;
    }
    if (key === routeSeen) return;
    routeSeen = key;
    if (ticketId && runId) {
      openToken++;
      closeRunStream();
      run = null;
      runError = '';
      runLoading = false;
      runStreamDown = false;
      return;
    }
    if (runId) {
      openRun(runId);
      return;
    }
    clearRun();
  });
  </script>


  {#snippet verdict(v)}
    {#if v}
      <Chip tone={v.verdict === 'pass' ? 'done' : 'needs-you'}>{v.verdict}{v.fresh ? '' : ' · stale'}</Chip>
    {:else}
      <span class="text-fg-3">—</span>
    {/if}
  {/snippet}

  <!-- Ticket type: a flip button until the Worker starts, a read-only chip after. -->
  {#snippet typeControl(t)}
    {@const type = ticketTypeOf(t)}
    {#if !t.workerSession}
      <Button size="sm" variant={type === 'design' ? 'secondary' : 'outline'} disabled={acting} title="Flip Ticket type before the Worker starts" onclick={(event) => { event.stopPropagation(); changeTicketType(t, type === 'design' ? 'code' : 'design'); }}>{type}</Button>
    {:else}
      <Chip tone={type === 'design' ? 'neutral' : 'outline'} title="Ticket type is read-only after the Worker starts">{type}</Chip>
    {/if}
    {#if t.typeOverride}
      <span class="flex items-center gap-1 text-caption text-fg-3">
        <span>overridden</span>
        {#if !t.workerSession}
          <Button size="sm" variant="ghost" class="px-2 text-caption" disabled={acting} title="Clear override and use the Ticket file type" onclick={(event) => { event.stopPropagation(); changeTicketType(t, ''); }}>Clear</Button>
        {/if}
      </span>
    {/if}
  {/snippet}

  <!-- Per-Ticket Run controls: Pause, or Resume with the Backend switch. -->
  {#snippet ticketControls(t)}
    {#if t.state === 'paused' && run.status === 'running'}
      <Button size="sm" variant="outline" class="w-fit" disabled={acting} onclick={(event) => { event.stopPropagation(); resumeTicketItem(t); }}>Resume {t.title || 'Ticket'}</Button>
    {:else if run.status === 'running' && !ticketFinished(t) && t.state !== 'paused'}
      <Button size="sm" variant="outline" class="w-fit" disabled={acting} onclick={(event) => { event.stopPropagation(); pauseTicketItem(t); }}>Pause {t.title || 'Ticket'}</Button>
    {/if}
    {#if t.state === 'paused' && !['done', 'failed', 'abandoned'].includes(run.status)}
      <RunSwitchPicker
        label={t.title || `Ticket ${t.id}`}
        description="Applies to this Ticket's Worker and both Reviewer Chains."
        initialBackend={t.chain?.worker?.backend ?? run.harness}
        initialModel={t.chain?.worker?.model ?? run.model}
        onSwitch={(selection) => switchTicketItem(t, selection)}
        disabled={acting}
      />
    {/if}
  {/snippet}

  {#if ticketId && runId}
  {#key `${runId}/${ticketId}`}
    <TicketPage {runId} {ticketId} {projectId} {onNavigate} onOpenSession={onOpenSession} />
  {/key}
  {:else}
  <div class="min-h-0 flex-1 overflow-auto">
  <div class="mx-auto flex w-full max-w-[1600px] flex-col gap-4 px-4 py-6 md:px-6 {discovery.selected && !run?.tickets ? 'max-md:pb-28' : ''}">
  <PageHeader title="Runs" icon={icons.run} scope={scopeLabel} scopeIcon={icons.project} class="animate-rise" />

  {#if run?.id && run.tickets}
    <section class="animate-rise flex flex-col gap-4 rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="run-header">
      <div class="flex items-start justify-between gap-4">
        <div class="flex min-w-0 flex-col gap-1.5">
          <div class="flex flex-wrap items-center gap-2">
            <Chip tone={runTone(run.status)} dot>{run.status ?? ''}</Chip>
            {#if ownerName}<Chip tone="outline" icon={icons.session} title="Run owner">{ownerName}</Chip>{/if}
          </div>
          <h2 class="min-w-0 truncate text-title text-fg-1">{run.title || discovery.previewTitle || run.spec}</h2>
          <p class="truncate text-caption text-fg-3">{run.harness} · {String(run.model ?? '').split('/').pop()}{run.effort ? ` · ${run.effort}` : ''}</p>
          {#if run.specBranch}
            <span class="flex min-w-0 items-center gap-1.5 text-caption text-fg-3" data-testid="run-spec-branch">
              <icons.git class="size-3.5 shrink-0" aria-hidden="true" /><span class="truncate font-mono">{run.specBranch}</span> ·
              {#if run.status === 'abandoned'}
                abandoned
              {:else if Number.isInteger(run.specBehind)}
                {run.specBehind} {run.specBehind === 1 ? 'commit' : 'commits'} behind master
              {:else}
                distance unavailable from master
              {/if}
            </span>
          {/if}
        </div>
        <div class="shrink-0 text-right" aria-label={`${merged} of ${run.tickets.length} Tickets merged`}>
          <div class="tabular-nums"><span class="text-numeric text-fg-1">{merged}</span><span class="text-title text-fg-3">/{run.tickets.length}</span></div>
          <div class="text-caption text-fg-3" aria-hidden="true">merged</div>
        </div>
      </div>
      {#if run.tickets.length}
        <div class="h-1.5 overflow-hidden rounded-full bg-muted" role="progressbar" aria-label="Tickets merged" aria-valuemin="0" aria-valuemax={run.tickets.length} aria-valuenow={merged}>
          <div class="motion-emphasis h-full rounded-full bg-status-done" style="width: {(merged / run.tickets.length) * 100}%"></div>
        </div>
      {/if}
      <div class="flex flex-wrap items-center gap-2" data-testid="run-actions">
        {#if run.status === 'paused' || run.status === 'failed' || run.status === 'needs-you'}
          <Button disabled={acting} onclick={resume}>Resume</Button>
        {:else if run.status === 'running'}
          <Button variant="outline" disabled={acting} onclick={pause}>Pause</Button>
        {/if}
        {#if run.status === 'done' || run.status === 'failed' || run.status === 'abandoned'}
          <Button variant="outline" disabled={starting || !discovery.selected || (scopedProject && !workDir)} onclick={start}>Restart Run</Button>
        {/if}
        {#if run.specBranch && (run.status !== 'abandoned' || run.error?.startsWith('abandon cleanup incomplete'))}
          {#if run.status !== 'abandoned'}
            <Button variant="outline" disabled={acting} onclick={rebase}>Rebase</Button>
          {/if}
          {#if confirmAbandon}
            <Button variant="destructive" disabled={acting} onclick={abandon}>Confirm abandon</Button>
            <Button variant="ghost" disabled={acting} onclick={() => (confirmAbandon = false)}>Cancel</Button>
          {:else}
            <Button variant="ghost" disabled={acting} onclick={() => (confirmAbandon = true)}>Abandon spec</Button>
          {/if}
        {/if}
      </div>
    </section>
    {#if run.status === 'paused'}
      <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4 shadow-elev-1" data-testid="run-switch-controls">
        <h3 class="text-title text-fg-1">Switch paused Run</h3>
        <RunSwitchPicker label="Run" description="Applies to every unmerged Ticket's Worker and both Reviewer Chains." initialBackend={run.harness} initialModel={run.model} onSwitch={switchRunSelection} disabled={acting} />
      </section>
    {/if}
    {#if run.error}
      <p role="alert" class="flex items-start gap-2 rounded-xl border border-status-failed/30 bg-status-failed/10 px-4 py-3 text-body text-status-failed"><CircleAlert class="mt-0.5 size-4 shrink-0" aria-hidden="true" />{run.error}</p>
    {/if}
    {#if run.status === 'needs-you' && run.finalMergeFiles?.length}
      <p role="alert" class="flex items-start gap-2 rounded-xl border border-status-needs-you/30 bg-status-needs-you/10 px-4 py-3 text-body text-status-needs-you" data-testid="run-final-merge-files">
        <CircleAlert class="mt-0.5 size-4 shrink-0" aria-hidden="true" />Final merge needs attention in: {run.finalMergeFiles.join(', ')}
      </p>
    {/if}
    {#each (run.tickets ?? []).filter((ticket) => ticket.warnings?.length) as ticket (ticket.id)}
      <div class="flex items-center gap-2 rounded-xl border border-status-needs-you/30 bg-status-needs-you/10 py-1 pl-3 pr-1 text-caption" data-testid={`ticket-chain-warnings-${ticket.id}`}>
        <CircleAlert class="size-4 shrink-0 text-status-needs-you" aria-hidden="true" />
        <span class="min-w-0 flex-1 truncate text-fg-1">{ticket.title || `Ticket ${ticket.num}`}</span>
        <Chip tone="needs-you">Chain warning</Chip>
        <InfoHint label={`Chain warning for ${ticket.title || `Ticket ${ticket.num}`}`}>
          <ul class="flex list-inside list-disc flex-col gap-1">{#each ticket.warnings as warning}<li>{warning}</li>{/each}</ul>
        </InfoHint>
      </div>
    {/each}
  {:else if discovery.selected}
    <section class="flex flex-col gap-4" data-testid="preview">
      <div class="flex flex-wrap items-center gap-x-3 gap-y-2" data-testid="preview-head">
        <div class="flex min-w-0 flex-1 flex-col">
          <h2 class="truncate text-title text-fg-1">{discovery.previewTitle || discovery.selected}</h2>
          {#if discovery.tickets.length}
            <p class="text-caption tabular-nums text-fg-3">{discovery.tickets.length} {discovery.tickets.length === 1 ? 'ticket' : 'tickets'} · {discovery.tickets.filter((t) => previewType(t) === 'design').length} design</p>
          {/if}
        </div>
        <!-- Phone: the primary action pins to the bottom edge above the safe area. -->
        <div class="max-md:fixed max-md:inset-x-0 max-md:bottom-0 max-md:z-30 max-md:border-t max-md:bg-background/95 max-md:px-4 max-md:pt-3 max-md:pb-[max(0.75rem,env(safe-area-inset-bottom))] max-md:backdrop-blur" data-testid="start-bar">
          <Button class="max-md:w-full" disabled={starting || !discovery.tickets.length || (scopedProject && !workDir)} onclick={start}>
            {starting ? 'Starting…' : 'Start Run'}
          </Button>
        </div>
      </div>
      {#if discovery.tickets.length}
        <ul class="flex flex-col rounded-2xl border bg-card shadow-elev-1" data-testid="preview-table">
          {#each discovery.tickets as t (t.id)}
            {@const ptype = previewType(t)}
            <li class="flex flex-col gap-1.5 border-t px-3 py-2.5 first:border-t-0" data-testid={`preview-row-${t.id}`}>
              <div class="flex min-w-0 items-center gap-2">
                <span class="w-6 shrink-0 text-caption tabular-nums text-fg-3">{t.num}</span>
                <span class="min-w-0 flex-1 truncate text-body text-fg-1" title={t.id}>{t.title}</span>
                <Button size="sm" variant={ptype === 'design' ? 'secondary' : 'outline'} disabled={starting} title="Flip Ticket type before starting the Run" onclick={() => flipPreviewType(t)}>{ptype}</Button>
              </div>
              <div class="flex flex-wrap items-center gap-1.5 pl-8">
                {#if t.blockers?.length}<Chip tone="outline" title={`Blocked by ${t.blockers.join(', ')}`}>{blockersLabel(t.blockers)}</Chip>{/if}
                <Chip tone="outline" icon={ListChecks} title="Acceptance boxes ticked">{t.done ?? 0}/{t.total ?? 0}</Chip>
                {#if previewTypeOverrides[t.id]}
                  <span class="flex items-center gap-1 text-caption text-fg-3">
                    <span>overridden</span>
                    <Button size="sm" variant="ghost" class="px-2 text-caption" disabled={starting} title="Clear override and use the Ticket file type" onclick={() => clearPreviewType(t)}>Clear</Button>
                  </span>
                {/if}
                {#if ptype === 'design' && discovery.designWarnings.workerFallback}
                  <Chip tone="needs-you" icon={CircleAlert} data-testid={`design-worker-fallback-${t.id}`}>Worker fallback</Chip>
                {/if}
                {#if ptype === 'design' && discovery.designWarnings.reviewerFallback}
                  <Chip tone="needs-you" icon={CircleAlert} data-testid={`design-reviewer-fallback-${t.id}`}>Reviewer fallback</Chip>
                {/if}
                {#if ptype === 'design' && (discovery.designWarnings.workerFallback || discovery.designWarnings.reviewerFallback)}
                  <InfoHint label="About design fallback models">
                    <ul class="flex flex-col gap-1">
                      {#if discovery.designWarnings.workerFallback}<li>No designer model set; using the worker model.</li>{/if}
                      {#if discovery.designWarnings.reviewerFallback}<li>No design reviewer model set; using the standards reviewer model.</li>{/if}
                    </ul>
                  </InfoHint>
                {/if}
              </div>
              {#if t.error}
                <p role="alert" class="rounded-xl bg-status-failed/10 px-3 py-2 text-caption text-status-failed">{t.error}</p>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}
    </section>
  {:else}
    <EmptyState icon={icons.run} title="Pick a spec from the sidebar" description="Its tickets preview here.">
      {#snippet action()}
        {#if onOpenMenu}
          <Button variant="secondary" class="md:hidden" onclick={onOpenMenu}>Browse specs</Button>
        {/if}
      {/snippet}
    </EmptyState>
  {/if}
  {#if runError}
    <p role="alert" class="flex items-start gap-2 rounded-xl border border-status-failed/30 bg-status-failed/10 px-4 py-3 text-body text-status-failed"><CircleAlert class="mt-0.5 size-4 shrink-0" aria-hidden="true" />{runError}</p>
  {/if}
  {#if runLoading && run?.id && !run.tickets}
    <div role="status" aria-label="Loading Run details" class="flex flex-col gap-3" data-testid="run-loading">
      <Skeleton class="h-28 rounded-2xl" />
      <Skeleton class="h-14 rounded-2xl" />
      <Skeleton class="h-14 rounded-2xl" />
    </div>
  {/if}
  {#if run?.tickets}
      {#if runStreamDown}
        <p role="status" class="flex items-center gap-2 text-caption text-fg-3" data-testid="run-reconnecting">
          <span class="size-1.5 rounded-full bg-status-needs-you motion-safe:animate-pulse"></span>Reconnecting to live updates…
        </p>
      {/if}

      {#if needsYouTickets.length}
        <div class="animate-rise flex flex-col gap-0.5 rounded-2xl border border-status-needs-you/30 bg-status-needs-you/10 p-2" data-testid="run-needs-you">
          <span class="flex items-center gap-1.5 px-2 pt-1 text-caption font-medium text-status-needs-you">
            <CircleAlert class="size-3.5" aria-hidden="true" />Needs you ({needsYouTickets.length})
          </span>
          {#each needsYouTickets as t (t.id)}
            <button
              type="button"
              class="motion-standard min-h-9 truncate rounded-lg px-2 py-1 text-left text-body text-fg-1 outline-none hover:bg-status-needs-you/15 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11"
              onclick={() => run?.id && onNavigate?.(runPath(run.id, t.id))}
            >{t.num ? `#${t.num} ` : ''}{t.title || t.id}{t.needsYou ? ` — ${t.needsYou}` : ''}</button>
          {/each}
        </div>
      {/if}

      <div class="flex items-center gap-2">
        <h2 class="text-title text-fg-1">Tickets</h2>
        <span class="text-caption tabular-nums text-fg-3">{run.tickets.length}</span>
        <Segmented class="ml-auto w-44" label="Tickets layout" value={view} onchange={pickView} items={[{ value: 'graph', label: 'Graph', icon: Workflow }, { value: 'list', label: 'List', icon: List }]} />
      </div>
      {#if view === 'graph'}
        <RunsGraph tickets={run.tickets ?? []} selectedId={ticketId} onSelect={(t) => run?.id && onNavigate?.(runPath(run.id, t.id))} />
      {:else if run.tickets?.length && phone.value}
        <ul class="flex flex-col gap-2" data-testid="live-tickets-cards">
          {#each run.tickets as t (t.id)}
            {@const tone = stateTone(t.state)}
            {@const worker = t.chain?.worker}
            <li class="flex items-center gap-1 rounded-2xl border bg-card pr-1 shadow-elev-1">
              <Row
                class="min-w-0 flex-1"
                title={`${t.num ? `#${t.num} ` : ''}${t.title}`}
                status={tone === 'neutral' ? 'idle' : tone}
                attention={t.state === 'needs-you' || Boolean(t.needsYou)}
                meta={[t.state, boxCount(t) === '—' ? '' : boxCount(t), worker ? `${worker.backend}/${worker.model}` : ''].filter(Boolean).join(' · ')}
                onclick={() => run?.id && onNavigate?.(runPath(run.id, t.id))}
              >
                {#snippet badges()}
                  {#if ticketTypeOf(t) === 'design'}<Chip tone="outline">design</Chip>{/if}
                  {#if t.warnings?.length}<CircleAlert class="size-4 shrink-0 text-status-needs-you" role="img" aria-label="Chain warning" />{/if}
                {/snippet}
              </Row>
              <IconButton icon={SlidersHorizontal} label={`Details for ${t.title}`} onclick={() => { sheetTicketId = t.id; sheetOpen = true; }} />
            </li>
          {/each}
        </ul>
      {:else if run.tickets?.length}
        <div class="overflow-x-auto rounded-2xl border bg-card shadow-elev-1" data-testid="live-tickets-wrap">
        <table class="w-full min-w-[1120px] text-left text-body" data-testid="live-tickets">
          <thead>
            <tr class="border-b bg-sidebar/60 text-caption text-fg-3">
              <th class="px-3 py-2 font-medium">Ticket</th>
              <th class="px-3 py-2 font-medium">State</th>
              <th class="px-3 py-2 font-medium">Agents</th>
              <th class="px-3 py-2 font-medium">Active Chain</th>
              <th class="px-3 py-2 font-medium">Spec review</th>
              <th class="px-3 py-2 font-medium">Standards</th>
              <th class="px-3 py-2 font-medium">Type</th>
              <th class="px-3 py-2 font-medium">Boxes</th>
              <th class="px-3 py-2 font-medium">Tip</th>
              <th class="px-3 py-2 font-medium">Refusal</th>
              <th class="px-3 py-2 font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {#each run.tickets as t (t.id)}
              <tr
                class="motion-standard cursor-pointer border-t hover:bg-surface-3/60 {ticketId && t.id === ticketId ? 'bg-surface-3' : ''}"
                data-current={ticketId && t.id === ticketId ? '' : undefined}
                onclick={(event) => {
                  if (event.target?.closest?.('button,[role="combobox"]')) return;
                  if (run?.id) onNavigate?.(runPath(run.id, t.id));
                }}
              >
                <td class="min-w-[14rem] max-w-[22rem] px-3 py-2 [overflow-wrap:anywhere]">
                  <div class="flex min-w-0 items-baseline gap-2">
                    <span class="shrink-0 tabular-nums text-fg-3">#{t.num}</span>
                    {#if onNavigate && run?.id}
                      <Button variant="link" size="sm" class="h-auto min-w-0 shrink justify-start whitespace-normal p-0 text-left text-body font-medium" onclick={(e) => { e.stopPropagation(); onNavigate(runPath(run.id, t.id)); }}>{t.title}</Button>
                    {:else}
                      <span class="font-medium">{t.title}</span>
                    {/if}
                  </div>
                  <div class="truncate pl-6 text-caption text-fg-3" title={t.id}>{t.id}</div>
                </td>
                <td class="px-3 py-2"><Chip tone={stateTone(t.state)} dot>{t.state}</Chip></td>
                <td class="px-3 py-2">
                  <div class="flex flex-wrap gap-1">
                    {#if t.workerSession}
                      <button type="button" class="motion-standard rounded-full border px-2.5 py-0.5 text-caption text-fg-2 outline-none hover:bg-surface-3 focus-visible:ring-[3px] focus-visible:ring-ring/40" title="Open the worker Session" onclick={(e) => { e.stopPropagation(); onOpenSession?.(t.workerSession); }}>worker</button>
                    {/if}
                    {#each Object.entries(t.reviewerSessions ?? {}) as [type, sid] (sid)}
                      <button type="button" class="motion-standard rounded-full border px-2.5 py-0.5 text-caption text-fg-2 outline-none hover:bg-surface-3 focus-visible:ring-[3px] focus-visible:ring-ring/40" title="Open the {reviewerLabel(type)} reviewer Session" onclick={(e) => { e.stopPropagation(); onOpenSession?.(sid); }}>{reviewerLabel(type)}</button>
                    {/each}
                    {#if !t.workerSession}<span class="text-fg-3">—</span>{/if}
                  </div>
                </td>
                <td class="px-3 py-2" data-testid={`ticket-chain-${t.id}`}>
                  <div class="flex min-w-52 flex-col gap-0.5 text-caption">
                    {#each activeChainEntries(t) as entry (entry.role)}
                      <span title={`${entry.label}: ${entry.backend}/${entry.model}`}>
                        {entry.label} {entry.position}/{entry.total} · {entry.backend}/{entry.model}
                      </span>
                    {/each}
                    {#if !activeChainEntries(t).length}<span class="text-fg-3">—</span>{/if}
                  </div>
                </td>
                <td class="px-3 py-2">{@render verdict(t.verdicts?.['reviewer-spec'])}</td>
                <td class="px-3 py-2">{@render verdict(t.verdicts?.['reviewer-standards'])}</td>
                <td class="px-3 py-2">
                  <div class="flex flex-col items-start gap-1">{@render typeControl(t)}</div>
                </td>
                <td class="px-3 py-2 tabular-nums">{boxCount(t)}</td>
                <td class="px-3 py-2 font-mono text-caption text-fg-3">{t.tip ? t.tip.slice(0, 8) : '—'}</td>
                <td class="max-w-[16rem] truncate px-3 py-2 text-caption text-fg-3" title={t.lastRefusal || t.needsYou || ''}>{t.lastRefusal || t.needsYou || '—'}</td>
                <td class="px-3 py-2" data-testid={`ticket-actions-${t.id}`}>
                  <div class="flex min-w-[18rem] flex-col gap-2">{@render ticketControls(t)}</div>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        </div>
      {/if}
    {/if}
  </div>
  </div>

  <!-- Phone: one Ticket's Chain, verdicts and controls in a bottom sheet. -->
  <BottomSheet bind:open={sheetOpen} title={sheetTicket ? `${sheetTicket.num ? `#${sheetTicket.num} ` : ''}${sheetTicket.title}` : 'Ticket'}>
    {#if sheetTicket && run}
      {@const t = sheetTicket}
      <div class="flex flex-wrap items-center gap-2" data-testid="ticket-sheet">
        <Chip tone={stateTone(t.state)} dot>{t.state}</Chip>
        {@render typeControl(t)}
        <Chip tone="outline" icon={ListChecks} title="Acceptance boxes ticked">{boxCount(t)}</Chip>
      </div>
      <dl class="grid grid-cols-[auto_1fr] items-center gap-x-4 gap-y-2 text-body">
        <dt class="text-caption text-fg-3">Chain</dt>
        <dd class="flex flex-col gap-0.5 text-caption">
          {#each activeChainEntries(t) as entry (entry.role)}<span>{entry.label} {entry.position}/{entry.total} · {entry.backend}/{entry.model}</span>{/each}
          {#if !activeChainEntries(t).length}<span class="text-fg-3">—</span>{/if}
        </dd>
        <dt class="text-caption text-fg-3">Spec review</dt>
        <dd>{@render verdict(t.verdicts?.['reviewer-spec'])}</dd>
        <dt class="text-caption text-fg-3">Standards</dt>
        <dd>{@render verdict(t.verdicts?.['reviewer-standards'])}</dd>
        <dt class="text-caption text-fg-3">Tip</dt>
        <dd class="font-mono text-caption text-fg-2">{t.tip ? t.tip.slice(0, 8) : '—'}</dd>
        {#if t.lastRefusal || t.needsYou}
          <dt class="text-caption text-fg-3">Refusal</dt>
          <dd class="text-caption text-fg-2">{t.lastRefusal || t.needsYou}</dd>
        {/if}
      </dl>
      {#if t.workerSession || Object.keys(t.reviewerSessions ?? {}).length}
        <div class="flex flex-wrap gap-2">
          {#if t.workerSession}<Button size="sm" variant="outline" onclick={() => onOpenSession?.(t.workerSession)}>Worker</Button>{/if}
          {#each Object.entries(t.reviewerSessions ?? {}) as [type, sid] (sid)}
            <Button size="sm" variant="outline" onclick={() => onOpenSession?.(sid)}>{reviewerLabel(type)}</Button>
          {/each}
        </div>
      {/if}
      <div class="flex flex-col gap-2">{@render ticketControls(t)}</div>
      <Button variant="secondary" onclick={() => { sheetOpen = false; run?.id && onNavigate?.(runPath(run.id, t.id)); }}>Open Ticket</Button>
    {/if}
  </BottomSheet>

  <!-- Phone: the graph opens full screen on request and owns every gesture there. -->
  <Dialog.Root bind:open={graphFull}>
    <Dialog.Portal>
      <Dialog.Content class="fixed inset-0 z-50 flex flex-col bg-background pt-[env(safe-area-inset-top)] outline-none" data-testid="graph-fullscreen">
        <div class="flex min-h-14 shrink-0 items-center gap-2 px-4">
          <Dialog.Title class="min-w-0 flex-1 truncate text-title text-fg-1">Dependency graph</Dialog.Title>
          <Dialog.Close>
            {#snippet child({ props })}
              <IconButton {...props} icon={icons.dismiss} label="Close graph" class="-mr-2" />
            {/snippet}
          </Dialog.Close>
        </div>
        <Dialog.Description class="sr-only">Drag to pan, pinch or use the buttons to zoom.</Dialog.Description>
        {#if run?.tickets}
          <RunsGraph fullscreen tickets={run.tickets} selectedId={ticketId} onSelect={(t) => { graphFull = false; run?.id && onNavigate?.(runPath(run.id, t.id)); }} />
        {/if}
      </Dialog.Content>
    </Dialog.Portal>
  </Dialog.Root>
  {/if}
