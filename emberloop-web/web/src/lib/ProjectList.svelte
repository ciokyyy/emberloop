<!-- Project sections of one User group: a collapsible header per Project (a "+"
     starts a draft chat there), then that Project's Session rows. -->
<script>
  import { Folder, FolderOpen, ChevronRight, Plus, Trash2, Pencil } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import SessionList from './SessionList.svelte';
  import { isCollapsed, togglePref, readJSON, writeJSON } from './sidebar.js';

  let {
    ownerId = '',
    sections = [],
    kids = new Map(),
    currentId = null,
    activeProjectId = null,
    force = false,
    onSelect = () => {},
    onNew = () => {},
    onRename = () => {},
    onRemove = () => {},
  } = $props();

  const KEY = 'eb.projects-collapsed';
  let prefs = $state(readJSON(KEY, {}));
  const sectionId = (key) => `${ownerId}/${key}`;
  function toggle(id) {
    prefs = togglePref(prefs, id, false);
    writeJSON(KEY, prefs);
  }
</script>

<div class="flex flex-col gap-1">
  {#each sections as sec (sec.key)}
    {@const id = sectionId(sec.key)}
    {@const open = force || !isCollapsed(prefs, id, false)}
    {@const project = { id: sec.key, name: sec.name, path: sec.path }}
    <section aria-label={sec.name} class="animate-rise">
      <div
        class="group flex items-center gap-1 rounded-xl px-1 py-1 text-sm transition-colors hover:bg-accent/50"
        class:bg-accent={!!sec.key && sec.key === activeProjectId && !currentId}
      >
        <button
          class="flex min-w-0 flex-1 items-center gap-1.5 rounded-lg px-1.5 py-1 text-left focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
          onclick={() => toggle(id)}
          aria-expanded={open}
          title={sec.path}
        >
          <ChevronRight class="size-3.5 shrink-0 text-muted-foreground transition-transform duration-200 {open ? 'rotate-90' : ''}" />
          {#if open}<FolderOpen class="size-4 shrink-0 text-muted-foreground" />{:else}<Folder class="size-4 shrink-0 text-muted-foreground" />{/if}
          <span class="min-w-0 flex-1 truncate font-medium">{sec.name}</span>
          {#if !open && sec.roots.length}<span class="text-xs text-muted-foreground">{sec.roots.length}</span>{/if}
        </button>
        {#if sec.key}
          <span class="flex items-center opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
            <Button variant="ghost" size="icon-sm" class="size-6" aria-label="Rename {sec.name}" onclick={() => onRename(project)}><Pencil class="size-3" /></Button>
            <Button variant="ghost" size="icon-sm" class="size-6" aria-label="Remove {sec.name}" onclick={() => onRemove(project)}><Trash2 class="size-3" /></Button>
          </span>
          <Button variant="ghost" size="icon-sm" class="size-6" aria-label="New chat in {sec.name}" onclick={() => onNew(sec.key)}><Plus class="size-3.5" /></Button>
        {/if}
      </div>
      {#if open}
        <div class="ml-3 border-l border-border pl-1">
          <SessionList roots={sec.roots} {kids} {currentId} {onSelect} />
        </div>
      {/if}
    </section>
  {/each}
</div>
