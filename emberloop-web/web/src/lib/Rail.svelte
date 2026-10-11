<!-- Rail 1: narrow icon rail switching the app area. The active icon gets a
     sliding pill so switching areas glides instead of snapping. The header
     holds the rail-2 collapse toggle (Ctrl/⌘+B, persisted). -->
<script>
  import { MessageSquare, Layers, Gauge, Brain, Settings, PanelLeftClose, PanelLeftOpen } from '@lucide/svelte';

  let { view = 'chat', collapsed = false, onSelect = () => {}, onToggleSidebar = () => {} } = $props();

  const items = [
    { id: 'chat', label: 'Chat', icon: MessageSquare },
    { id: 'runs', label: 'Specs and Runs', icon: Layers },
    { id: 'memory', label: 'Memory', icon: Brain },
    { id: 'quota', label: 'Quota', icon: Gauge },
    { id: 'settings', label: 'Settings', icon: Settings },
  ];
  const index = $derived(Math.max(0, items.findIndex((i) => i.id === view)));
  const ToggleIcon = $derived(collapsed ? PanelLeftOpen : PanelLeftClose);
</script>

<nav class="relative flex w-14 shrink-0 flex-col items-center gap-1 border-r bg-sidebar py-3 max-md:hidden" aria-label="Areas" data-testid="rail">
  <button
    type="button"
    title={collapsed ? 'Expand sidebar (Ctrl/⌘+B)' : 'Collapse sidebar (Ctrl/⌘+B)'}
    aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
    aria-pressed={collapsed}
    onclick={onToggleSidebar}
    class="relative grid size-10 shrink-0 place-items-center rounded-xl text-muted-foreground transition-colors duration-200 ease-[var(--ease-smooth)] hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none"
  >
    <ToggleIcon class="size-[18px]" />
  </button>
  <div class="my-1 h-px w-8 shrink-0 bg-border" aria-hidden="true"></div>
  <div class="relative flex flex-col items-center gap-1">
    <span
      class="pointer-events-none absolute top-0 left-1/2 size-10 -translate-x-1/2 rounded-xl bg-accent transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]"
      style="transform: translateY({index * 2.75}rem);"
      aria-hidden="true"
    ></span>
    {#each items as it (it.id)}
      {@const Icon = it.icon}
      <button
        type="button"
        title={it.label}
        aria-label={it.label}
        aria-current={view === it.id ? 'page' : undefined}
        onclick={() => onSelect(it.id)}
        class="relative grid size-10 place-items-center rounded-xl transition-colors duration-200 hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none {view === it.id ? 'text-foreground' : 'text-muted-foreground'}"
      >
        <Icon class="size-[18px]" />
      </button>
    {/each}
  </div>
</nav>
