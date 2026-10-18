<!-- Project scope list: one searchable list of scopes (All projects, No
     project, the recorded Projects, and a missing current one). The switcher
     popover and the phone Project sheet both render it, so the scopes and
     their order stay in one place. -->
<script>
  import { Command } from 'bits-ui';
  import { Check, Search } from '@lucide/svelte';
  import EmptyState from './ui/empty-state.svelte';
  import { icons } from './icons.js';
  import { scopeItems, scopeValue } from './projectScope.js';

  let { projects = [], selectedId = null, onSelect = () => {}, inputId = undefined } = $props();

  const items = $derived(scopeItems(projects, selectedId));
  const value = $derived(scopeValue(selectedId));
</script>

<Command.Root label="Project scope" class="flex min-h-0 flex-col">
  <div class="flex items-center gap-2 border-b border-border px-3">
    <Search class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
    <Command.Input
      id={inputId}
      placeholder="Search projects"
      aria-label="Search projects"
      class="h-11 w-full bg-transparent text-body text-fg-1 outline-none placeholder:text-fg-3 md:h-10"
    />
  </div>
  <Command.List class="max-h-64 overflow-y-auto overscroll-contain p-1.5">
    <Command.Empty class="p-1.5"><EmptyState compact icon={icons.project} title="No projects match" /></Command.Empty>
    {#each items as item (item.value)}
      <Command.Item
        value={item.value}
        keywords={[item.label]}
        onSelect={() => onSelect(item.value)}
        class="motion-standard flex min-h-11 cursor-default items-center gap-2 rounded-lg px-3 text-body outline-none select-none data-[selected]:bg-surface-3 md:min-h-9"
      >
        <span class="min-w-0 flex-1 truncate">{item.label}</span>
        {#if item.value === value}<Check class="size-4 shrink-0 text-fg-2" aria-hidden="true" />{/if}
      </Command.Item>
    {/each}
  </Command.List>
</Command.Root>
