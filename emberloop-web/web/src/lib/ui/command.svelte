<!-- shadcn-svelte Command on bits-ui: search input, list, empty state.
     Rows are virtualized with TanStack Virtual (fixed row height, overscan);
     filtering is manual (shouldFilter=false) so the virtualizer sees the
     filtered list. Items may carry a `detail` (pig: provider) rendered as a
     muted second line; those lists use taller rows. The search box hides
     when items are few. -->
<script>
  import { Command } from 'bits-ui';
  import { Check, Search } from '@lucide/svelte';
  import { createVirtualizer } from '@tanstack/svelte-virtual';
  import { cn } from './cn.js';
  import { untrack } from 'svelte';

  const SINGLE_HEIGHT = 36;
  const RICH_HEIGHT = 56;
  const OVERSCAN = 6;
  const SEARCH_THRESHOLD = 8;

  let {
    items = [],
    selectedValue = '',
    onSelect = () => {},
    label = '',
    searchPlaceholder = 'Search…',
    emptyText = 'No results found.',
    class: className = '',
  } = $props();

  let search = $state('');
  // Start on the selected item so opening highlights it and the scroll
  // effect below brings it into view. The list remounts on every open.
  let highlight = $state(selectedValue);
  let listEl = $state(null);
  let inputEl = $state(null);

  // Rich lists (rows with a detail line) use taller rows; the fixed height
  // keeps the virtualizer exact.
  const rowHeight = $derived(items.some((i) => i.detail) ? RICH_HEIGHT : SINGLE_HEIGHT);

  const showSearch = $derived(items.length > SEARCH_THRESHOLD);
  // Search matches the combined text: provider, model id and label.
  const filtered = $derived(
    search.trim()
      ? items.filter((i) =>
          `${i.label} ${i.value} ${i.detail ?? ''}`.toLowerCase().includes(search.trim().toLowerCase()),
        )
      : items,
  );

  const virtualizer = createVirtualizer({
    count: items.length,
    getScrollElement: () => listEl,
    estimateSize: () => rowHeight,
    overscan: OVERSCAN,
  });

  // The list mounts when the popover opens: (re)bind count, row height and
  // scroll element so the virtualizer measures after mount.
  $effect(() => {
    const count = filtered.length;
    const el = listEl;
    const height = rowHeight;
    untrack(() => {
      try {
        $virtualizer.setOptions({ count, estimateSize: () => height, getScrollElement: () => el });
      } catch {
        // Unmounted scroll element: the next run rebinds after mount.
      }
    });
  });

  // Arrow-key highlight follows into view across the virtual window.
  $effect(() => {
    const current = highlight;
    if (!current) return;
    const index = filtered.findIndex((i) => i.value === current);
    if (index < 0) return;
    untrack(() => {
      try {
        $virtualizer.scrollToIndex(index, { align: 'auto' });
      } catch {
        // Zero-size scroll element (jsdom): nothing to scroll.
      }
    });
  });

  // A fresh list landing (async Daemon models after open) brings the current
  // highlight into view. It never moves the highlight itself.
  $effect(() => {
    const el = listEl;
    const count = filtered.length;
    const current = highlight;
    if (!el || !current) return;
    untrack(() => {
      void count;
      const index = filtered.findIndex((i) => i.value === current);
      if (index < 0) return;
      try {
        $virtualizer.scrollToIndex(index, { align: 'auto' });
      } catch {
        // Zero-size scroll element (jsdom): nothing to scroll.
      }
    });
  });

  // Combobox focus lands in search.
  $effect(() => {
    inputEl?.focus();
  });

  // The virtualizer returns no rows before it measures (jsdom: zero-size
  // scroll element). Fall back to one viewport window, never the full list:
  // rendering all 500 rows defeats windowing and times the tests out.
  const virtualRows = $derived.by(() => {
    try {
      const rows = $virtualizer.getVirtualItems();
      if (rows.length > 0) return rows.map((r) => ({ index: r.index, start: r.start }));
    } catch {
      // Fall through to the windowed list below.
    }
    const viewport = listEl?.clientHeight > 0 ? listEl.clientHeight : 256;
    const windowed = Math.min(filtered.length, Math.ceil(viewport / Math.max(1, rowHeight)) + OVERSCAN * 2);
    return Array.from({ length: windowed }, (_, index) => ({ index, start: index * rowHeight }));
  });

  const totalSize = $derived.by(() => {
    try {
      const total = $virtualizer.getTotalSize();
      if (total > 0) return total;
    } catch {
      // Fall through to the estimate below.
    }
    return filtered.length * rowHeight;
  });
</script>

<Command.Root shouldFilter={false} bind:value={highlight} {label} class={cn(className)}>
  {#if showSearch}
    <div class="flex items-center gap-2 border-b border-border px-3">
      <Search class="size-4 shrink-0 text-muted-foreground" />
      <Command.Input
        bind:value={search}
        bind:ref={inputEl}
        placeholder={searchPlaceholder}
        aria-label="Search options"
        class="h-10 w-full bg-transparent text-sm outline-none placeholder:text-muted-foreground"
      />
    </div>
  {/if}
  <Command.List bind:ref={listEl} class="max-h-64 overflow-y-auto p-1.5">
    {#if filtered.length === 0}
      <Command.Empty class="px-3 py-6 text-center text-sm text-muted-foreground">{emptyText}</Command.Empty>
    {:else}
      <div style="position: relative; height: {totalSize}px; width: 100%;">
        {#each virtualRows as row (filtered[row.index]?.value ?? row.index)}
          {@const item = filtered[row.index]}
          {#if item}
            <Command.Item
              value={item.value}
              keywords={[item.label, item.value, item.detail ?? ''].filter(Boolean)}
              onSelect={() => onSelect(item.value)}
              class="absolute top-0 left-0 flex w-full cursor-default items-center gap-2 rounded-lg pr-8 pl-3 outline-none select-none motion-standard transition-colors data-[selected]:bg-accent"
              style="height: {rowHeight}px; transform: translateY({row.start}px);"
            >
              <span class="flex min-w-0 flex-1 flex-col justify-center leading-tight">
                <span class="truncate text-sm">{item.label}</span>
                {#if item.detail}
                  <span class="truncate text-xs text-muted-foreground">{item.detail}</span>
                {/if}
              </span>
              {#if item.value === selectedValue}
                <Check class="absolute right-2.5 size-4 shrink-0" />
              {/if}
            </Command.Item>
          {/if}
        {/each}
      </div>
    {/if}
  </Command.List>
</Command.Root>
