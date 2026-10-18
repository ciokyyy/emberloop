<!-- Project scope switcher: one compact trigger (icon, name, chevron) that opens
     a popover with a search box and the scope list. Always searchable; All
     projects and No project are stable routes beside the recorded Projects. -->
<script>
  import { ChevronsUpDown } from '@lucide/svelte';
  import Popover from './ui/popover.svelte';
  import ProjectScopeList from './ProjectScopeList.svelte';
  import { icons } from './icons.js';
  import { scopeItems, scopePath, scopeValue } from './projectScope.js';

  let { projects = [], selectedId = null, onNavigate = () => {} } = $props();

  const current = $derived(scopeItems(projects, selectedId).find((item) => item.value === scopeValue(selectedId)));
  let open = $state(false);

  function selectScope(next) {
    open = false;
    onNavigate(scopePath(next));
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
    <ProjectScopeList {projects} {selectedId} onSelect={selectScope} />
  </Popover>
</div>
