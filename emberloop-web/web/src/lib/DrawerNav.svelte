<!-- Phone drawer area switcher: one pinned row of five tabs (icon over label)
     instead of five stacked rows, so the contextual list above keeps the
     height. Each tab is a 56 px target in the thumb zone; `attention` puts a
     needs-you dot on the area that needs the User. -->
<script>
  import { areas } from './areas.js';

  let { view = 'chat', attention = {}, onSelect = () => {} } = $props();
</script>

<nav class="grid shrink-0 grid-cols-5 gap-0.5 border-t pt-2" aria-label="Areas" data-testid="drawer-nav">
  {#each areas as it (it.id)}
    {@const Icon = it.icon}
    <button
      type="button"
      aria-current={view === it.id ? 'page' : undefined}
      aria-label={attention[it.id] ? `${it.short}, needs you` : undefined}
      onclick={() => onSelect(it.id)}
      class="motion-standard relative flex min-h-14 flex-col items-center justify-center gap-1 rounded-xl px-0 text-caption font-medium focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none active:scale-[0.97] {view === it.id ? 'bg-surface-3 text-fg-1' : 'text-fg-3 hover:text-fg-1'}"
    >
      <Icon class="size-5" aria-hidden="true" />
      <span class="max-w-full truncate">{it.short}</span>
      {#if attention[it.id]}
        <span class="absolute top-2 left-1/2 ml-2 size-2 rounded-full bg-status-needs-you ring-2 ring-sidebar" data-attention aria-hidden="true"></span>
      {/if}
    </button>
  {/each}
</nav>
