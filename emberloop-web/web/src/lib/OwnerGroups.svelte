<!-- Session tree grouped by User. Scoped views receive one Project from the parent
     switcher and render its rows directly; the unscoped helper mode retains
     Project sections for its standalone grouping tests. -->
<script>
  import { ChevronRight } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Select from './ui/select.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import { icons } from './icons.js';
  import UserAvatar from './UserAvatar.svelte';
  import ProjectList from './ProjectList.svelte';
  import SessionList from './SessionList.svelte';
  import { buildOwnerGroups, FILTERS, projectIdForPath } from './owners.js';
  import { readJSON, writeJSON, isCollapsed, togglePref } from './sidebar.js';

  let {
    users = [],
    me = null,
    sessions = [],
    runs = [],
    projects = [],
    currentId = null,
    activeProjectId = null,
    query = '',
    scoped = false,
    scopeProjectId = '',
    onSelect = () => {},
    onNew = () => {},
    onRename = () => {},
    onRemove = () => {},
  } = $props();

  const FILTER_KEY = 'eb.owner-filter';
  const PROJECT_KEY = 'eb.owner-project';
  const GROUP_KEY = 'eb.owners-collapsed';
  const storedFilter = readJSON(FILTER_KEY, 'mine');
  let filter = $state(FILTERS.includes(storedFilter) ? storedFilter : 'mine');
  let pickedProject = $state(readJSON(PROJECT_KEY, ''));
  let groupPrefs = $state(readJSON(GROUP_KEY, {}));

  const projectId = $derived.by(() => {
    const known = (id) => projects.some((p) => p.id === id);
    if (known(pickedProject)) return pickedProject;
    if (known(activeProjectId)) return activeProjectId;
    return projects[0]?.id ?? '';
  });
  const scopedProjects = $derived(scoped ? projects.filter((p) => p.id === scopeProjectId) : projects);
  const scopedSessions = $derived(scoped ? sessions.filter((s) => (s.projectId ?? '') === scopeProjectId) : sessions);
  const scopedRuns = $derived(scoped ? runs.filter((r) => (r.projectId || projectIdForPath(projects, r.repo)) === scopeProjectId) : runs);
  const visibleFilter = $derived(scoped ? (filter === 'all' ? 'all' : 'mine') : filter);
  const q = $derived(query.trim());
  const view = $derived(buildOwnerGroups({ users, me, sessions: scopedSessions, runs: scopedRuns, projects: scopedProjects, filter: visibleFilter, projectId, query }));
  const rootsFor = (group) => group.sections.flatMap((section) => section.roots);
  const projectItems = $derived(projects.map((p) => ({ value: p.id, label: p.name })));

  function setFilter(f) {
    filter = f;
    writeJSON(FILTER_KEY, f);
  }
  function pickProject(id) {
    pickedProject = id;
    writeJSON(PROJECT_KEY, id);
    setFilter('project');
  }
  function toggleGroup(g) {
    groupPrefs = togglePref(groupPrefs, g.id, !g.own);
    writeJSON(GROUP_KEY, groupPrefs);
  }
</script>

<div class="flex min-h-0 flex-col gap-2">
  <div class="flex flex-wrap items-center gap-1.5 px-1" role="group" aria-label="Show sessions">
    <Button variant={filter === 'mine' ? 'secondary' : 'outline'} size="sm" class="max-md:min-w-16" aria-pressed={filter === 'mine'} onclick={() => setFilter('mine')}>Mine</Button>
    <Button variant={filter === 'all' ? 'secondary' : 'outline'} size="sm" class="max-md:min-w-16" aria-pressed={filter === 'all'} onclick={() => setFilter('all')}>Everyone</Button>
    {#if !scoped}
      <Select
        class="h-8 w-auto min-w-0 rounded-full px-2.5 text-caption max-md:h-11 max-md:text-body {filter === 'project' ? 'bg-surface-3' : ''}"
        items={projectItems}
        value={projectId}
        onValueChange={pickProject}
        placeholder="Project"
        disabled={!projects.length}
        aria-label="Project filter"
      />
    {/if}
  </div>

  <div class="flex min-h-0 flex-1 flex-col gap-1 overflow-auto">
    {#if scoped && !scopedSessions.length && !scopedRuns.length}
      <EmptyState icon={icons.session} title="No Sessions here yet" description="Start one with New chat." class="mx-1 px-4 py-6" />
    {:else}
      {#each view.groups as g (g.id)}
        {@const open = !!q || !isCollapsed(groupPrefs, g.id, !g.own)}
        <section aria-label={g.name} data-owner={g.id} class="animate-rise">
          <button
            type="button"
            class="motion-standard flex min-h-10 w-full items-center gap-2 rounded-xl px-2 text-left text-body hover:bg-surface-3/50 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none max-md:min-h-11"
            onclick={() => toggleGroup(g)}
            aria-expanded={open}
          >
            <ChevronRight class="motion-standard size-4 shrink-0 text-fg-3 {open ? 'rotate-90' : ''}" aria-hidden="true" />
            <UserAvatar name={g.name} tone={g.tone} online={g.online} />
            <span class="min-w-0 flex-1 truncate font-medium text-fg-1">
              {g.name}{#if g.own}<span class="ml-1 text-caption font-normal text-fg-3">you</span>{/if}
            </span>
            <span class="text-caption tabular-nums text-fg-3" data-testid="owner-count">{g.count}</span>
          </button>
          {#if open}
            <div class="ml-3 border-l-2 pl-1" style="border-color: {g.tone}">
              {#if scoped}
                <SessionList roots={rootsFor(g)} kids={view.kids} {currentId} onSelect={onSelect} />
              {:else}
                <ProjectList
                  ownerId={g.id}
                  sections={g.sections}
                  kids={view.kids}
                  {currentId}
                  {activeProjectId}
                  force={!!q}
                  {onSelect}
                  {onNew}
                  {onRename}
                  {onRemove}
                />
              {/if}
            </div>
          {/if}
        </section>
      {:else}
        <p class="px-3 py-4 text-caption text-fg-3">
          {q ? `No chats match “${q}”.` : 'No chats yet. Start one with New chat.'}
        </p>
      {/each}
    {/if}
  </div>
</div>
