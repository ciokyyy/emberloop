<!-- Projects in the sidebar: each Project groups its Sessions. A "+" on a
     Project starts a draft chat in it, so the working directory is set by
     where you start, not typed per Session. -->
<script>
  import { Folder, FolderOpen, ChevronRight, Plus, Trash2, Pencil } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import SessionList from './SessionList.svelte';

  let {
    projects = [],
    sessions = [],
    currentId = null,
    activeProjectId = null,
    onSelect = () => {},
    onNew = () => {},
    onRename = () => {},
    onRemove = () => {},
  } = $props();

  const KEY = 'eb.collapsed';
  let collapsed = $state(new Set(JSON.parse(localStorage.getItem(KEY) ?? '[]')));
  function toggle(id) {
    const next = new Set(collapsed);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    collapsed = next;
    localStorage.setItem(KEY, JSON.stringify([...next]));
  }

  const known = $derived(new Set(projects.map((p) => p.id)));
  const byProject = $derived.by(() => {
    const m = new Map();
    for (const s of sessions) {
      const key = known.has(s.projectId) ? s.projectId : '';
      if (!m.has(key)) m.set(key, []);
      m.get(key).push(s);
    }
    return m;
  });
  const orphans = $derived(byProject.get('') ?? []);
</script>

<div class="flex flex-col gap-1">
  {#each projects as p (p.id)}
    {@const open = !collapsed.has(p.id)}
    {@const list = byProject.get(p.id) ?? []}
    <section aria-label={p.name}>
      <div
        class="group flex items-center gap-1 rounded-xl px-1 py-1 text-sm transition-colors hover:bg-accent/50"
        class:bg-accent={p.id === activeProjectId && !currentId}
      >
        <button
          class="flex min-w-0 flex-1 items-center gap-1.5 rounded-lg px-1.5 py-1 text-left focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
          onclick={() => toggle(p.id)}
          aria-expanded={open}
          title={p.path}
        >
          <ChevronRight class="size-3.5 shrink-0 text-muted-foreground transition-transform duration-200 {open ? 'rotate-90' : ''}" />
          {#if open}<FolderOpen class="size-4 shrink-0 text-muted-foreground" />{:else}<Folder class="size-4 shrink-0 text-muted-foreground" />{/if}
          <span class="min-w-0 flex-1 truncate font-medium">{p.name}</span>
          {#if !open && list.length}<span class="text-xs text-muted-foreground">{list.length}</span>{/if}
        </button>
        <span class="flex items-center opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
          <Button variant="ghost" size="icon-sm" class="size-6" aria-label="Rename {p.name}" onclick={() => onRename(p)}><Pencil class="size-3" /></Button>
          <Button variant="ghost" size="icon-sm" class="size-6" aria-label="Remove {p.name}" onclick={() => onRemove(p)}><Trash2 class="size-3" /></Button>
        </span>
        <Button variant="ghost" size="icon-sm" class="size-6" aria-label="New chat in {p.name}" onclick={() => onNew(p.id)}><Plus class="size-3.5" /></Button>
      </div>
      {#if open}
        <div class="ml-3 border-l border-border pl-1">
          <SessionList sessions={list} {currentId} {onSelect} empty="" />
        </div>
      {/if}
    </section>
  {:else}
    <p class="px-3 py-4 text-sm text-muted-foreground">Add a project to start. Chats started in a project run in its folder.</p>
  {/each}
  {#if orphans.length}
    <section aria-label="Other">
      <p class="px-3 pt-2 pb-1 text-xs font-medium text-muted-foreground">Other</p>
      <SessionList sessions={orphans} {currentId} {onSelect} empty="" />
    </section>
  {/if}
</div>
