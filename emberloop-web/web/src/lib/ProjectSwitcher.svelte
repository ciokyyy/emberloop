<script>
  import Select from './ui/select.svelte';
  import { buildPath } from './router.js';

  let { projects = [], selectedId = null, onNavigate = () => {} } = $props();

  const ALL = '__all_projects__';
  const NONE = '__no_project__';
  const items = $derived([
    { value: ALL, label: 'All projects' },
    { value: NONE, label: 'No project' },
    ...projects.map((project) => ({ value: project.id, label: project.name })),
    ...(selectedId && selectedId !== 'none' && !projects.some((project) => project.id === selectedId)
      ? [{ value: selectedId, label: 'Project not found' }]
      : []),
  ]);
  const value = $derived(selectedId == null ? ALL : selectedId === '' || selectedId === 'none' ? NONE : selectedId);

  function selectScope(next) {
    if (next === ALL) onNavigate(buildPath({ name: 'home' }));
    else onNavigate(buildPath({ name: 'project', projectId: next === NONE ? 'none' : next }));
  }
</script>

<div class="flex flex-col gap-1" data-testid="project-switcher">
  <span class="px-2 text-[11px] font-medium uppercase tracking-wide text-muted-foreground">Project</span>
  <Select
    class="w-full"
    items={items}
    {value}
    onValueChange={selectScope}
    aria-label="Project scope"
  />
</div>
