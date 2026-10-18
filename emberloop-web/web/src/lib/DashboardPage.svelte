<script>
  import { onMount } from 'svelte';
  import { LayoutDashboard, ChevronRight } from '@lucide/svelte';
  import PageHeader from './ui/page-header.svelte';
  import Section from './ui/section.svelte';
  import Row from './ui/row.svelte';
  import StatCard from './ui/stat-card.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import { icons, statusIcons } from './icons.js';
  import { getDashboard } from './api.js';
  import { watch } from './live.js';
  import { buildPath } from './router.js';

  let { projects = [], onNavigate = () => {}, initialDashboard = null } = $props();
  const bootDashboard = typeof window !== 'undefined' ? window.__EL_BOOT__?.['/api/dashboard'] : null;
  const initialPayload = initialDashboard ?? bootDashboard;
  let groups = $state(initialPayload?.groups ?? {});
  let loaded = $state(Boolean(initialPayload));
  let error = $state('');
  let request = 0;
  let mounted = false;
  let refreshScheduled = false;

  const projectNames = $derived(new Map(projects.map((project) => [project.id, project.name])));
  const activeGroups = $derived(
    Object.entries(groups)
      .map(([projectId, group]) => ({
        projectId,
        name: projectId === 'none' ? 'No project' : projectNames.get(projectId) ?? 'Project not found',
        sessions: (group.sessions ?? []).filter((session) => session.status === 'working' || session.status === 'needs-you'),
        runs: (group.runs ?? []).filter((run) => run.status === 'running' || run.status === 'needs-you'),
      }))
      .filter((group) => group.sessions.length || group.runs.length)
      .sort((a, b) => a.name.localeCompare(b.name)),
  );

  // Flat lists for the first viewport: what needs the User, then what runs.
  // Each entry names its Project so one row structure serves every Project.
  const specLabel = (run) => String(run.spec ?? run.id).split('/').filter(Boolean).at(-2) || run.spec || run.id;
  const entries = $derived(
    activeGroups.flatMap((group) => [
      ...group.sessions.map((session) => ({
        kind: 'session',
        testid: 'dashboard-session',
        key: `s:${session.id}`,
        title: session.title || session.id,
        needs: session.status === 'needs-you',
        meta: ['Session', group.name, session.harness].filter(Boolean).join(' · '),
        path: buildPath({ name: 'session', id: session.id }),
      })),
      ...group.runs.map((run) => ({
        kind: 'run',
        testid: 'dashboard-run',
        key: `r:${run.id}`,
        title: specLabel(run),
        needs: run.status === 'needs-you',
        meta: ['Run', group.name, run.counts?.total ? `${run.counts.merged ?? 0}/${run.counts.total} merged` : ''].filter(Boolean).join(' · '),
        path: buildPath({ name: 'project-run', projectId: group.projectId, runId: run.id }),
      })),
    ]),
  );
  const needsYou = $derived(entries.filter((entry) => entry.needs));
  const running = $derived(entries.filter((entry) => !entry.needs));
  const projectRows = $derived(
    activeGroups.map((group) => {
      const items = [...group.sessions.map((s) => s.status === 'needs-you'), ...group.runs.map((r) => r.status === 'needs-you')];
      const needs = items.filter(Boolean).length;
      const parts = [needs ? `${needs} needs you` : '', items.length - needs ? `${items.length - needs} running` : ''].filter(Boolean);
      return { group, needs, meta: parts.join(' · ') };
    }),
  );

  async function refresh() {
    const token = ++request;
    try {
      const payload = await getDashboard();
      if (token !== request) return;
      groups = payload?.groups ?? {};
      error = '';
    } catch {
      if (token === request) error = 'Could not load the Dashboard.';
    } finally {
      if (token === request) loaded = true;
    }
  }
  function scheduleRefresh() {
    if (refreshScheduled) return;
    refreshScheduled = true;
    queueMicrotask(() => {
      refreshScheduled = false;
      if (mounted) void refresh();
    });
  }
  function onLiveChange(topic) {
    if (topic === 'hello' && !loaded) return;
    scheduleRefresh();
  }

  onMount(() => {
	const snapshot = typeof document !== 'undefined' ? document.getElementById('el-ssr-dashboard') : null;
	if (snapshot) requestAnimationFrame(() => requestAnimationFrame(() => snapshot.remove()));
    mounted = true;
	if (!initialPayload) void refresh();
    const stopSessions = watch('sessions', onLiveChange);
    const stopRuns = watch('runs', onLiveChange);
    return () => {
      mounted = false;
      request++;
      stopSessions();
      stopRuns();
    };
  });
</script>

<div class="min-h-0 flex-1 overflow-auto" data-testid="dashboard">
  <div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-4 pt-4 pb-[max(4rem,env(safe-area-inset-bottom))] md:px-6 md:pt-6">
    <PageHeader title="Dashboard" icon={LayoutDashboard} scope="All projects" scopeIcon={icons.project} class="animate-rise" />

    {#if error && loaded && !activeGroups.length}
      <ErrorState title="Could not load the Dashboard" detail={error} onRetry={refresh} />
    {:else if error}
      <ErrorState compact title={error} onRetry={refresh} class="animate-rise" />
    {/if}
    {#if !loaded}
      <div role="status" aria-label="Loading Dashboard" class="flex flex-col gap-6">
        <div class="grid grid-cols-2 gap-3 md:gap-4">
          {#each [0, 1] as i (i)}<Skeleton class="h-24 rounded-2xl" />{/each}
        </div>
        <div class="flex flex-col gap-2">
          {#each [0, 1, 2] as i (i)}<Skeleton class="h-14 rounded-xl" />{/each}
        </div>
      </div>
    {:else if !error || activeGroups.length}
      <div class="animate-rise grid grid-cols-2 gap-3 md:gap-4" data-testid="dashboard-stats">
        <StatCard label="Needs you" value={needsYou.length} tone={needsYou.length ? 'needs-you' : null} icon={statusIcons['needs-you']} caption={needsYou.length ? 'Open the first one' : 'All clear'} />
        <StatCard label="Running" value={running.length} tone={running.length ? 'working' : null} icon={statusIcons.working} caption={running.length ? 'Working now' : 'Nothing running'} />
      </div>

      {#if entries.length}
        {#if needsYou.length}
          <Section title="Needs you" icon={statusIcons['needs-you']} class="animate-rise" data-testid="dashboard-needs-you">
            <ul class="flex flex-col gap-1">
              {#each needsYou as entry (entry.key)}
                <li>
                  <Row title={entry.title} meta={entry.meta} status="needs-you" attention onclick={() => onNavigate(entry.path)} data-testid={entry.testid} />
                </li>
              {/each}
            </ul>
          </Section>
        {/if}
        {#if running.length}
          <Section title="Running" icon={statusIcons.working} class="animate-rise" data-testid="dashboard-running">
            <ul class="flex flex-col gap-1">
              {#each running as entry (entry.key)}
                <li>
                  <Row title={entry.title} meta={entry.meta} status="working" onclick={() => onNavigate(entry.path)} data-testid={entry.testid} />
                </li>
              {/each}
            </ul>
          </Section>
        {/if}
        <Section title="Projects" icon={icons.project} description="Open a Project to see all its Sessions and Runs." class="animate-rise">
          <ul class="flex flex-col gap-1">
            {#each projectRows as { group, needs, meta } (group.projectId)}
              <li>
                <Row
                  title={group.name}
                  meta={meta}
                  status={needs ? 'needs-you' : 'working'}
                  onclick={() => onNavigate(buildPath({ name: 'project', projectId: group.projectId }))}
                  data-testid="dashboard-project"
                  aria-label="Open {group.name}"
                >
                  {#snippet trailing()}<ChevronRight class="size-4 text-fg-3" aria-hidden="true" />{/snippet}
                </Row>
              </li>
            {/each}
          </ul>
        </Section>
      {:else}
        <EmptyState icon={statusIcons.done} title="Nothing needs attention" description="Running and needs-you work appears here." class="animate-rise" />
      {/if}
    {/if}
  </div>
</div>
