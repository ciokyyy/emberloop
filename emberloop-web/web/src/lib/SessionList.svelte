<!-- Session tree: newest activity first, Subagents nested under parents. -->
<script>
  let { sessions = [], currentId = null, onSelect = () => {} } = $props();

  const STATUS_DOT = {
    idle: 'bg-zinc-400',
    working: 'bg-blue-500 animate-pulse',
    failed: 'bg-red-500',
    ended: 'bg-zinc-600',
  };

  function buildTree(list) {
    const byId = new Map(list.map((s) => [s.id, s]));
    const kids = new Map();
    const roots = [];
    for (const s of list) {
      const parent = s.parentId;
      if (parent && byId.has(parent)) {
        if (!kids.has(parent)) kids.set(parent, []);
        kids.get(parent).push(s);
      } else {
        roots.push(s);
      }
    }
    return { roots, kids };
  }
  const tree = $derived(buildTree(sessions));
  const short = (id) => String(id ?? '').slice(0, 8);
</script>

{#snippet row(s)}
  <button
    onclick={() => onSelect(s.id)}
    aria-current={s.id === currentId}
    class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left text-sm hover:bg-muted"
    class:bg-muted={s.id === currentId}
  >
    <span class="h-2 w-2 shrink-0 rounded-full {STATUS_DOT[s.status] ?? STATUS_DOT.idle}" title={s.status}></span>
    <span class="min-w-0 flex-1 truncate font-medium">{s.harness} · {short(s.id)}{#if s.scout}<span class="ml-1 rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">scout</span>{/if}</span>
    <span class="shrink-0 text-xs text-muted-foreground">{s.status}</span>
  </button>
{/snippet}

<nav aria-label="Sessions" class="flex flex-col gap-0.5">
  {#each tree.roots as s (s.id)}
    {@render row(s)}
    {#each tree.kids.get(s.id) ?? [] as kid (kid.id)}
      <div class="ml-4 border-l border-border pl-1">
        {@render row(kid)}
      </div>
    {/each}
  {:else}
    <p class="px-3 py-4 text-sm text-muted-foreground">No Sessions yet. Create one to start.</p>
  {/each}
</nav>
