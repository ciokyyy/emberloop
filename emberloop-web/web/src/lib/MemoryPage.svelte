<!-- Memory page: the User's own bank. Opens on a summary (pending, new, Rules) and
     the proposals that wait for approval; Memories (with legacy observations and
     Episodes), Rules, Observations, Promotions, Profile, Status, Ledger and Purge
     sit in collapsed Sections. Purge needs the typed PURGE confirmation and
     reports what it removed. Loads once on mount and never polls; only the
     buttons fetch again. -->
<script>
  import PageHeader from './ui/page-header.svelte';
  import IconButton from './ui/icon-button.svelte';
  import { icons } from './icons.js';
  import Button from './ui/button.svelte';
  import { CheckCheck, ChevronDown, Eye, ListChecks, ScrollText, Trash2 } from '@lucide/svelte';
  import Chip from './ui/chip.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import StatCard from './ui/stat-card.svelte';
  import Input from './ui/input.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import ProfileSettings from './ProfileSettings.svelte';
  import PromotionQueue from './PromotionQueue.svelte';
  import {
    getMemories, getEpisodes, forgetMemory, parkMemory, pinMemory,
    getNewMemoryCount,
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
    const visitKey = `emberloop.memory.lastVisit.${userId}`;
    const previousVisit = !counterInitialized && typeof localStorage !== 'undefined'
      ? Number(localStorage.getItem(visitKey) || 0)
      : 0;
    try {
      const [st, r, o, q, l, m, ep, newCount] = await Promise.all([
        getMemoryStatus(), getRules(), getObservations(), getRulesQueue(), getMemoryLedger(), getMemories(), getEpisodes(),
        counterInitialized ? Promise.resolve({ count: newSinceVisit }) : getNewMemoryCount(previousVisit),
      ]);
      status = st;
      rules = Array.isArray(r) ? r.map(normalizeRule) : [];
      observations = Array.isArray(o) ? o.map(normalizeObservation) : [];
      queue = Array.isArray(q) ? q.map(normalizeQueueItem) : [];
      ledger = Array.isArray(l) ? l.map(normalizeLedger) : [];
      memories = Array.isArray(m) ? m : [];
      episodes = Array.isArray(ep) ? ep : [];
      if (!counterInitialized && typeof localStorage !== 'undefined') {
        newSinceVisit = Number(newCount?.count) || 0;
        localStorage.setItem(visitKey, String(Date.now()));
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
  const oldestPending = $derived(status?.queue?.oldestPendingAgeHours ? `${Number(status.queue.oldestPendingAgeHours).toFixed(1)}h` : '');

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

{#snippet purgeBox(label, checked, onchange)}
  <label class="-m-2.5 grid shrink-0 cursor-pointer place-items-center p-2.5"><input
    type="checkbox"
    aria-label={label}
    {checked}
    {onchange}
    class="size-5 cursor-pointer accent-primary"
  /></label>
{/snippet}

<div class="flex min-h-0 flex-1 flex-col" data-testid="memory-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-[max(4rem,env(safe-area-inset-bottom))] md:px-6">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <PageHeader
        title="Memory"
        icon={icons.memory}
        hint="Your standing instructions, approved notes and pending proposals. Only your bank shows here."
        onBack={onBack}
        backOn="desktop"
        class="animate-rise"
      >
        {#snippet meta()}
          {#if status}
            {#if available}
              <Chip tone="done" dot data-testid="memory-status">Available</Chip>
            {:else}
              <Chip tone="failed" dot data-testid="memory-status">Unavailable</Chip>
            {/if}
          {:else if loading}
            <Chip tone="paused" data-testid="memory-status">Loading…</Chip>
          {:else}
            <Chip tone="failed" dot data-testid="memory-status">Unavailable</Chip>
          {/if}
        {/snippet}
        {#snippet actions()}
          <IconButton icon={icons.refresh} label="Refresh memory" variant="outline" spin={loading} onclick={load} />
        {/snippet}
      </PageHeader>

      {#if statusError}
        <p role="alert" class="rounded-xl bg-status-failed/10 px-4 py-2 text-body text-status-failed">{statusError}</p>
      {/if}
      {#if error}
        <p role="alert" class="rounded-xl bg-status-failed/10 px-4 py-2 text-body text-status-failed">{error}</p>
      {/if}
      {#if notice}
        <p role="status" class="rounded-xl bg-status-done/10 px-4 py-2 text-body text-status-done">{notice}</p>
      {/if}

      <!-- Summary: what needs the User, then counts. -->
      <div class="grid grid-cols-3 gap-2 md:gap-4" data-testid="memory-summary">
        <StatCard
          label="Pending"
          value={status?.queue?.pending ?? pendingCount}
          caption={(status?.queue?.pending ?? pendingCount) ? (oldestPending ? `oldest ${oldestPending}` : 'need approval') : 'all clear'}
          tone={(status?.queue?.pending ?? pendingCount) ? 'needs-you' : 'done'}
          icon={icons.memory}
        />
        <StatCard label="New" value={newSinceVisit} caption="since last visit" icon={icons.memory} />
        <StatCard label="Rules" value={rules.length} caption={`${memories.length} memories`} icon={icons.memory} />
      </div>
      <p class="sr-only" data-testid="memory-header-summary">
        {newSinceVisit} new since last visit · Queue {status?.queue?.pending ?? pendingCount} pending · Oldest pending {status?.queue?.oldestPendingAgeHours ? `${Number(status.queue.oldestPendingAgeHours).toFixed(1)} hours` : '—'}
      </p>

      <!-- Queue: each proposal with quotes and source Sessions. -->
      <Section title={`Proposals waiting (${pendingCount})`} icon={icons.memory} class="animate-rise" data-testid="memory-queue">
        {#snippet actions()}
          <IconButton icon={CheckCheck} label="Approve selected" variant="outline" disabled={pendingCount === 0} onclick={batchSelected} />
        {/snippet}
        {#if loading && !queue.length}
          <div role="status" aria-label="Loading proposals"><Skeleton class="h-24 rounded-2xl" /></div>
        {:else if queue.length === 0}
          <EmptyState icon={icons.memory} title="No proposals" description="A repeated subject or a /remember lands here." class="py-6" />
        {:else}
          {#each queueGroups as g (g.sessionId || 'none')}
            <div class="flex flex-col gap-2">
              <div class="flex min-h-9 items-center gap-2 px-1">
                <span class="min-w-0 flex-1 truncate text-caption text-fg-3">Session {g.sessionId || '(no session)'} · {g.rows.length}</span>
                <Button variant="ghost" size="sm" onclick={() => batchSession(g.sessionId, g.rows)}>
                  Approve session ({g.rows.length})
                </Button>
              </div>
              <Panel tag="ul">
                {#each g.rows as { item, proposal } (item.id)}
                  <li class="p-4" data-testid={`memory-queue-${item.id}`}>
                    <div class="flex items-start gap-3">
                      {@render purgeBox(`Select proposal ${item.id}`, selQueue.includes(item.id), () => (selQueue = toggle(selQueue, item.id)))}
                      <div class="flex min-w-0 flex-1 flex-col gap-2">
                        <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-caption text-fg-3">
                          <Chip tone={item.status === 'pending' ? 'needs-you' : 'paused'} dot>{item.status}</Chip>
                          <span class="tabular-nums">#{item.id}</span>
                          <span>{proposal.kind} · {proposal.subjectKey}</span>
                        </div>
                        {#if editingId === item.id}
                          <FloatingInput label="Rule text" bind:value={editText} />
                        {:else}
                          <div class="text-body break-words text-fg-1">{proposal.text}</div>
                        {/if}
                        {#if proposal.quotes.length}
                          <details class="group">
                            <summary class="flex min-h-9 cursor-pointer list-none items-center gap-1 rounded-lg text-caption text-fg-3 outline-none hover:text-fg-1 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 [&::-webkit-details-marker]:hidden">
                              <ChevronDown class="motion-standard size-3.5 -rotate-90 group-open:rotate-0" aria-hidden="true" />
                              Evidence · {proposal.quotes.length}
                            </summary>
                            <ul class="mt-1 flex flex-col gap-1">
                              {#each proposal.quotes as q (q)}
                                <li class="rounded-lg bg-muted px-3 py-2 text-caption text-fg-2">“{q}”</li>
                              {/each}
                            </ul>
                          </details>
                        {/if}
                        <div class="text-caption text-fg-3">
                          Sessions: {proposal.sessions.join(', ') || '—'}{proposal.reason ? ` · ${proposal.reason}` : ''}
                          {#if proposal.relation} · {proposal.relation} {proposal.relatedRuleId}{/if}
                        </div>
                        {#if item.status === 'pending'}
                          <div class="flex gap-2 max-md:[&>*]:flex-1">
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
              </Panel>
            </div>
          {/each}
          {#if queue.some((q) => q.status !== 'pending')}
            <p class="text-caption text-fg-3">Decided proposals stay as labels.</p>
          {/if}
        {/if}
      </Section>

      <!-- Memories, legacy observations and Episodes behind one tab strip. -->
      <Section title={`Memories (${memories.length})`} icon={icons.memory} collapsible open={false} class="animate-rise" data-testid="memory-items-section">
        <div role="group" aria-label="Memory tabs" class="flex w-full gap-0.5 overflow-x-auto rounded-full bg-muted p-1 md:w-fit">
          {#each [['memories', `Memories · ${newSinceVisit} new`], ['legacy', 'Legacy observations'], ['episodes', 'Episodes']] as [id, label] (id)}
            <button
              type="button"
              class="motion-standard min-h-9 shrink-0 rounded-full px-3 text-caption whitespace-nowrap outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 max-md:text-body {tab === id ? 'bg-surface-3 text-fg-1 shadow-elev-1' : 'text-fg-2 hover:text-fg-1'}"
              aria-pressed={tab === id}
              onclick={() => (tab = id)}
            >{label}</button>
          {/each}
        </div>
        {#if tab === 'memories'}
          <div class="flex flex-col gap-3" data-testid="memory-items">
            <form class="grid gap-2 sm:grid-cols-[minmax(0,1fr)_9rem_9rem_auto]" onsubmit={(e) => { e.preventDefault(); filterMemories(); }}>
              <Input aria-label="Search memories" placeholder="Search memories" bind:value={memoryQuery} />
              <Input aria-label="Memory kind filter" placeholder="Kind (optional)" bind:value={memoryKind} />
              <Input aria-label="Memory status filter" placeholder="Status (optional)" bind:value={memoryStatus} />
              <Button variant="outline" type="submit">Filter</Button>
            </form>
            {#if memories.length === 0}
              <EmptyState icon={icons.memory} title="No memories" description="Automatic extraction files them after settled turns." class="py-6" />
            {:else}
              <Panel tag="ul">
                {#each memories as m (m.id)}
                  <li class="flex flex-col gap-2 p-4" data-testid={`memory-item-${m.id}`}>
                    <div class="flex items-start gap-3">
                      <div class="flex min-w-0 flex-1 flex-col gap-1">
                        <div class="text-body break-words text-fg-1">{m.text}{m.pinned ? ' · Pinned' : ''}</div>
                        <div class="text-caption text-fg-3">{m.kind} · {m.status} · importance {Number(m.importance).toFixed(2)} · ×{m.mentionCount} · {m.veracity}</div>
                      </div>
                      <label class="flex min-h-9 shrink-0 cursor-pointer items-center gap-1.5 text-caption text-fg-3 max-md:min-h-11"><input type="checkbox" aria-label={`Purge memory ${m.id}`} checked={selMemories.includes(m.id)} onchange={() => (selMemories = toggle(selMemories, m.id))} class="size-5 cursor-pointer accent-primary" /> Purge</label>
                    </div>
                    <blockquote class="rounded-lg bg-muted px-3 py-2 text-caption text-fg-2">“{m.quote}”</blockquote>
                    <a class="w-fit text-caption text-fg-3 underline underline-offset-4 hover:text-fg-1 max-md:flex max-md:min-h-11 max-md:items-center" href={`/s/${encodeURIComponent(m.sessionId)}`}>Session {m.sessionId}</a>
                    {#if editingMemory === m.id}
                      <FloatingInput label="Memory text" bind:value={editMemoryText} />
                      <div class="flex gap-2 max-md:[&>*]:flex-1">
                        <Button size="sm" onclick={() => mutateMemory('edit', m)}>Save edit</Button>
                        <Button size="sm" variant="ghost" onclick={() => (editingMemory = 0)}>Cancel</Button>
                      </div>
                    {:else}
                      <div class="flex flex-wrap gap-2">
                        <Button size="sm" variant="outline" onclick={() => mutateMemory('forget', m)}>Forget</Button>
                        <Button size="sm" variant="outline" onclick={() => mutateMemory('park', m)}>Park</Button>
                        <Button size="sm" variant="outline" onclick={() => mutateMemory('pin', m)}>{m.pinned ? 'Unpin' : 'Pin'}</Button>
                        <Button size="sm" variant="outline" onclick={() => mutateMemory('promote', m)}>Promote to Rule</Button>
                        <Button size="sm" variant="ghost" onclick={() => { editingMemory = m.id; editMemoryText = m.text; }}>Edit</Button>
                      </div>
                    {/if}
                  </li>
                {/each}
              </Panel>
            {/if}
          </div>
        {:else if tab === 'legacy'}
          <div class="flex flex-col gap-3" data-testid="memory-legacy">
            {#if observations.length === 0}
              <EmptyState icon={icons.memory} title="No observations" class="py-6" />
            {:else}
              <Panel tag="ul">
                {#each observations as o (o.id)}
                  <li class="flex flex-col gap-2 p-4">
                    <div class="text-body break-words text-fg-1">{o.quote}</div>
                    <div class="text-caption text-fg-3">{o.kind} · Session {o.sessionId}</div>
                    <div><Button size="sm" variant="outline" onclick={() => run(() => promoteObservationMemory(o.id))}>Promote to Memory</Button></div>
                  </li>
                {/each}
              </Panel>
            {/if}
          </div>
        {:else}
          <div class="flex flex-col gap-3" data-testid="memory-episodes">
            {#if episodes.length === 0}
              <EmptyState icon={icons.session} title="No Episodes" class="py-6" />
            {:else}
              <Panel tag="ul">
                {#each episodes as ep (ep.id)}
                  <li class="flex flex-col gap-1 p-4 text-body">
                    <label class="flex min-h-9 w-fit cursor-pointer items-center gap-1.5 text-caption text-fg-3 max-md:min-h-11"><input type="checkbox" aria-label={`Purge episode ${ep.id}`} checked={selEpisodes.includes(ep.id)} onchange={() => (selEpisodes = toggle(selEpisodes, ep.id))} class="size-5 cursor-pointer accent-primary" /> Purge</label>
                    <span class="break-words">{ep.text}</span>
                    <a class="w-fit text-caption text-fg-3 underline underline-offset-4 hover:text-fg-1 max-md:flex max-md:min-h-11 max-md:items-center" href={`/s/${encodeURIComponent(ep.sessionId)}`}>Session {ep.sessionId}</a>
                  </li>
                {/each}
              </Panel>
            {/if}
          </div>
        {/if}
      </Section>

      {#if !loading}
        <!-- Rules: list newest first, search through FTS5. -->
        <Section title={`Rules (${rules.length})`} icon={ListChecks} collapsible open={false} class="animate-rise" data-testid="memory-rules">
          <form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); searchRules(); }}>
            <Input aria-label="Search rules" placeholder="Search rules" bind:value={ruleQuery} />
            <Button variant="outline" type="submit" disabled={searching}>Search</Button>
          </form>
          {#if rules.length === 0}
            <EmptyState icon={icons.memory} title="No rules yet" description="Approved proposals land here." class="py-6" />
          {:else}
            <Panel tag="ul">
              {#each rules as r (r.ruleId)}
                <li class="flex items-start gap-3 p-4" data-testid={`memory-rule-${r.ruleId}`}>
                  {@render purgeBox(`Purge rule ${r.ruleId}`, selRules.includes(r.ruleId), () => (selRules = toggle(selRules, r.ruleId)))}
                  <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                    <div class="text-body break-words text-fg-1">{r.text}</div>
                    <div class="text-caption text-fg-3">{r.kind} · {r.subjectKey} · {r.status} · v{r.version}</div>
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
            </Panel>
          {/if}
        </Section>

        <!-- Observations: grounded candidates, searchable, never injected. -->
        <Section title={`Observations (${observations.length})`} icon={Eye} collapsible open={false} class="animate-rise" data-testid="memory-observations">
          <form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); searchObs(); }}>
            <Input aria-label="Search observations" placeholder="Search observations" bind:value={obsQuery} />
            <Button variant="outline" type="submit" disabled={searching}>Search</Button>
          </form>
          {#if observations.length === 0}
            <EmptyState icon={icons.memory} title="No observations yet" description="The miner stores grounded candidates here." class="py-6" />
          {:else}
            <Panel tag="ul">
              {#each observations as o (o.id)}
                <li class="flex items-start gap-3 p-4" data-testid={`memory-observation-${o.id}`}>
                  {@render purgeBox(`Purge observation ${o.id}`, selObs.includes(o.id), () => (selObs = toggle(selObs, o.id)))}
                  <div class="flex min-w-0 flex-1 flex-col gap-0.5">
                    <div class="text-body break-words text-fg-1">{o.quote}</div>
                    <div class="text-caption text-fg-3">{o.kind} · {o.subjectKey} · session {o.sessionId} · ×{o.evidenceCount}</div>
                  </div>
                </li>
              {/each}
            </Panel>
          {/if}
        </Section>

        <!-- Promotions: Rules filed as project knowledge. Each item shows its
          target path and diff preview with approve and reject; approving
          writes the worktree file and leaves it uncommitted for review. -->
        <PromotionQueue bind:this={promoQueue} embedded />

        <!-- Profile: standing instructions with versions. -->
        <ProfileSettings embedded />

        <!-- Status and ledger: every stage row, oldest first. -->
        <Section title="Status" icon={icons.quota} collapsible open={false} class="animate-rise" data-testid="memory-stats">
          <Panel padded divided={false}>
            <dl class="grid gap-x-6 gap-y-2 text-body sm:grid-cols-2">
              <div class="flex justify-between gap-3"><dt class="text-fg-3">Pending proposals</dt><dd class="tabular-nums" data-testid="memory-pending">{status?.queue?.pending ?? pendingCount}</dd></div>
              <div class="flex justify-between gap-3"><dt class="text-fg-3">Queue budget</dt><dd class="tabular-nums">{status?.queue?.budget ?? '—'}</dd></div>
              <div class="flex justify-between gap-3"><dt class="text-fg-3">Oldest pending age</dt><dd class="tabular-nums">{status?.queue?.oldestPendingAgeHours ? `${Number(status.queue.oldestPendingAgeHours).toFixed(1)} hours` : '—'}</dd></div>
              <div class="flex justify-between gap-3"><dt class="text-fg-3">Approval latency</dt><dd class="tabular-nums">{latencyLabel(status?.queue?.avgApprovalLatencyHours)}</dd></div>
              <div class="flex justify-between gap-3"><dt class="text-fg-3">Ledger rows</dt><dd class="tabular-nums">{status?.ledgerSize ?? ledger.length}</dd></div>
            </dl>
            <div class="text-caption text-fg-3">
              {#if lastNightly}
                Last nightly: {lastNightly.stage} · {lastNightly.status}
              {:else}
                Last nightly: none yet
              {/if}
              {#if lastLedger}
                <span> · Last run: {lastLedger.stage}</span>
              {/if}
            </div>
          </Panel>
          {#if status?.metrics}
            <Panel padded divided={false} data-testid="memory-production-metrics" aria-label="Memory production metrics">
              <dl class="grid gap-x-6 gap-y-2 text-body sm:grid-cols-2">
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Memories per day</dt><dd class="tabular-nums">{status.metrics.memoriesPerDay}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Forgotten within 7 days</dt><dd class="tabular-nums">{(Number(status.metrics.forgotten7Day || 0) * 100).toFixed(1)}%</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Promoted share</dt><dd class="tabular-nums">{(Number(status.metrics.promotedShare || 0) * 100).toFixed(1)}%</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Injection hit rate</dt><dd class="tabular-nums">{Number(status.metrics.injectionHitRate || 0).toFixed(2)}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Contradictions</dt><dd class="tabular-nums">{status.metrics.contradictions}</dd></div>
                <div class="flex justify-between gap-3"><dt class="text-fg-3">Extractor + embedder per day</dt><dd class="tabular-nums">${(Number(status.metrics.extractorCostUsd || 0) + Number(status.metrics.embedderCostUsd || 0)).toFixed(4)}</dd></div>
              </dl>
              <p class="text-caption text-fg-3">Forgotten target: under 20%.</p>
              {#if status.metrics.costAlert}<p role="alert" class="text-caption text-status-failed">Above $0.05/day</p>{/if}
            </Panel>
          {/if}
        </Section>

        <Section title={`Ledger (${ledger.length})`} icon={ScrollText} collapsible open={false} class="animate-rise" data-testid="memory-ledger">
          {#if ledger.length === 0}
            <EmptyState icon={icons.quota} title="No runs yet" description="Queue decisions and nightly stages write here." class="py-6" />
          {:else}
            <Panel tag="ul" class="max-h-72 overflow-auto">
              {#each ledger.slice(-20).reverse() as l (l.id)}
                <li class="flex items-start gap-3 px-4 py-3 text-caption" data-testid={`memory-ledger-${l.id}`}>
                  {@render purgeBox(`Purge ledger ${l.id}`, selLedger.includes(l.id), () => (selLedger = toggle(selLedger, l.id)))}
                  <span class="min-w-0 flex-1 break-words text-fg-2"><span class="text-fg-1">{l.stage}</span> · {l.status} · {l.runId}</span>
                </li>
              {/each}
            </Panel>
          {/if}
        </Section>

        <!-- Purge: the human delete path. -->
        <Section title="Purge" icon={Trash2} collapsible open={false} class="animate-rise" data-testid="memory-purge">
          <Panel padded divided={false}>
            <p class="text-caption text-fg-3">
              Selected: {counts.memories} memories, {counts.episodes} episodes, {counts.rules} rules, {counts.observations} observations, {counts.queue} queue items, {counts.ledger} ledger rows.
              This is the only delete path. Type PURGE to confirm.
            </p>
            <FloatingInput label="Type PURGE to confirm" bind:value={confirm} placeholder="PURGE" />
            <Button variant="destructive" class="w-fit max-md:w-full" disabled={!canPurge} onclick={runPurge}>
              {purging ? 'Purging…' : 'Purge selected'}
            </Button>
            {#if purgeResult}
              <p role="status" class="rounded-xl bg-status-done/10 px-4 py-2 text-body text-status-done">{purgeResult}</p>
            {/if}
          </Panel>
        </Section>
      {:else}
        <div role="status" aria-label="Loading memory" class="flex flex-col gap-3">
          <Skeleton class="h-12 rounded-2xl" />
          <Skeleton class="h-12 rounded-2xl" />
        </div>
      {/if}
    </div>
  </div>
</div>
