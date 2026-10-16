<!-- Memory page: the User's own bank. Rules and observations list newest
     first and search through FTS5; each Rule offers Promote to file it as a
     promotion against its own worktree; the queue shows each proposal with its
     quotes and source Sessions with approve, edit, reject and batch approve;
     the promotions queue below approves proposed diffs; the Profile editor
     (versions plus restore) lives below; purge needs the typed PURGE
     confirmation and reports what it removed; the header chip shows engine
     status and stats show queue size, approval latency, the last nightly run
     and the ledger. Loads once on mount and never polls; only the buttons
     fetch again. -->
<script>
  import { ArrowLeft, RefreshCw } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import Input from './ui/input.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import ProfileSettings from './ProfileSettings.svelte';
  import PromotionQueue from './PromotionQueue.svelte';
  import {
    getMemories, getEpisodes, forgetMemory, parkMemory, pinMemory,
    promoteMemoryRule, editMemory, promoteObservationMemory,
    getRules, getObservations, getRulesQueue, decideRuleQueue, batchRuleQueue,
    getMemoryLedger, getMemoryStatus, purgeMemory, proposePromotion,
  } from './api.js';
  import {
    normalizeRule,
    normalizeObservation,
    normalizeQueueItem,
    normalizeLedger,
    parseProposal,
    groupQueueBySession,
    purgeCounts,
    purgeSummary,
    latencyLabel,
  } from './memory.js';

  let { onBack = () => {}, userId = 'default' } = $props();

  let status = $state(null);
  let statusError = $state('');
  let rules = $state([]);
  let observations = $state([]);
  let queue = $state([]);
  let memories = $state([]);
  let episodes = $state([]);
  let tab = $state('memories');
  let memoryQuery = $state('');
  let memoryKind = $state('');
  let memoryStatus = $state('');
  let editingMemory = $state(0);
  let editMemoryText = $state('');
  let newSinceVisit = $state(0);
  let counterInitialized = false;
  let ledger = $state([]);
  let loading = $state(true);
  let error = $state('');
  let notice = $state('');
  let ruleQuery = $state('');
  let obsQuery = $state('');
  let searching = $state(false);

  // Purge selection: IDs the human checked on the page.
  let selRules = $state([]);
  let selObs = $state([]);
  let selMemories = $state([]);
  let selEpisodes = $state([]);
  let selQueue = $state([]);
  let selLedger = $state([]);
  let confirm = $state('');
  let purging = $state(false);
  let purgeResult = $state('');
  // Queue edit state: one item at a time.
  let editingId = $state(0);
  let editText = $state('');

  // Promotion propose state: one Rule at a time. The queue section below is
  // the PromotionQueue component; proposing refreshes it in place.
  let promoQueue = $state(null);
  let promoteBusy = $state('');

  function toggle(list, id) {
    const i = list.indexOf(id);
    if (i === -1) return [...list, id];
    return list.filter((x) => x !== id);
  }

  async function load() {
    loading = true;
    error = '';
    statusError = '';
    try {
      const [st, r, o, q, l, m, ep] = await Promise.all([
        getMemoryStatus(), getRules(), getObservations(), getRulesQueue(), getMemoryLedger(), getMemories(), getEpisodes(),
      ]);
      status = st;
      rules = Array.isArray(r) ? r.map(normalizeRule) : [];
      observations = Array.isArray(o) ? o.map(normalizeObservation) : [];
      queue = Array.isArray(q) ? q.map(normalizeQueueItem) : [];
      ledger = Array.isArray(l) ? l.map(normalizeLedger) : [];
      memories = Array.isArray(m) ? m : [];
      if (!counterInitialized && typeof localStorage !== 'undefined') {
		const key = `emberloop.memory.lastVisit.${userId}`;
        const previous = Number(localStorage.getItem(key) || 0);
        newSinceVisit = memories.filter((item) => Date.parse(item.createdAt) > previous).length;
        localStorage.setItem(key, String(Date.now()));
        counterInitialized = true;
      }
    } catch (e) {
      if (!status && !rules.length && !queue.length) error = e.message || 'Could not load memory.';
      else statusError = e.message || 'Refresh failed.';
    } finally {
      loading = false;
    }
  }
  load();


  async function filterMemories() {
    try { memories = await getMemories({ q: memoryQuery, kind: memoryKind, status: memoryStatus }); }
    catch (e) { error = e.message || 'Memory search failed.'; }
  }
  async function mutateMemory(action, m) {
    await run(async () => {
      if (action === 'forget') await forgetMemory(m.id);
      if (action === 'park') await parkMemory(m.id);
      if (action === 'pin') await pinMemory(m.id, !m.pinned);
      if (action === 'promote') { await promoteMemoryRule(m.id); notice = 'Pending Rule proposal created; it still needs approval.'; }
      if (action === 'edit') { await editMemory(m.id, editMemoryText.trim()); editingMemory = 0; editMemoryText = ''; }
    });
  }
  async function run(fn) {
    error = '';
    notice = '';
    try {
      await fn();
      await load();
    } catch (e) {
      error = e.message || 'Request failed.';
    }
  }

  async function searchRules() {
    searching = true;
    error = '';
    try {
      const r = await getRules(ruleQuery);
      rules = Array.isArray(r) ? r.map(normalizeRule) : [];
    } catch (e) {
      error = e.message || 'Search failed.';
    } finally {
      searching = false;
    }
  }

  async function searchObs() {
    searching = true;
    error = '';
    try {
      const o = await getObservations(obsQuery);
      observations = Array.isArray(o) ? o.map(normalizeObservation) : [];
    } catch (e) {
      error = e.message || 'Search failed.';
    } finally {
      searching = false;
    }
  }

  // Promote one Rule from the list: file it as a promotion against its own
  // worktree (the Daemon resolves the Session from the Rule's provenance),
  // then refresh the promotions queue below so its diff is ready to approve.
  async function promoteRule(r) {
    promoteBusy = r.ruleId;
    error = '';
    notice = '';
    try {
      const item = await proposePromotion(r.ruleId, '', '');
      notice = `Promotion proposed for ${item.target_path ?? 'project knowledge'}; approve its diff below.`;
      await promoQueue?.refresh();
    } catch (e) {
      error = e.message || 'Request failed.';
    } finally {
      promoteBusy = '';
    }
  }

  async function approveItem(item, text = null) {
    await run(async () => {
      await decideRuleQueue(item.id, 'approved', text ? { text } : null);
      if (editingId) {
        editingId = 0;
        editText = '';
      }
      notice = 'Rule approved.';
    });
  }

  async function rejectItem(item) {
    await run(async () => {
      await decideRuleQueue(item.id, 'rejected');
      notice = 'Proposal rejected. It stays as a label.';
    });
  }

  async function batchSession(sessionId, rows) {
    await run(async () => {
      if (sessionId) await batchRuleQueue({ sessionId, status: 'approved' });
      else await batchRuleQueue({ ids: rows.map((r) => r.item.id), status: 'approved' });
      notice = `Approved ${rows.length} proposal${rows.length === 1 ? '' : 's'}.`;
    });
  }

  async function batchSelected() {
    const ids = queue.filter((q) => q.status === 'pending' && q.kind === 'rule' && selQueue.includes(q.id)).map((q) => q.id);
    if (!ids.length) {
      error = 'Select queue items first.';
      return;
    }
    await run(async () => {
      await batchRuleQueue({ ids, status: 'approved' });
      selQueue = [];
      notice = `Approved ${ids.length} proposal${ids.length === 1 ? '' : 's'}.`;
    });
  }

  function startEdit(item, proposal) {
    editingId = item.id;
    editText = proposal.text;
  }

  const queueGroups = $derived(groupQueueBySession(queue));
  const pendingCount = $derived(queue.filter((q) => q.status === 'pending').length);
  const selection = $derived({ observations: selObs, memories: selMemories, episodes: selEpisodes, rules: selRules, queue: selQueue, ledger: selLedger });
  const counts = $derived(purgeCounts(selection));
  const canPurge = $derived(counts.total > 0 && confirm.trim() === 'PURGE' && !purging);
  const available = $derived(status?.available === true);
  const lastNightly = $derived(status?.lastNightly ?? null);
  const lastLedger = $derived(status?.lastLedger ?? null);

  async function runPurge() {
    if (!canPurge) return;
    purging = true;
    error = '';
    notice = '';
    purgeResult = '';
    try {
      const res = await purgeMemory(selection, confirm.trim());
      purgeResult = purgeSummary(res);
      selRules = [];
      selObs = [];
      selMemories = [];
      selEpisodes = [];
      selQueue = [];
      selLedger = [];
      confirm = '';
      await load();
    } catch (e) {
      error = e.message || 'Purge failed.';
    } finally {
      purging = false;
    }
  }
</script>

<div class="flex min-h-0 flex-1 flex-col" data-testid="memory-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-16">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div class="flex items-end gap-3">
        <div class="flex flex-col gap-1">
          <Button variant="ghost" size="sm" class="-ml-3 w-fit text-muted-foreground" onclick={onBack}><ArrowLeft />Back</Button>
          <h1 class="text-3xl font-semibold tracking-tight">Memory</h1>
          <p class="text-sm text-muted-foreground">Your standing instructions, approved notes and pending proposals. Only your bank shows here.</p>
        </div>
        <div class="ml-auto flex shrink-0 items-center gap-2">
          {#if status}
            {#if available}
              <Badge data-testid="memory-status">Available</Badge>
            {:else}
              <Badge variant="destructive" data-testid="memory-status">Unavailable</Badge>
            {/if}
          {:else if loading}
            <Badge data-testid="memory-status">Loading…</Badge>
          {:else}
            <Badge variant="destructive" data-testid="memory-status">Unavailable</Badge>
          {/if}
          <Button variant="outline" size="sm" onclick={load} aria-label="Refresh memory"><RefreshCw /></Button>
        </div>
      </div>
        <nav class="flex flex-wrap gap-2" aria-label="Memory tabs">
          <Button size="sm" variant={tab === 'memories' ? 'default' : 'outline'} onclick={() => (tab = 'memories')}>Memories · {newSinceVisit} new</Button>
          <Button size="sm" variant={tab === 'legacy' ? 'default' : 'outline'} onclick={() => (tab = 'legacy')}>Legacy observations</Button>
          <Button size="sm" variant={tab === 'episodes' ? 'default' : 'outline'} onclick={() => (tab = 'episodes')}>Episodes</Button>
        </nav>
        {#if tab === 'memories'}
          <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4" data-testid="memory-items">
            <h2 class="text-sm font-semibold">Memories ({memories.length})</h2>
            <form class="flex flex-wrap gap-2" onsubmit={(e) => { e.preventDefault(); filterMemories(); }}>
              <Input aria-label="Search memories" placeholder="Search memories" bind:value={memoryQuery} />
              <Input aria-label="Memory kind filter" placeholder="Kind (optional)" bind:value={memoryKind} />
              <Input aria-label="Memory status filter" placeholder="Status (optional)" bind:value={memoryStatus} />
              <Button variant="outline" size="sm" type="submit">Filter</Button>
            </form>
            {#each memories as m (m.id)}
              <article class="rounded-xl border p-3" data-testid={`memory-item-${m.id}`}>
                <label><input type="checkbox" aria-label={`Purge memory ${m.id}`} checked={selMemories.includes(m.id)} onchange={() => (selMemories = toggle(selMemories,m.id))} /> Purge</label>
                <div class="text-sm font-medium">{m.text}{m.pinned ? ' · Pinned' : ''}</div>
                <div class="mt-1 text-xs text-muted-foreground">{m.kind} · {m.status} · importance {Number(m.importance).toFixed(2)} · ×{m.mentionCount} · {m.veracity}</div>
                <blockquote class="mt-2 rounded-lg bg-muted px-2 py-1 text-xs">“{m.quote}”</blockquote>
                <a class="text-xs underline" href={`/s/${encodeURIComponent(m.sessionId)}`}>Session {m.sessionId}</a>
                {#if editingMemory === m.id}
                  <FloatingInput label="Memory text" bind:value={editMemoryText} />
                  <Button size="sm" onclick={() => mutateMemory('edit', m)}>Save edit</Button>
                  <Button size="sm" variant="ghost" onclick={() => (editingMemory = 0)}>Cancel</Button>
                {:else}
                  <div class="mt-2 flex flex-wrap gap-2">
                    <Button size="sm" variant="outline" onclick={() => mutateMemory('forget', m)}>Forget</Button>
                    <Button size="sm" variant="outline" onclick={() => mutateMemory('park', m)}>Park</Button>
                    <Button size="sm" variant="outline" onclick={() => mutateMemory('pin', m)}>{m.pinned ? 'Unpin' : 'Pin'}</Button>
                    <Button size="sm" variant="outline" onclick={() => mutateMemory('promote', m)}>Promote to Rule</Button>
                    <Button size="sm" variant="ghost" onclick={() => { editingMemory = m.id; editMemoryText = m.text; }}>Edit</Button>
                  </div>
                {/if}
              </article>
            {/each}
          </section>
        {:else if tab === 'legacy'}
          <section class="animate-rise flex flex-col gap-2 rounded-2xl border bg-card p-4" data-testid="memory-legacy">
            <h2 class="text-sm font-semibold">Legacy observations</h2>
            {#each observations as o (o.id)}
              <article class="rounded-xl border p-3"><div>{o.quote}</div><div class="text-xs text-muted-foreground">{o.kind} · Session {o.sessionId}</div><Button size="sm" onclick={() => run(() => promoteObservationMemory(o.id))}>Promote to Memory</Button></article>
            {/each}
          </section>
        {:else}
          <section class="animate-rise flex flex-col gap-2 rounded-2xl border bg-card p-4" data-testid="memory-episodes">
            <h2 class="text-sm font-semibold">Episodes (read-only)</h2>
            {#each episodes as ep (ep.id)}<article class="rounded-xl border p-3 text-sm"><label><input type="checkbox" aria-label={`Purge episode ${ep.id}`} checked={selEpisodes.includes(ep.id)} onchange={() => (selEpisodes = toggle(selEpisodes,ep.id))} /> Purge</label>{ep.text}<a class="block text-xs underline" href={`/s/${encodeURIComponent(ep.sessionId)}`}>Session {ep.sessionId}</a></article>{/each}
          </section>
        {/if}

      {#if statusError}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{statusError}</p>
      {/if}
      {#if error}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}
      {#if notice}
        <p role="status" class="rounded-xl bg-green-500/10 px-4 py-2 text-sm text-green-300">{notice}</p>
      {/if}

      {#if loading}
        <div class="h-24 animate-pulse rounded-2xl bg-card"></div>
      {:else}
        <!-- Stats: queue size, approval latency, last nightly run and ledger. -->
        <section class="animate-rise flex flex-col gap-2 rounded-2xl border bg-card p-4" data-testid="memory-stats">
          <h2 class="text-sm font-semibold">Status</h2>
          <div class="grid gap-2 text-sm sm:grid-cols-2">
            <div>Pending proposals: <span class="font-medium tabular-nums" data-testid="memory-pending">{status?.queue?.pending ?? pendingCount}</span></div>
            <div>Queue budget: <span class="tabular-nums">{status?.queue?.budget ?? '—'}</span></div>
            <div>Oldest pending age: <span class="tabular-nums">{status?.queue?.oldestPendingAgeHours ? `${Number(status.queue.oldestPendingAgeHours).toFixed(1)} hours` : '—'}</span></div>
            <div>Approval latency: <span class="tabular-nums">{latencyLabel(status?.queue?.avgApprovalLatencyHours)}</span></div>
            <div>Ledger rows: <span class="tabular-nums">{status?.ledgerSize ?? ledger.length}</span></div>
          </div>
          <div class="text-xs text-muted-foreground">
            {#if lastNightly}
              Last nightly: {lastNightly.stage} · {lastNightly.status}
            {:else}
              Last nightly: none yet
            {/if}
            {#if lastLedger}
              <span> · Last run: {lastLedger.stage}</span>
            {/if}
          </div>
        </section>

        <!-- Rules: list newest first, search through FTS5. -->
        <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4" data-testid="memory-rules">
          <h2 class="text-sm font-semibold">Rules ({rules.length})</h2>
          <form
            class="flex gap-2"
            onsubmit={(e) => {
              e.preventDefault();
              searchRules();
            }}
          >
            <Input aria-label="Search rules" placeholder="Search rules" bind:value={ruleQuery} />
            <Button variant="outline" size="sm" type="submit" disabled={searching}>Search</Button>
          </form>
          {#if rules.length === 0}
            <p class="text-sm text-muted-foreground">No rules yet. Approved proposals land here.</p>
          {:else}
            <ul class="flex flex-col gap-2">
              {#each rules as r (r.ruleId)}
                <li class="flex items-start gap-3 rounded-xl border p-3" data-testid={`memory-rule-${r.ruleId}`}>
                  <label class="-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5"><input
                    type="checkbox"
                    aria-label={`Purge rule ${r.ruleId}`}
                    checked={selRules.includes(r.ruleId)}
                    onchange={() => (selRules = toggle(selRules, r.ruleId))}
                    class="size-5 cursor-pointer accent-primary"
                  /></label>
                  <div class="min-w-0 flex-1">
                    <div class="text-sm">{r.text}</div>
                    <div class="mt-1 text-xs text-muted-foreground">{r.kind} · {r.subjectKey} · {r.status} · v{r.version}</div>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="shrink-0"
                    disabled={promoteBusy === r.ruleId}
                    aria-label={`Promote rule ${r.ruleId}`}
                    title="Propose this Rule as an edit to the nearest AGENTS.md"
                    onclick={() => promoteRule(r)}
                  >
                    {promoteBusy === r.ruleId ? 'Promoting…' : 'Promote'}
                  </Button>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Observations: grounded candidates, searchable, never injected. -->
        <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4" data-testid="memory-observations">
          <h2 class="text-sm font-semibold">Observations ({observations.length})</h2>
          <form
            class="flex gap-2"
            onsubmit={(e) => {
              e.preventDefault();
              searchObs();
            }}
          >
            <Input aria-label="Search observations" placeholder="Search observations" bind:value={obsQuery} />
            <Button variant="outline" size="sm" type="submit" disabled={searching}>Search</Button>
          </form>
          {#if observations.length === 0}
            <p class="text-sm text-muted-foreground">No observations yet. The miner stores grounded candidates here.</p>
          {:else}
            <ul class="flex flex-col gap-2">
              {#each observations as o (o.id)}
                <li class="flex items-start gap-3 rounded-xl border p-3" data-testid={`memory-observation-${o.id}`}>
                  <label class="-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5"><input
                    type="checkbox"
                    aria-label={`Purge observation ${o.id}`}
                    checked={selObs.includes(o.id)}
                    onchange={() => (selObs = toggle(selObs, o.id))}
                    class="size-5 cursor-pointer accent-primary"
                  /></label>
                  <div class="min-w-0 flex-1">
                    <div class="text-sm">{o.quote}</div>
                    <div class="mt-1 text-xs text-muted-foreground">{o.kind} · {o.subjectKey} · session {o.sessionId} · ×{o.evidenceCount}</div>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Queue: each proposal with quotes and source Sessions. -->
        <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4" data-testid="memory-queue">
          <div class="flex items-center gap-2">
            <h2 class="text-sm font-semibold">Proposals waiting ({pendingCount})</h2>
            <Button variant="outline" size="sm" class="ml-auto" disabled={pendingCount === 0} onclick={batchSelected}>Approve selected</Button>
          </div>
          {#if queue.length === 0}
            <p class="text-sm text-muted-foreground">No proposals. A repeated subject or a /remember lands here.</p>
          {:else}
            {#each queueGroups as g (g.sessionId || 'none')}
              <div class="flex flex-col gap-2">
                <div class="flex items-center gap-2">
                  <span class="text-xs text-muted-foreground">Session {g.sessionId || '(no session)'} · {g.rows.length}</span>
                  <Button variant="ghost" size="sm" class="ml-auto" onclick={() => batchSession(g.sessionId, g.rows)}>
                    Approve session ({g.rows.length})
                  </Button>
                </div>
                <ul class="flex flex-col gap-2">
                  {#each g.rows as { item, proposal } (item.id)}
                    <li class="rounded-xl border p-3" data-testid={`memory-queue-${item.id}`}>
                      <div class="flex items-start gap-3">
                        <label class="-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5"><input
                          type="checkbox"
                          aria-label={`Select proposal ${item.id}`}
                          checked={selQueue.includes(item.id)}
                          onchange={() => (selQueue = toggle(selQueue, item.id))}
                          class="size-5 cursor-pointer accent-primary"
                        /></label>
                        <div class="min-w-0 flex-1">
                          <div class="text-xs text-muted-foreground">#{item.id} · {item.status} · {proposal.kind} · {proposal.subjectKey}</div>
                          {#if editingId === item.id}
                            <FloatingInput label="Rule text" bind:value={editText} />
                          {:else}
                            <div class="mt-1 text-sm">{proposal.text}</div>
                          {/if}
                          {#if proposal.quotes.length}
                            <ul class="mt-2 flex flex-col gap-1">
                              {#each proposal.quotes as q (q)}
                                <li class="rounded-lg bg-muted px-2 py-1 text-xs">“{q}”</li>
                              {/each}
                            </ul>
                          {/if}
                          <div class="mt-1 text-xs text-muted-foreground">
                            Sessions: {proposal.sessions.join(', ') || '—'}{proposal.reason ? ` · ${proposal.reason}` : ''}
                            {#if proposal.relation} · {proposal.relation} {proposal.relatedRuleId}{/if}
                          </div>
                          {#if item.status === 'pending'}
                            <div class="mt-2 flex gap-2">
                              {#if editingId === item.id}
                                <Button size="sm" onclick={() => approveItem(item, editText.trim())}>Save</Button>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  onclick={() => {
                                    editingId = 0;
                                    editText = '';
                                  }}>Cancel</Button
                                >
                              {:else}
                                <Button size="sm" onclick={() => approveItem(item)}>Approve</Button>
                                <Button variant="ghost" size="sm" onclick={() => startEdit(item, proposal)}>Edit</Button>
                                <Button variant="ghost" size="sm" onclick={() => rejectItem(item)}>Reject</Button>
                              {/if}
                            </div>
                          {/if}
                        </div>
                      </div>
                    </li>
                  {/each}
                </ul>
              </div>
            {/each}
            {#if queue.some((q) => q.status !== 'pending')}
              <p class="text-xs text-muted-foreground">Decided proposals stay as labels.</p>
            {/if}
          {/if}
        </section>

        <!-- Promotions: Rules filed as project knowledge. Each item shows its
          target path and diff preview with approve and reject; approving
          writes the worktree file and leaves it uncommitted for review. -->
        <PromotionQueue bind:this={promoQueue} />

        <!-- Profile: standing instructions with versions. -->
        <ProfileSettings />

        <!-- Ledger: every stage row, oldest first. -->
        <section class="animate-rise flex flex-col gap-2 rounded-2xl border bg-card p-4" data-testid="memory-ledger">
          <h2 class="text-sm font-semibold">Ledger ({ledger.length})</h2>
          {#if ledger.length === 0}
            <p class="text-xs text-muted-foreground">No runs yet. Queue decisions and nightly stages write here.</p>
          {:else}
            <ul class="flex max-h-64 flex-col gap-1 overflow-auto">
              {#each ledger.slice(-20).reverse() as l (l.id)}
                <li class="flex items-start gap-3 text-xs" data-testid={`memory-ledger-${l.id}`}>
                  <label class="-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5"><input
                    type="checkbox"
                    aria-label={`Purge ledger ${l.id}`}
                    checked={selLedger.includes(l.id)}
                    onchange={() => (selLedger = toggle(selLedger, l.id))}
                    class="size-5 cursor-pointer accent-primary"
                  /></label>
                  <span class="min-w-0 flex-1"><span class="font-medium">{l.stage}</span> · {l.status} · {l.runId}</span>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Purge: the human delete path. -->
        <section class="animate-rise flex flex-col gap-3 rounded-2xl border bg-card p-4" data-testid="memory-purge">
          <h2 class="text-sm font-semibold">Purge</h2>
          <p class="text-xs text-muted-foreground">
            Selected: {counts.memories} memories, {counts.episodes} episodes, {counts.rules} rules, {counts.observations} observations, {counts.queue} queue items, {counts.ledger} ledger rows.
            This is the only delete path. Type PURGE to confirm.
          </p>
          <FloatingInput label="Type PURGE to confirm" bind:value={confirm} placeholder="PURGE" />
          <div>
            <Button variant="destructive" size="sm" disabled={!canPurge} onclick={runPurge}>
              {purging ? 'Purging…' : 'Purge selected'}
            </Button>
          </div>
          {#if purgeResult}
            <p role="status" class="rounded-xl bg-green-500/10 px-4 py-2 text-sm text-green-300">{purgeResult}</p>
          {/if}
        </section>
      {/if}
    </div>
  </div>
</div>
