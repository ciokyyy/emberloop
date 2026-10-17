<!-- Page section: title row (icon, title, hint as text or snippet, actions), one-line
     description, then content. `collapsible` makes the title a disclosure
     toggle (a native <details>) for secondary detail. -->
<script>
  import { ChevronDown } from '@lucide/svelte';
  import InfoHint from './info-hint.svelte';
  import { cn } from './cn.js';

  let {
    title,
    description = '',
    icon: Icon = null,
    hint = null,
    actions = null,
    collapsible = false,
    open = $bindable(true),
    headingLevel = 2,
    class: className = '',
    children,
    ...rest
  } = $props();
  const heading = $derived(`h${Math.min(6, Math.max(1, headingLevel))}`);
</script>

{#snippet head()}
  {#if Icon}<Icon class="size-4 shrink-0 text-fg-3" aria-hidden="true" />{/if}
  <svelte:element this={heading} class="min-w-0 truncate text-title text-fg-1">{title}</svelte:element>
  {#if hint}
    <!-- Inside a <summary> the hint must not toggle the disclosure. -->
    <span class="contents" role="presentation" onclick={(event) => { if (collapsible) event.preventDefault(); }}>
      <InfoHint label={`About ${title}`}>{#if typeof hint === 'string'}{hint}{:else}{@render hint()}{/if}</InfoHint>
    </span>
  {/if}
{/snippet}

{#if collapsible}
  <details class={cn('group flex flex-col gap-3', className)} bind:open data-section {...rest}>
    <summary class="flex min-h-9 cursor-pointer list-none items-center gap-2 rounded-lg outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 [&::-webkit-details-marker]:hidden">
      {@render head()}
      <ChevronDown class="motion-standard ml-auto size-4 shrink-0 text-fg-3 group-open:rotate-180" aria-hidden="true" />
    </summary>
    {#if description}<p class="-mt-1 text-caption text-fg-3">{description}</p>{/if}
    {@render children?.()}
  </details>
{:else}
  <section class={cn('flex flex-col gap-3', className)} data-section {...rest}>
    <header class="flex min-h-9 items-center gap-2 max-md:min-h-11">
      {@render head()}
      {#if actions}<div class="ml-auto flex shrink-0 items-center gap-1">{@render actions()}</div>{/if}
    </header>
    {#if description}<p class="-mt-2 text-caption text-fg-3">{description}</p>{/if}
    {@render children?.()}
  </section>
{/if}
