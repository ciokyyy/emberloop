<!-- Rows of one Project section: root rows (Runs and root Sessions) with their
     Subagents and Run Workers nested behind a chevron. Rows stay quiet; a
     needs-you row gets the amber treatment; a Run row carries the Run tag. -->
<script>
  import { ChevronRight } from '@lucide/svelte';
  import Badge from './ui/badge.svelte';
  import { groupAllDone, sessionLabel, readJSON, writeJSON, isCollapsed, togglePref } from './sidebar.js';
  import { dotState } from './owners.js';

  let { roots = [], kids = new Map(), currentId = null, onSelect = () => {} } = $props();

  const STATUS_DOT = {
    idle: 'bg-zinc-400',
    working: 'bg-blue-500 animate-pulse',
    failed: 'bg-red-500',
    capped: 'bg-amber-500',
    ended: 'bg-zinc-600',
    needs: 'bg-amber-500',
  };
  const LOUD = 'border border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/15';

  const SUB_KEY = 'eb.subagents-collapsed';
  let subPrefs = $state(readJSON(SUB_KEY, {}));
  function toggleSubs(id, list) {
    subPrefs = togglePref(subPrefs, id, groupAllDone(list));
    writeJSON(SUB_KEY, subPrefs);
  }
</script>

{#snippet text(s)}
  <span class="min-w-0 flex-1 truncate font-medium">
    {sessionLabel(s)}
    {#if s.kind === 'run'}<Badge variant="outline" class="ml-1 px-1.5 py-0 text-[10px]">Run</Badge>{/if}
    {#if s.scout}<Badge class="ml-1 px-1.5 py-0 text-[10px]">scout</Badge>{/if}
  </span>
  {#if s.capped}
    <span class="shrink-0 text-xs font-medium text-amber-400">capped</span>
  {:else if s.status === 'failed'}
    <span class="shrink-0 text-xs font-medium text-red-400">failed</span>
  {:else if s.status === 'ended' || s.status === 'done' || s.status === 'paused'}
    <span class="shrink-0 text-xs text-muted-foreground">{s.status}</span>
  {/if}
{/snippet}

{#snippet row(s)}
  {@const state = dotState(s)}
  <button
    type="button"
    onclick={() => onSelect(s)}
    aria-current={s.id === currentId}
    title={s.id}
    class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none {s.id === currentId ? 'bg-accent' : ''} {state === 'needs' ? LOUD : ''}"
  >
    <span class="size-2 shrink-0 rounded-full {STATUS_DOT[state]}" title={state}></span>
    {@render text(s)}
  </button>
{/snippet}

{#snippet parent(s)}
  {@const list = kids.get(s.id) ?? []}
  {@const collapsed = isCollapsed(subPrefs, s.id, groupAllDone(list))}
  {@const state = dotState(s)}
  <div class="flex items-center gap-0.5 rounded-xl transition-colors hover:bg-accent/60 {s.id === currentId ? 'bg-accent' : ''} {state === 'needs' ? LOUD : ''}">
    <button
      type="button"
      onclick={() => toggleSubs(s.id, list)}
      aria-expanded={!collapsed}
      aria-label="{collapsed ? 'Expand' : 'Collapse'} subagents of {sessionLabel(s)}"
      class="grid size-7 shrink-0 place-items-center rounded-lg text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
    >
      <ChevronRight class="size-3.5 transition-transform duration-200 ease-[var(--ease-smooth)] {collapsed ? '' : 'rotate-90'}" />
    </button>
    <button
      type="button"
      onclick={() => onSelect(s)}
      aria-current={s.id === currentId}
      title={s.id}
      class="flex min-w-0 flex-1 items-center gap-2 rounded-lg py-2 pr-3 text-left text-sm focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
    >
      <span class="size-2 shrink-0 rounded-full {STATUS_DOT[state]}" title={state}></span>
      {@render text(s)}
      <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{list.length} sub</span>
    </button>
  </div>
  {#if !collapsed}
    <div class="ml-4 border-l border-border pl-1">
      {#each list as kid (kid.id)}
        {@render row(kid)}
      {/each}
    </div>
  {/if}
{/snippet}

<div class="flex flex-col gap-0.5">
  {#each roots as s (s.id)}
    {#if (kids.get(s.id) ?? []).length}{@render parent(s)}
    {:else}{@render row(s)}{/if}
  {/each}
</div>
