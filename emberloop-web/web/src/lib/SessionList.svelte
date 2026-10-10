<!-- Session tree: attention tiers (Needs you / Working / Recent) with
     collapsible section headers, Subagents nested under their parent behind a
     chevron. Rows stay quiet by default; only needs-you rows get the loud
     amber treatment. Tiering reads only the status the session list already
     carries (see lib/sidebar.js). -->
<script>
  import { ChevronRight } from '@lucide/svelte';
  import Badge from './ui/badge.svelte';
  import {
    TIER_ORDER,
    TIER_LABEL,
    tierOf,
    rootTiers,
    buildTree,
    groupAllDone,
    sessionLabel,
    loadIdSet,
    storeIdSet,
    readJSON,
    writeJSON,
    isCollapsed,
    togglePref,
  } from './sidebar.js';

  let { sessions = [], currentId = null, onSelect = () => {}, empty = 'No Sessions yet. Create one to start.' } = $props();

  const STATUS_DOT = {
    idle: 'bg-zinc-400',
    working: 'bg-blue-500 animate-pulse',
    failed: 'bg-red-500',
    ended: 'bg-zinc-600',
  };

  const TIER_KEY = 'eb.tiers-collapsed';
  let tiersCollapsed = $state(loadIdSet(TIER_KEY));
  function toggleTier(tier) {
    const next = new Set(tiersCollapsed);
    if (next.has(tier)) next.delete(tier);
    else next.add(tier);
    tiersCollapsed = next;
    storeIdSet(TIER_KEY, next);
  }

  const SUB_KEY = 'eb.subagents-collapsed';
  let subPrefs = $state(readJSON(SUB_KEY, {}));
  function toggleSubs(id, kids) {
    subPrefs = togglePref(subPrefs, id, groupAllDone(kids));
    writeJSON(SUB_KEY, subPrefs);
  }

  const tree = $derived(buildTree(sessions));
  // Tiers hold tree roots only: a Subagent renders nested under its parent
  // once, never again as a loose row in its own tier.
  const tiers = $derived(rootTiers(sessions));
  // One populated tier needs no headers: the list reads as before.
  const sectioned = $derived(TIER_ORDER.filter((t) => tiers[t].length > 0).length > 1);
</script>

{#snippet row(s)}
  {@const loud = tierOf(s) === 'needs'}
  <button
    onclick={() => onSelect(s.id)}
    aria-current={s.id === currentId}
    title={s.id}
    class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition-colors hover:bg-accent/60 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none {s.id === currentId ? 'bg-accent' : ''} {loud ? 'border border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/15' : ''}"
  >
    <span class="size-2 shrink-0 rounded-full {STATUS_DOT[s.status] ?? STATUS_DOT.idle}" title={s.status}></span>
    <span class="min-w-0 flex-1 truncate font-medium">{sessionLabel(s)}{#if s.scout}<Badge class="ml-1 px-1.5 py-0 text-[10px]">scout</Badge>{/if}</span>
    {#if s.status === 'failed'}<span class="shrink-0 text-xs font-medium text-red-400">failed</span>
    {:else if s.status === 'ended'}<span class="shrink-0 text-xs text-muted-foreground">ended</span>{/if}
  </button>
{/snippet}

{#snippet parent(s)}
  {@const kids = tree.kids.get(s.id) ?? []}
  {@const collapsed = isCollapsed(subPrefs, s.id, groupAllDone(kids))}
  {@const loud = tierOf(s) === 'needs'}
  <div
    class="flex items-center gap-0.5 rounded-xl transition-colors hover:bg-accent/60 {s.id === currentId ? 'bg-accent' : ''} {loud ? 'border border-amber-500/30 bg-amber-500/10 hover:bg-amber-500/15' : ''}"
  >
    <button
      type="button"
      onclick={() => toggleSubs(s.id, kids)}
      aria-expanded={!collapsed}
      aria-label="{collapsed ? 'Expand' : 'Collapse'} subagents of {sessionLabel(s)}"
      class="grid size-7 shrink-0 place-items-center rounded-lg text-muted-foreground transition-colors hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
    >
      <ChevronRight class="size-3.5 transition-transform duration-200 ease-[var(--ease-smooth)] {collapsed ? '' : 'rotate-90'}" />
    </button>
    <button
      onclick={() => onSelect(s.id)}
      aria-current={s.id === currentId}
      title={s.id}
      class="flex min-w-0 flex-1 items-center gap-2 rounded-lg py-2 pr-3 text-left text-sm focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
    >
      <span class="size-2 shrink-0 rounded-full {STATUS_DOT[s.status] ?? STATUS_DOT.idle}" title={s.status}></span>
      <span class="min-w-0 flex-1 truncate font-medium">{sessionLabel(s)}{#if s.scout}<Badge class="ml-1 px-1.5 py-0 text-[10px]">scout</Badge>{/if}</span>
      <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{kids.length} sub</span>
      {#if s.status === 'failed'}<span class="shrink-0 text-xs font-medium text-red-400">failed</span>
      {:else if s.status === 'ended'}<span class="shrink-0 text-xs text-muted-foreground">ended</span>{/if}
    </button>
  </div>
  {#if !collapsed}
    <div class="ml-4 border-l border-border pl-1">
      {#each kids as kid (kid.id)}
        {@render row(kid)}
      {/each}
    </div>
  {/if}
{/snippet}

<nav aria-label="Sessions" class="flex flex-col gap-0.5">
  {#if !sessions.length}
    {#if empty}<p class="px-3 py-4 text-sm text-muted-foreground">{empty}</p>{/if}
  {:else if !sectioned}
    {#each tree.roots as s (s.id)}
      {#if (tree.kids.get(s.id) ?? []).length}{@render parent(s)}
      {:else}{@render row(s)}{/if}
    {/each}
  {:else}
    {#each TIER_ORDER as tier (tier)}
      {#if tiers[tier].length}
        {@const open = !tiersCollapsed.has(tier)}
        <section aria-label={TIER_LABEL[tier]} class={tier === 'needs' ? 'animate-rise' : ''}>
          <button
            type="button"
            onclick={() => toggleTier(tier)}
            aria-expanded={open}
            class="flex w-full items-center gap-1.5 rounded-xl px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none {tier === 'needs' ? 'text-amber-200/80 hover:text-amber-200' : ''}"
          >
            <ChevronRight class="size-3.5 transition-transform duration-200 ease-[var(--ease-smooth)] {open ? 'rotate-90' : ''}" />
            <span>{TIER_LABEL[tier]}</span>
            <span class="tabular-nums">{tiers[tier].length}</span>
            {#if tier === 'working'}<span class="size-1.5 animate-pulse rounded-full bg-blue-500"></span>{/if}
          </button>
          {#if open}
            <div class="flex flex-col gap-0.5">
              {#each tiers[tier] as s (s.id)}
                {#if (tree.kids.get(s.id) ?? []).length}{@render parent(s)}
                {:else}{@render row(s)}{/if}
              {/each}
            </div>
          {/if}
        </section>
      {/if}
    {/each}
  {/if}
</nav>
