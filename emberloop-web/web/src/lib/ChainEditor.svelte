<!-- One role's Chain: a Primary entry and up to two fallbacks, each a harness,
     a model and an effort. Desktop edits entries inline in a grid. Phone shows
     entries as a list; a tap opens a sheet for that entry. -->
<script>
  import { onMount } from 'svelte';
  import { ArrowDown, ArrowUp, ChevronRight, ImageOff, Plus, Trash2 } from '@lucide/svelte';
  import BottomSheet from './ui/bottom-sheet.svelte';
  import Chip from './ui/chip.svelte';
  import Combobox from './ui/combobox.svelte';
  import IconButton from './ui/icon-button.svelte';
  import InfoHint from './ui/info-hint.svelte';

  let {
    role,
    entries,
    harnessItems,
    itemsFor,
    effortItemsFor,
    saving = false,
    lacksImages = () => false,
    onadd,
    onremove,
    onmove,
    onbackend,
    onmodel,
    oneffort,
  } = $props();

  const phoneQuery = '(max-width: 767px)';
  let phone = $state(typeof matchMedia === 'function' && matchMedia(phoneQuery).matches);
  let editing = $state(-1);

  onMount(() => {
    if (typeof matchMedia !== 'function') return;
    const query = matchMedia(phoneQuery);
    phone = query.matches;
    const change = (event) => (phone = event.matches);
    query.addEventListener?.('change', change);
    return () => query.removeEventListener?.('change', change);
  });

  const slot = (index) => (index === 0 ? 'Primary' : `Fallback ${index}`);
  const full = $derived(entries.length >= 3);
  const entry = $derived(editing >= 0 ? entries[editing] : null);
  const effortLabel = (effort) => (effort ? effort[0].toUpperCase() + effort.slice(1) : '');
  let sheetOpen = $state(false);

  function open(index) {
    editing = index;
    sheetOpen = true;
  }
  // The sheet follows an entry that moves, so later edits hit the same entry.
  function move(index, delta) {
    const destination = index + delta;
    if (destination < 0 || destination >= entries.length) return;
    if (editing === index) editing = destination;
    onmove(index, delta);
  }
  // A removed entry closes its sheet; the list shifts under the index.
  function remove(index) {
    sheetOpen = false;
    editing = -1;
    onremove(index);
  }
</script>

{#snippet fields(item, index)}
  <Combobox
    class="md:h-10"
    aria-label={`${role.label} ${slot(index)} harness`}
    value={item.backend}
    items={harnessItems}
    onValueChange={(value) => onbackend(index, value)}
  />
  <Combobox
    class="md:h-10"
    aria-label={`${role.label} ${slot(index)} model`}
    value={item.model}
    items={itemsFor(item.backend, item.model)}
    placeholder="Select a model"
    onValueChange={(value) => onmodel(index, value)}
  />
  <Combobox
    class="md:h-10"
    aria-label={`${role.label} ${slot(index)} effort`}
    value={item.effort ?? ''}
    items={effortItemsFor(item.backend, item.model)}
    onValueChange={(value) => oneffort(index, value)}
  />
{/snippet}

{#snippet actions(index)}
  <IconButton icon={ArrowUp} label={`Move ${role.label} ${slot(index)} up`} disabled={saving || index === 0} onclick={() => move(index, -1)} />
  <IconButton icon={ArrowDown} label={`Move ${role.label} ${slot(index)} down`} disabled={saving || index === entries.length - 1} onclick={() => move(index, 1)} />
  <IconButton icon={Trash2} label={`Remove ${role.label} ${slot(index)}`} disabled={saving} onclick={() => remove(index)} />
{/snippet}

<section class="flex flex-col gap-3 p-4" data-testid={`agent-chain-${role.id}`}>
  <div class="flex min-h-9 items-center gap-1">
    <h3 class="min-w-0 truncate text-body text-fg-1">{role.label}</h3>
    <InfoHint label={`About ${role.label}`} class="max-md:-my-2.5">{role.hint}</InfoHint>
    <IconButton
      icon={Plus}
      label={`Add ${role.label} entry`}
      variant="outline"
      class="ml-auto"
      onclick={onadd}
      disabled={saving || full}
    />
  </div>

  {#if !entries.length}
    <p class="text-caption text-fg-3">No entries configured.</p>
  {/if}

  {#each entries as item, index (index)}
    {#if phone}
      <button
        type="button"
        class="motion-standard flex min-h-14 w-full min-w-0 items-center gap-3 rounded-xl border bg-surface-0 px-3 py-2 text-left outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40"
        aria-label={`Edit ${role.label} ${slot(index)}`}
        data-testid={`chain-entry-${role.id}-${index + 1}`}
        onclick={() => open(index)}
      >
        <span class="flex min-w-0 flex-1 flex-col">
          <span class="text-caption text-fg-3">{slot(index)}</span>
          <span class="truncate text-body text-fg-1">{item.model || 'No model'}</span>
          <span class="truncate text-caption text-fg-3">{item.backend}</span>
        </span>
        {#if item.effort}<Chip tone="outline">{effortLabel(item.effort)}</Chip>{/if}
        {#if lacksImages(item)}<Chip tone="needs-you" icon={ImageOff}>No images</Chip>{/if}
        <ChevronRight class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
      </button>
    {:else}
      <div
        class="grid items-center gap-2 md:grid-cols-[minmax(4.5rem,0.5fr)_minmax(0,1fr)_minmax(0,1.3fr)_minmax(0,1fr)_auto]"
        data-testid={`chain-entry-${role.id}-${index + 1}`}
      >
        <span class="text-caption text-fg-3">{slot(index)}</span>
        {@render fields(item, index)}
        <div class="flex items-center justify-end">{@render actions(index)}</div>
      </div>
    {/if}
    {#if !phone && lacksImages(item)}
      <p class="flex items-start gap-2 rounded-xl bg-status-needs-you/10 px-3 py-2 text-caption text-status-needs-you" data-testid={`image-input-warning-${role.id}-${index}`}>
        <ImageOff class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
        This model does not accept image input. This advisory warning does not prevent saving the Chain.
      </p>
    {/if}
  {/each}
</section>

{#if phone}
  <BottomSheet bind:open={sheetOpen} title={entry ? `${role.label} · ${slot(editing)}` : role.label} description="Harness, model and effort for this Chain entry.">
    {#if entry}
      <div class="flex flex-col gap-3">
        <div class="flex flex-col gap-1.5">
          <span class="text-caption text-fg-3">Harness</span>
          <Combobox
            aria-label={`${role.label} ${slot(editing)} harness`}
            value={entry.backend}
            items={harnessItems}
            onValueChange={(value) => onbackend(editing, value)}
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <span class="text-caption text-fg-3">Model</span>
          <Combobox
            aria-label={`${role.label} ${slot(editing)} model`}
            value={entry.model}
            items={itemsFor(entry.backend, entry.model)}
            placeholder="Select a model"
            onValueChange={(value) => onmodel(editing, value)}
          />
        </div>
        <div class="flex flex-col gap-1.5">
          <span class="text-caption text-fg-3">Effort</span>
          <Combobox
            aria-label={`${role.label} ${slot(editing)} effort`}
            value={entry.effort ?? ''}
            items={effortItemsFor(entry.backend, entry.model)}
            onValueChange={(value) => oneffort(editing, value)}
          />
        </div>
        {#if lacksImages(entry)}
          <p class="flex items-start gap-2 rounded-xl bg-status-needs-you/10 px-3 py-2 text-caption text-status-needs-you" data-testid={`image-input-warning-${role.id}-${editing}`}>
            <ImageOff class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
            This model does not accept image input. This advisory warning does not prevent saving the Chain.
          </p>
        {/if}
      </div>
    {/if}
    {#snippet footer()}
      {#if entry}{@render actions(editing)}{/if}
    {/snippet}
  </BottomSheet>
{/if}
