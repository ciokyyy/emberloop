<!-- Phone drawer area switcher: one pinned row of five tabs (icon over label)
     instead of five stacked rows, so the contextual list above keeps the
     height. Each tab is a 44px-plus target in the thumb zone. -->
<script>
  import { areas } from './areas.js';

  let { view = 'chat', onSelect = () => {} } = $props();
</script>

<nav class="grid shrink-0 grid-cols-5 gap-0.5 border-t pt-2" aria-label="Areas" data-testid="drawer-nav">
  {#each areas as it (it.id)}
    {@const Icon = it.icon}
    <button
      type="button"
      aria-current={view === it.id ? 'page' : undefined}
      onclick={() => onSelect(it.id)}
      class="flex min-h-14 flex-col items-center justify-center gap-1 rounded-xl px-0 text-[11px] font-medium leading-none max-[360px]:text-[10px] transition-colors duration-200 ease-[var(--ease-smooth)] focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none active:scale-[0.97] {view === it.id ? 'bg-accent text-foreground' : 'text-muted-foreground hover:text-foreground'}"
    >
      <Icon class="size-5" />
      <span class="max-w-full truncate">{it.short}</span>
    </button>
  {/each}
</nav>
