<!-- Rail 2 for Runs: spec discovery on top (repo directory for global Runs,
     fixed Project path for scoped Runs), then every Run newest first. A spec
     pick repaints RunsView through the shared discovery store; the view owns
     the preview table, Start button and live Run. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import Button from './ui/button.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import Section from './ui/section.svelte';
  import Row from './ui/row.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import { icons } from './icons.js';
  import { buildPath } from './router.js';
  import { runsStore } from './runsStore.js';
  import {
    discovery,
    ensureDiscoveryContext,
    initDiscoveryRepo,
    loadDiscovery,
    resetDiscovery,
    selectDiscovery,
  } from './runsDiscovery.svelte.js';

  let { workDir = '', runId = '', ticketId = '', projectId = null, projectsLoaded = true, onNavigate = () => {} } = $props();
  const scopedProject = $derived(projectId != null && projectId !== 'none');
  let projectRepoKey = '';

  let runs = $state([]);
  // False until the shared summary request answers, so a cold load never reads
  // "No Runs yet".
  let loaded = $state(false);
  const stopRuns = runsStore.subscribe((items) => {
    runs = [...items].sort((a, b) => String(b.updatedAt ?? '').localeCompare(String(a.updatedAt ?? '')));
  });
  const scopedRuns = $derived(projectId == null ? runs : runs.filter((run) => (run.projectId ?? '') === (projectId === 'none' ? '' : projectId)));

  // A deep link seeds the picker from the list summary. The full Run and its
  // Ticket bodies are fetched by the main view only when that Run opens.
  let seededFor = '';
  $effect(() => {
    const id = runId;
    if (!id || seededFor === id) return;
    const summary = scopedRuns.find((run) => run.id === id);
    if (!summary) return;
    seededFor = id;
    void ensureDiscoveryContext(summary);
  });

  // Project Runs bind spec discovery and starts to the selected Project path.
  // Changing Projects clears the previous discovery state before loading the
  // new Project's specs; global Runs keep their editable repo directory.
  $effect(() => {
    const id = projectId;
    const dir = workDir;
    const ready = projectsLoaded;
    if (id != null && id !== 'none') {
      const key = `${id}:${dir}:${ready}`;
      if (projectRepoKey === key) return;
      projectRepoKey = key;
      if (!dir) {
        resetDiscovery();
        if (ready) discovery.message = 'Project folder is unavailable.';
      } else {
        void loadDiscovery(dir);
      }
      return;
    }
    projectRepoKey = '';
    if (dir) discovery.repo = dir;
  });

  onMount(() => {
    if (!scopedProject) {
      initDiscoveryRepo(workDir);
      // A cold deep link waits for the list summary to seed the picker.
      if (!runId && discovery.repo && !discovery.specs.length && !discovery.selected) void loadDiscovery();
    }
    void runsStore.load().then(
      () => { loaded = true; },
      () => { loaded = true; },
    );
  });

  onDestroy(stopRuns);
  const needsYouRuns = $derived(scopedRuns.filter((run) => run.counts?.needsYou > 0));
  const needsYouCount = $derived(scopedRuns.reduce((count, run) => count + (run.counts?.needsYou ?? 0), 0));
  const RUN_STATUS = { running: 'working', 'needs-you': 'needs-you', done: 'done', failed: 'failed', paused: 'paused', abandoned: 'paused' };
  const runPath = (id) => buildPath(projectId == null ? { name: 'run', runId: id } : { name: 'project-run', projectId, runId: id });
  const runsPath = () => buildPath(projectId == null ? { name: 'runs' } : { name: 'project-runs', projectId });
  const specLabel = (run) => String(run.spec ?? run.id).split('/').filter(Boolean).at(-2) || run.id;
  const merged = (run) => run.counts?.merged ?? 0;
  const total = (run) => run.counts?.total ?? 0;
</script>

<div class="flex min-h-0 flex-1 flex-col gap-3" data-testid="runs-sidebar">
  {#if needsYouCount}
    <div class="animate-rise flex flex-col gap-1 rounded-2xl border border-status-needs-you/30 bg-status-needs-you/10 p-2" data-testid="needs-you-strip">
      <span class="px-2 pt-0.5 text-caption font-medium text-status-needs-you">Needs you ({needsYouCount})</span>
      {#each needsYouRuns as run (run.id)}
        <button
          type="button"
          class="motion-standard line-clamp-2 rounded-lg px-2 py-2 text-left text-body hover:bg-status-needs-you/15 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none max-md:min-h-11"
          onclick={() => onNavigate(runPath(run.id))}
        >{specLabel(run)} · {run.counts.needsYou} Ticket{run.counts.needsYou === 1 ? '' : 's'} need attention</button>
      {/each}
    </div>
  {/if}
  <Section title="Specs" headingLevel={2} collapsible open={!runId} data-testid="runs-specs">
    {#if scopedProject}
      <div class="flex flex-col gap-2" data-testid="project-runs-discovery">
        <p class="break-all px-1 text-caption text-fg-3">Specs in {workDir || 'Project folder'}</p>
        <Button type="button" size="sm" variant="secondary" class="w-full" disabled={!workDir} onclick={() => loadDiscovery(workDir)}>Find specs</Button>
      </div>
    {:else}
      <form
        class="flex flex-col gap-2"
        onsubmit={(e) => {
          e.preventDefault();
          loadDiscovery();
        }}
      >
        <FloatingInput class="min-w-0 flex-1" label="Repo directory" bind:value={discovery.repo} placeholder="/path/to/repo" />
        <Button type="submit" size="sm" variant="secondary" class="w-full">Find specs</Button>
      </form>
    {/if}
    {#if discovery.error}
      <p role="alert" class="rounded-xl bg-status-failed/10 px-3 py-2 text-caption text-status-failed">{discovery.error}</p>
    {/if}
    {#if discovery.message}
      <p class="rounded-xl border px-3 py-2 text-caption text-fg-3">{discovery.message}</p>
    {/if}
    {#if discovery.specs.length}
      <ul class="flex min-h-0 flex-col gap-0.5">
        {#each discovery.specs as s (s.spec)}
          <li>
            <Row
              title={s.title || s.spec}
              meta="{s.tickets} Ticket{s.tickets === 1 ? '' : 's'}"
              icon={icons.ticket}
              selected={s.spec === discovery.selected}
              onclick={() => selectDiscovery(s.spec)}
            />
          </li>
        {/each}
      </ul>
    {/if}
  </Section>
  <Section title="Runs" headingLevel={2} class="min-h-0 flex-1" data-testid="runs-list">
    {#snippet actions()}
      <Button type="button" variant="ghost" size="sm" onclick={() => onNavigate(runsPath())}>New run</Button>
    {/snippet}
    <ul class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto">
      {#each scopedRuns as r (r.id)}
        <li class="animate-rise">
          <Row
            title={specLabel(r)}
            meta="{merged(r)}/{total(r)} merged"
            status={RUN_STATUS[r.status] ?? 'working'}
            attention={r.status === 'needs-you'}
            current={r.id === runId}
            onclick={() => onNavigate(runPath(r.id))}
          />
        </li>
      {:else}
        {#if loaded}
          <li><EmptyState icon={icons.run} title="No Runs yet" description="Pick a spec, then start a Run." class="px-4 py-6" /></li>
        {:else}
          <li role="status" aria-label="Loading Runs" class="flex flex-col gap-1">
            {#each [0, 1, 2] as i (i)}<Skeleton class="h-12 rounded-xl" />{/each}
          </li>
        {/if}
      {/each}
    </ul>
  </Section>
</div>
