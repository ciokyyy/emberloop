<!-- Project scope switcher: one compact trigger (icon, name, chevron) that opens
     a popover with a search box and the scope list. Always searchable; All
     projects and No project are stable routes beside the recorded Projects. -->
<script>
  import { Command } from 'bits-ui';
  import { Check, ChevronsUpDown, Search } from '@lucide/svelte';
  import Popover from './ui/popover.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import { icons } from './icons.js';
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
  const current = $derived(items.find((item) => item.value === value));
  let open = $state(false);

  function selectScope(next) {
    open = false;
    if (next === ALL) onNavigate(buildPath({ name: 'home' }));
    else onNavigate(buildPath({ name: 'project', projectId: next === NONE ? 'none' : next }));
  }
</script>

<div data-testid="project-switcher">
  <Popover
    bind:open
    aria-label="Project scope"
    class="motion-standard flex min-h-10 w-full min-w-0 items-center gap-2 rounded-xl bg-surface-3/50 px-3 text-left text-body text-fg-1 outline-none hover:bg-surface-3 focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11"
    contentClass="w-(--bits-popover-anchor-width) min-w-56 shadow-elev-2"
  >
    {#snippet trigger()}
      <icons.project class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
      <span class="min-w-0 flex-1 truncate">{current?.label ?? 'All projects'}</span>
      <ChevronsUpDown class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
    {/snippet}
    <Command.Root label="Project scope" class="flex flex-col">
      <div class="flex items-center gap-2 border-b border-border px-3">
        <Search class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
        <Command.Input
          placeholder="Search projects"
          aria-label="Search projects"
          class="h-11 w-full bg-transparent text-body text-fg-1 outline-none placeholder:text-fg-3 md:h-10"
        />
      </div>
      <Command.List class="max-h-64 overflow-y-auto p-1.5">
        <Command.Empty class="p-1.5"><EmptyState compact icon={icons.project} title="No projects match" /></Command.Empty>
        {#each items as item (item.value)}
          <Command.Item
            value={item.value}
            keywords={[item.label]}
            onSelect={() => selectScope(item.value)}
            class="motion-standard flex min-h-11 cursor-default items-center gap-2 rounded-lg px-3 text-body outline-none select-none data-[selected]:bg-surface-3 md:min-h-9"
          >
            <span class="min-w-0 flex-1 truncate">{item.label}</span>
            {#if item.value === value}<Check class="size-4 shrink-0 text-fg-2" aria-hidden="true" />{/if}
          </Command.Item>
        {/each}
      </Command.List>
    </Command.Root>
  </Popover>
</div>
