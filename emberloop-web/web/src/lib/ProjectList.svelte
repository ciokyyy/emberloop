<!-- Project sections of one User group: a collapsible header per Project (a "+"
     starts a draft chat there), then that Project's Session rows. -->
<script>
  import { ChevronRight, Plus, Trash2 } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import { icons } from './icons.js';
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
        class="motion-standard group flex items-center gap-1 rounded-xl px-1 hover:bg-surface-3/50"
        class:bg-surface-3={!!sec.key && sec.key === activeProjectId && !currentId}
      >
        <button
          class="flex min-h-10 min-w-0 flex-1 items-center gap-2 rounded-lg px-1.5 text-left text-body focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none max-md:min-h-11"
          onclick={() => toggle(id)}
          aria-expanded={open}
          title={sec.path}
        >
          <ChevronRight class="motion-standard size-4 shrink-0 text-fg-3 {open ? 'rotate-90' : ''}" aria-hidden="true" />
          <icons.project class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
          <span class="min-w-0 flex-1 truncate font-medium text-fg-1">{sec.name}</span>
          {#if !open && sec.roots.length}<span class="text-caption tabular-nums text-fg-3">{sec.roots.length}</span>{/if}
        </button>
        {#if sec.key}
          <!-- Rename and Remove are hover and keyboard actions on desktop; phone
               has them in Project settings, so no hover-only control remains. -->
          <span class="flex items-center opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 max-md:hidden">
            <IconButton icon={icons.edit} label="Rename {sec.name}" class="size-7" onclick={() => onRename(project)} />
            <IconButton icon={Trash2} label="Remove {sec.name}" class="size-7" onclick={() => onRemove(project)} />
          </span>
          <IconButton icon={Plus} label="New chat in {sec.name}" class="size-7 max-md:size-11" onclick={() => onNew(sec.key)} />
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
