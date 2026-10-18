<!-- Rail 1: narrow icon rail switching the app area. The active icon gets a
     sliding pill so switching areas glides instead of snapping. The header
     holds the rail-2 collapse toggle (Ctrl/⌘+B, persisted). `attention` maps
     an area id to true when something there needs the User: a small
     needs-you dot sits on the icon. Every icon carries a tooltip. -->
<script>
  import { PanelLeftClose, PanelLeftOpen } from '@lucide/svelte';
  import Tooltip from './ui/tooltip.svelte';
  import { areas } from './areas.js';

  let { view = 'chat', collapsed = false, attention = {}, onSelect = () => {}, onToggleSidebar = () => {} } = $props();

  const items = areas;
  const index = $derived(Math.max(0, items.findIndex((i) => i.id === view)));
  const ToggleIcon = $derived(collapsed ? PanelLeftOpen : PanelLeftClose);
  const toggleLabel = $derived(collapsed ? 'Expand sidebar' : 'Collapse sidebar');
  const RING = 'focus-visible:ring-[3px] focus-visible:ring-ring/40 focus-visible:outline-none';
</script>

<nav class="relative flex w-14 shrink-0 flex-col items-center gap-1 border-r bg-sidebar py-3 max-md:hidden" aria-label="Areas" data-testid="rail">
  <Tooltip text="{toggleLabel} (Ctrl/⌘+B)" side="right">
    {#snippet trigger(props)}
      <button
        type="button"
        {...props}
        aria-label={toggleLabel}
        aria-keyshortcuts="Control+B Meta+B"
        aria-pressed={collapsed}
        onclick={onToggleSidebar}
        class="motion-standard relative grid size-11 shrink-0 place-items-center rounded-xl text-fg-3 hover:text-fg-1 {RING}"
      >
        <ToggleIcon class="size-5" aria-hidden="true" />
      </button>
    {/snippet}
  </Tooltip>
  <div class="my-1 h-px w-8 shrink-0 bg-border" aria-hidden="true"></div>
  <div class="relative flex flex-col items-center gap-1">
    <span
      class="motion-emphasis pointer-events-none absolute top-0 left-1/2 size-11 -translate-x-1/2 rounded-xl bg-surface-3 transition-transform"
      style="transform: translateY({index * 3}rem);"
      aria-hidden="true"
    ></span>
    {#each items as it (it.id)}
      {@const Icon = it.icon}
      <Tooltip text={it.label} side="right">
        {#snippet trigger(props)}
          <button
            type="button"
            {...props}
            aria-label={attention[it.id] ? `${it.label}, needs you` : it.label}
            aria-current={view === it.id ? 'page' : undefined}
            onclick={() => onSelect(it.id)}
            class="motion-standard relative grid size-11 place-items-center rounded-xl hover:text-fg-1 {RING} {view === it.id ? 'text-fg-1' : 'text-fg-3'}"
          >
            <Icon class="size-5" aria-hidden="true" />
            {#if attention[it.id]}
              <span class="absolute top-2 right-2 size-2 rounded-full bg-status-needs-you ring-2 ring-sidebar" data-attention aria-hidden="true"></span>
            {/if}
          </button>
        {/snippet}
      </Tooltip>
    {/each}
  </div>
</nav>
