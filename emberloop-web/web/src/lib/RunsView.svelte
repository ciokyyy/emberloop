<!-- Runs view: the main area owns the picked spec's ticket preview, the Run
     start button and the live Tickets as a dependency graph or table.
     Spec discovery (repo directory, spec list, Find specs form, preview
     pick) lives in the rail-2 RunsSidebar and shares this view's
     `runsDiscovery` store, so a sidebar pick repaints this area. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import RunsGraph from './RunsGraph.svelte';
  import { startRun, getRun, openRunStream, pauseRun, resumeRun, pauseTicket as pauseRunTicket, resumeTicket as resumeRunTicket, switchRun as switchRunBackend, switchTicket as switchTicketBackend, rebaseRun, abandonRun, setRunTicketType } from './api.js';
  import RunSwitchPicker from './RunSwitchPicker.svelte';
  import TicketPage from './TicketPage.svelte';
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
 	const runBadge = (status) => {
 		if (status === 'done') return 'success';
 		if (status === 'failed') return 'destructive';
 		if (status === 'paused' || status === 'needs-you') return 'warning';
 		if (status === 'abandoned') return 'outline';
 		return 'secondary';
 	};
  const stateBadge = (state) =>
    state === 'merged' ? 'success' : state === 'needs-you' ? 'warning' : state === 'discarded' || state === 'parked' ? 'outline' : state === 'queued' ? 'outline' : 'secondary';
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

  let { workDir = '', onOpenSession = null, runId = '', ticketId = '', projectId = null, onNavigate = null, onOpenMenu = null } = $props();

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
  // List/Graph toggle for the live tickets; the graph is the default view.
  // Phones open the ticket table: a wide dependency graph cannot fit there
  // and its pan surface traps page scroll. The Graph toggle stays one tap away.
  let graphView = $state(typeof window !== 'undefined' && window.matchMedia?.('(max-width: 767px)')?.matches ? 'list' : 'graph');
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
      <Badge variant={v.verdict === 'pass' ? 'success' : 'warning'}>{v.verdict}{v.fresh ? '' : ' · stale'}</Badge>
    {:else}
      <span class="text-muted-foreground">—</span>
    {/if}
  {/snippet}

  {#if ticketId && runId}
  {#key `${runId}/${ticketId}`}
    <TicketPage {runId} {ticketId} {projectId} {onNavigate} onOpenSession={onOpenSession} />
  {/key}
  {:else}
  <div class="min-h-0 flex-1 overflow-auto">
  <div class="mx-auto flex w-full max-w-[1600px] flex-col gap-4 px-4 py-6 sm:px-6">
  <h2 class="text-lg font-semibold tracking-tight">Runs</h2>

  {#if run?.id && run.tickets}
    <div class="flex flex-wrap items-center gap-x-3 gap-y-2" data-testid="run-header">
      <h3 class="text-sm font-semibold tracking-tight">{run.title || discovery.previewTitle || run.spec}</h3>
      <Badge variant={runBadge(run.status)}>{run.status ?? ''}</Badge>
      <span class="text-xs text-muted-foreground">
        {merged}/{run.tickets.length} merged · {run.harness} · {String(run.model ?? '').split('/').pop()}{run.effort ? ` · ${run.effort}` : ''}
      </span>
      {#if run.specBranch}
        <span class="text-xs text-muted-foreground" data-testid="run-spec-branch">
          <span class="font-mono">{run.specBranch}</span> ·
          {#if run.status === 'abandoned'}
            abandoned
          {:else if Number.isInteger(run.specBehind)}
            {run.specBehind} {run.specBehind === 1 ? 'commit' : 'commits'} behind master
          {:else}
            distance unavailable from master
          {/if}
        </span>
      {/if}
      <div class="ml-auto flex flex-wrap items-center gap-2">
        {#if run.status === 'done' || run.status === 'failed' || run.status === 'abandoned'}
          <Button size="sm" variant="outline" disabled={starting || !discovery.selected || (scopedProject && !workDir)} onclick={start}>Restart Run</Button>
        {/if}
        {#if run.specBranch && (run.status !== 'abandoned' || run.error?.startsWith('abandon cleanup incomplete'))}
          {#if run.status !== 'abandoned'}
            <Button size="sm" variant="outline" disabled={acting} onclick={rebase}>Rebase</Button>
          {/if}
          {#if confirmAbandon}
            <Button size="sm" variant="destructive" disabled={acting} onclick={abandon}>Confirm abandon</Button>
            <Button size="sm" variant="ghost" disabled={acting} onclick={() => (confirmAbandon = false)}>Cancel</Button>
          {:else}
            <Button size="sm" variant="ghost" disabled={acting} onclick={() => (confirmAbandon = true)}>Abandon spec</Button>
          {/if}
        {/if}
      </div>
    </div>
    {#if run.status === 'paused'}
      <section class="animate-rise flex flex-col gap-2 rounded-2xl border bg-card p-3" data-testid="run-switch-controls">
        <div>
          <h4 class="text-sm font-medium">Switch paused Run</h4>
          <p class="text-xs text-muted-foreground">Choose a Backend and model for every unmerged Ticket, then resume when ready.</p>
        </div>
        <RunSwitchPicker label="Run" description="Applies to every unmerged Ticket's Worker and both Reviewer Chains." initialBackend={run.harness} initialModel={run.model} onSwitch={switchRunSelection} disabled={acting} />
      </section>
    {/if}
    {#if run.error}
      <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{run.error}</p>
    {/if}
    {#if run.status === 'needs-you' && run.finalMergeFiles?.length}
      <p role="alert" class="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-2 text-sm text-amber-200" data-testid="run-final-merge-files">
        Final merge needs attention in: {run.finalMergeFiles.join(', ')}
      </p>
    {/if}
  {:else if discovery.selected}
    <div class="flex items-center gap-2">
      <h3 class="text-sm font-semibold tracking-tight">{discovery.previewTitle || discovery.selected}</h3>
      <Button size="sm" class="ml-auto" disabled={starting || !discovery.tickets.length || (scopedProject && !workDir)} onclick={start}>
        {starting ? 'Starting…' : 'Start Run'}
      </Button>
    </div>
    {#if discovery.tickets.length}
      <div class="overflow-x-auto rounded-2xl border px-3" data-testid="preview-wrap">
      <table class="w-full min-w-[30rem] text-left text-sm" data-testid="preview-table">
        <thead>
          <tr class="text-muted-foreground">
            <th class="py-1 pr-2 font-medium">No.</th>
            <th class="py-1 pr-2 font-medium">ID</th>
            <th class="py-1 pr-2 font-medium">Title</th>
            <th class="py-1 pr-2 font-medium">Blocked by</th>
            <th class="py-1 pr-2 font-medium">Type</th>
            <th class="py-1 font-medium">Boxes</th>
          </tr>
        </thead>
        <tbody>
          {#each discovery.tickets as t (t.id)}
            <tr class="border-t">
              <td class="py-1 pr-2">{t.num}</td>
              <td class="py-1 pr-2">{t.id}</td>
              <td class="py-1 pr-2">{t.title}</td>
              <td class="py-1 pr-2">{(t.blockers ?? []).join(', ') || '—'}</td>
              <td class="py-1 pr-2">
                <Button size="sm" variant={previewType(t) === 'design' ? 'secondary' : 'outline'} disabled={starting} title="Flip Ticket type before starting the Run" onclick={() => flipPreviewType(t)}>{previewType(t)}</Button>
                {#if previewTypeOverrides[t.id]}
                  <div class="flex items-center gap-1 text-xs text-muted-foreground">
                    <span>overridden</span>
                    <Button size="sm" variant="link" class="h-auto min-h-0 p-0 text-xs" disabled={starting} title="Clear override and use the Ticket file type" onclick={() => clearPreviewType(t)}>Clear</Button>
                  </div>
                {/if}
              </td>
              <td class="py-1">{`${t.done ?? 0}/${t.total ?? 0}`}</td>
            </tr>
            {#if t.error}
              <tr>
                <td colspan="5" class="pb-2">
                  <p role="alert" class="rounded-xl bg-destructive/10 px-3 py-1 text-sm text-destructive">{t.error}</p>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
      </div>
    {/if}
  {:else}
    <p class="rounded-xl border px-4 py-2 text-sm text-muted-foreground">Pick a spec from the sidebar to preview its tickets.</p>
    {#if onOpenMenu}
      <button type="button" class="mt-3 rounded-full bg-secondary px-4 py-2 text-sm md:hidden" onclick={onOpenMenu}>Browse specs</button>
    {/if}
  {/if}
  {#if runError}
    <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{runError}</p>
  {/if}
  {#if runLoading && run?.id && !run.tickets}
    <p role="status" class="rounded-xl border px-4 py-2 text-sm text-muted-foreground" data-testid="run-loading">Loading Run details…</p>
  {/if}
  {#if run?.tickets}
      {#if runStreamDown}
        <p role="status" class="flex items-center gap-2 rounded-xl border px-4 py-2 text-sm text-muted-foreground" data-testid="run-reconnecting">
          <span class="size-1.5 animate-pulse rounded-full bg-amber-400"></span>Reconnecting to live updates…
        </p>
      {/if}

      {#if needsYouTickets.length}
        <div class="animate-rise flex flex-col gap-1 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-2" data-testid="run-needs-you">
          <span class="px-2 pt-0.5 text-[11px] font-medium uppercase tracking-wide text-amber-300">
            Needs you ({needsYouTickets.length})
          </span>
          {#each needsYouTickets as t (t.id)}
            <button
              type="button"
              class="truncate rounded-lg px-2 py-1 text-left text-sm transition-colors hover:bg-amber-500/15"
              onclick={() => run?.id && onNavigate?.(runPath(run.id, t.id))}
            >{t.num ? `#${t.num} ` : ''}{t.title || t.id}{t.needsYou ? ` — ${t.needsYou}` : ''}</button>
          {/each}
        </div>
      {/if}

      <div class="flex items-center gap-2">
        <h4 class="text-sm font-semibold tracking-tight">Live Tickets</h4>
        {#if !run.title && !run.tickets}<Badge>{run.status ?? ''}</Badge>{/if}
        <div class="ml-auto flex items-center gap-2">
          <div class="flex rounded-full bg-muted p-0.5 text-xs" role="group" aria-label="Tickets layout">
            <Button size="sm" variant={graphView === 'graph' ? 'secondary' : 'ghost'} class="rounded-full" aria-pressed={graphView === 'graph'} onclick={() => (graphView = 'graph')}>Graph</Button>
            <Button size="sm" variant={graphView === 'list' ? 'secondary' : 'ghost'} class="rounded-full" aria-pressed={graphView === 'list'} onclick={() => (graphView = 'list')}>List</Button>
          </div>
          {#if run.status === 'paused' || run.status === 'failed' || run.status === 'needs-you'}
            <Button size="sm" variant="outline" disabled={acting} onclick={resume}>Resume</Button>
          {:else if run.status === 'running'}
            <Button size="sm" variant="outline" disabled={acting} onclick={pause}>Pause</Button>
          {/if}
        </div>
      </div>
      {#if graphView === 'graph'}
        <RunsGraph tickets={run.tickets ?? []} selectedId={ticketId} onSelect={(t) => run?.id && onNavigate?.(runPath(run.id, t.id))} />
      {:else if run.tickets?.length}
        <div class="overflow-x-auto rounded-2xl border" data-testid="live-tickets-wrap">
        <table class="w-full min-w-[1120px] text-left text-sm" data-testid="live-tickets">
          <thead>
            <tr class="border-b bg-sidebar/60 text-xs text-muted-foreground">
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
                class="cursor-pointer border-t transition-colors hover:bg-accent/60 {ticketId && t.id === ticketId ? 'bg-accent' : ''}"
                data-current={ticketId && t.id === ticketId ? '' : undefined}
                onclick={(event) => {
                  if (event.target?.closest?.('button,[role="combobox"]')) return;
                  if (run?.id) onNavigate?.(runPath(run.id, t.id));
                }}
              >
                <td class="min-w-[14rem] max-w-[22rem] px-3 py-2 [overflow-wrap:anywhere]">
                  <div class="flex min-w-0 items-baseline gap-2">
                    <span class="shrink-0 tabular-nums text-muted-foreground">#{t.num}</span>
                    {#if onNavigate && run?.id}
                      <Button variant="link" size="sm" class="h-auto min-w-0 shrink justify-start whitespace-normal p-0 text-left font-medium max-md:min-h-11 max-md:py-2" onclick={(e) => { e.stopPropagation(); onNavigate(runPath(run.id, t.id)); }}>{t.title}</Button>
                    {:else}
                      <span class="font-medium">{t.title}</span>
                    {/if}
                  </div>
                  <div class="truncate pl-6 text-xs text-muted-foreground" title={t.id}>{t.id}</div>
                </td>
                <td class="px-3 py-2"><Badge variant={stateBadge(t.state)}>{t.state}</Badge></td>
                <td class="px-3 py-2">
                  <div class="flex flex-wrap gap-1">
                    {#if t.workerSession}
                      <button type="button" class="rounded-full border px-2 py-0.5 text-xs transition-colors hover:bg-accent" title="Open the worker Session" onclick={(e) => { e.stopPropagation(); onOpenSession?.(t.workerSession); }}>worker</button>
                    {/if}
                    {#each Object.entries(t.reviewerSessions ?? {}) as [type, sid] (sid)}
                      <button type="button" class="rounded-full border px-2 py-0.5 text-xs transition-colors hover:bg-accent" title="Open the {reviewerLabel(type)} reviewer Session" onclick={(e) => { e.stopPropagation(); onOpenSession?.(sid); }}>{reviewerLabel(type)}</button>
                    {/each}
                    {#if !t.workerSession}<span class="text-muted-foreground">—</span>{/if}
                  </div>
                </td>
                <td class="px-3 py-2" data-testid={`ticket-chain-${t.id}`}>
                  <div class="flex min-w-52 flex-col gap-0.5 text-xs">
                    {#each activeChainEntries(t) as entry (entry.role)}
                      <span title={`${entry.label}: ${entry.backend}/${entry.model}`}>
                        {entry.label} {entry.position}/{entry.total} · {entry.backend}/{entry.model}
                      </span>
                    {/each}
                    {#if !activeChainEntries(t).length}<span class="text-muted-foreground">—</span>{/if}
                  </div>
                </td>
                <td class="px-3 py-2">{@render verdict(t.verdicts?.['reviewer-spec'])}</td>
                <td class="px-3 py-2">{@render verdict(t.verdicts?.['reviewer-standards'])}</td>
                <td class="px-3 py-2">
                  {#if !t.workerSession}
                    <Button size="sm" variant={(t.effectiveType || t.type || 'code') === 'design' ? 'secondary' : 'outline'} disabled={acting} title="Flip Ticket type before the Worker starts" onclick={(event) => { event.stopPropagation(); changeTicketType(t, (t.effectiveType || t.type || 'code') === 'design' ? 'code' : 'design'); }}>{t.effectiveType || t.type || 'code'}</Button>
                  {:else}
                    <Badge variant={(t.effectiveType || t.type || 'code') === 'design' ? 'secondary' : 'outline'} title="Ticket type is read-only after the Worker starts">{t.effectiveType || t.type || 'code'}</Badge>
                  {/if}
                  {#if t.typeOverride}
                    <div class="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
                      <span>overridden</span>
                      {#if !t.workerSession}
                        <Button size="sm" variant="link" class="h-auto min-h-0 p-0 text-xs" disabled={acting} title="Clear override and use the Ticket file type" onclick={(event) => { event.stopPropagation(); changeTicketType(t, ''); }}>Clear</Button>
                      {/if}
                    </div>
                  {/if}
                </td>
                <td class="px-3 py-2 tabular-nums">{boxCount(t)}</td>
                <td class="px-3 py-2 font-mono text-xs text-muted-foreground">{t.tip ? t.tip.slice(0, 8) : '—'}</td>
                <td class="max-w-[16rem] truncate px-3 py-2 text-xs text-muted-foreground" title={t.lastRefusal || t.needsYou || ''}>{t.lastRefusal || t.needsYou || '—'}</td>
                <td class="px-3 py-2" data-testid={`ticket-actions-${t.id}`}>
                  <div class="flex min-w-[18rem] flex-col gap-2">
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
                  </div>
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
  {/if}
