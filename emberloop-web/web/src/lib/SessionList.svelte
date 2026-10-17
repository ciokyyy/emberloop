<!-- Rows of one Project section: root rows (Runs and root Sessions) with their
     Subagents and Run Workers nested behind a chevron. Every row is the shared
     Row: status glyph, title (Run and scout chips), one line of meta and an
     unread dot. A needs-you row gets the amber tint. -->
<script>
  import { ChevronRight } from '@lucide/svelte';
  import Row from './ui/row.svelte';
  import Chip from './ui/chip.svelte';
  import { groupAllDone, sessionLabel, readJSON, writeJSON, isCollapsed, togglePref } from './sidebar.js';
  import { dotState } from './owners.js';

  let { roots = [], kids = new Map(), currentId = null, onSelect = () => {} } = $props();

  // dotState -> shared status glyph; `idle` is the quiet hollow circle.
  const STATUS = { needs: 'needs-you', capped: 'paused', working: 'working', failed: 'failed', ended: 'done', idle: 'idle' };
  const WORD = { needs: 'Needs you', capped: 'Capped', working: 'Working', failed: 'Failed', ended: 'Ended', idle: 'Idle' };
  const RUN_WORD = { paused: 'Paused', done: 'Done' };

  function metaOf(s, subs = 0) {
    const state = dotState(s);
    const word = s.kind === 'run' && state !== 'needs' && RUN_WORD[s.status] ? RUN_WORD[s.status] : WORD[state];
    return [word, s.harness, subs ? `${subs} sub` : ''].filter(Boolean).join(' · ');
  }

  const SUB_KEY = 'eb.subagents-collapsed';
  let subPrefs = $state(readJSON(SUB_KEY, {}));
  function toggleSubs(id, list) {
    subPrefs = togglePref(subPrefs, id, groupAllDone(list));
    writeJSON(SUB_KEY, subPrefs);
  }
</script>

{#snippet row(s, subs = 0)}
  {@const state = dotState(s)}
  {#snippet chips()}
    {#if s.kind === 'run'}<Chip tone="outline" class="shrink-0">Run</Chip>{/if}
    {#if s.scout}<Chip class="shrink-0">scout</Chip>{/if}
  {/snippet}
  <Row
    title={sessionLabel(s)}
    meta={metaOf(s, subs)}
    status={STATUS[state]}
    statusWord={false}
    attention={state === 'needs'}
    current={s.id === currentId}
    unread={Boolean(s.unread || s.reportUnread)}
    badges={chips}
    onclick={() => onSelect(s)}
    data-session-id={s.id}
    class="flex-1"
  />
{/snippet}

{#snippet parent(s)}
  {@const list = kids.get(s.id) ?? []}
  {@const collapsed = isCollapsed(subPrefs, s.id, groupAllDone(list))}
  <div class="flex items-center gap-0.5">
    <button
      type="button"
      onclick={() => toggleSubs(s.id, list)}
      aria-expanded={!collapsed}
      aria-label="{collapsed ? 'Expand' : 'Collapse'} subagents of {sessionLabel(s)}"
      class="motion-standard grid size-8 shrink-0 place-items-center rounded-lg text-fg-3 hover:text-fg-1 focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none max-md:size-11"
    >
      <ChevronRight class="motion-standard size-4 {collapsed ? '' : 'rotate-90'}" aria-hidden="true" />
    </button>
    {@render row(s, list.length)}
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
