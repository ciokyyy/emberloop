<script>
  import { onMount } from 'svelte';
  import Button from './ui/button.svelte';
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
  <div class="mx-auto flex w-full max-w-5xl flex-col gap-5 px-4 py-6 sm:px-6">
    <header class="animate-rise">
      <p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Workspace</p>
      <h1 class="mt-1 text-2xl font-semibold tracking-tight">All projects</h1>
      <p class="mt-1 text-sm text-muted-foreground">Work that is running or needs your attention.</p>
    </header>

    {#if error}
      <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-3 text-sm text-destructive">{error}</p>
    {/if}
    {#if !loaded}
      <p role="status" class="rounded-xl border px-4 py-3 text-sm text-muted-foreground">Loading Dashboard…</p>
    {:else if activeGroups.length}
      <div class="grid gap-4">
        {#each activeGroups as group (group.projectId)}
          <section class="animate-rise rounded-2xl border bg-card p-4" aria-label={group.name} data-testid="dashboard-project">
            <div class="mb-3 flex items-center justify-between gap-3">
              <h2 class="truncate text-base font-semibold">{group.name}</h2>
              {#if group.projectId !== 'none'}
                <Button variant="ghost" size="sm" onclick={() => onNavigate(buildPath({ name: 'project', projectId: group.projectId }))}>Open project</Button>
              {:else}
                <Button variant="ghost" size="sm" onclick={() => onNavigate(buildPath({ name: 'project', projectId: 'none' }))}>Open no-project Sessions</Button>
              {/if}
            </div>
            {#if group.sessions.length}
              <div class="mb-3">
                <h3 class="mb-1 px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Sessions</h3>
                <ul class="flex flex-col gap-1">
                  {#each group.sessions as session (session.id)}
                    <li>
                      <Button
                        variant="ghost"
                        class="h-auto min-h-11 w-full justify-start text-left"
                        data-testid="dashboard-session"
                        onclick={() => onNavigate(buildPath({ name: 'session', id: session.id }))}
                      >
                        <span class="min-w-0 flex-1 truncate">{session.title || session.id}</span>
                        <span class="shrink-0 text-xs text-muted-foreground">{session.status === 'working' ? 'Running' : 'Needs you'}</span>
                      </Button>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
            {#if group.runs.length}
              <div>
                <h3 class="mb-1 px-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Runs</h3>
                <ul class="flex flex-col gap-1">
                  {#each group.runs as run (run.id)}
                    <li>
                      <Button
                        variant="ghost"
                        class="h-auto min-h-11 w-full justify-start text-left"
                        data-testid="dashboard-run"
                        onclick={() => onNavigate(buildPath({ name: 'project-run', projectId: group.projectId, runId: run.id }))}
                      >
                        <span class="min-w-0 flex-1 truncate">{run.spec || run.id}</span>
                        <span class="shrink-0 text-xs text-muted-foreground">{run.status === 'running' ? 'Running' : 'Needs you'}</span>
                      </Button>
                    </li>
                  {/each}
                </ul>
              </div>
            {/if}
          </section>
        {/each}
      </div>
    {:else}
      <p class="rounded-2xl border px-4 py-8 text-center text-sm text-muted-foreground">Nothing needs attention. Running and needs-you work will appear here.</p>
    {/if}
  </div>
</div>
