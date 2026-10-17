<!-- Page header, one pattern for every area: [menu on phone] [back] [icon]
     title, scope chip, extra chips, hint, then actions pushed right. The
     title is the page's one h1 (headline on desktop, title size on phone,
     like a native navigation bar). A Back button that shows on phone replaces the
     menu button there (drill-down pages); `backOn` is `always`, `phone` or
     `desktop`. `size="compact"` is for the chat header,
     where the conversation leads. The phone menu button comes from the shell
     context (App) unless `onMenu` overrides it; without either it is absent. -->
<script>
  import { getContext } from 'svelte';
  import { ArrowLeft, Menu } from '@lucide/svelte';
  import IconButton from './icon-button.svelte';
  import Chip from './chip.svelte';
  import InfoHint from './info-hint.svelte';
  import { cn } from './cn.js';
  import { SHELL_KEY } from '../shell.js';

  let {
    title,
    icon: Icon = null,
    scope = '',
    scopeIcon = null,
    hint = '',
    size = 'page',
    onBack = null,
    backLabel = 'Back',
    onMenu = undefined,
    menuLabel = undefined,
    subtitle = '',
    titleAttrs = {},
    scopeAttrs = {},
    backOn = 'always',
    meta = null,
    actions = null,
    class: className = '',
    ...rest
  } = $props();

  const shell = getContext(SHELL_KEY);
  const phoneBack = $derived(Boolean(onBack) && backOn !== 'desktop');
  const openMenu = $derived(phoneBack ? null : onMenu === undefined ? shell?.openMenu : onMenu);
  const menuName = $derived(menuLabel ?? shell?.menuLabel ?? 'Menu');
</script>

<header class={cn('flex min-h-11 min-w-0 items-center gap-2 md:gap-3', className)} data-page-header {...rest}>
  {#if openMenu}
    <IconButton icon={Menu} label={menuName} size="icon-sm" class="-ml-2 md:hidden" onclick={openMenu} />
  {/if}
  {#if onBack}
    <IconButton icon={ArrowLeft} label={backLabel} size="icon-sm" class={cn('-ml-2', backOn === 'phone' && 'md:hidden', backOn === 'desktop' && 'max-md:hidden')} onclick={onBack} />
  {/if}
  {#if Icon && size === 'page'}
    <span class="grid size-10 shrink-0 place-items-center rounded-xl bg-card text-fg-2 shadow-elev-1 max-md:hidden" aria-hidden="true"><Icon class="size-5" /></span>
  {/if}
  <div class="flex min-w-0 flex-col" {...titleAttrs}>
    <h1 class={cn('min-w-0 truncate text-fg-1', size === 'compact' ? 'text-title' : 'text-headline max-md:text-title')}>{title}</h1>
    {#if subtitle}<span class="truncate text-caption text-fg-3 max-sm:hidden">{subtitle}</span>{/if}
  </div>
  {#if scope}
    <Chip tone="outline" icon={scopeIcon} class="shrink-[2] max-sm:max-w-[45%]" data-scope-chip {...scopeAttrs}>{scope}</Chip>
  {/if}
  {@render meta?.()}
  {#if hint}<InfoHint label={`About ${title.toLowerCase()}`}>{#if typeof hint === 'string'}{hint}{:else}{@render hint()}{/if}</InfoHint>{/if}
  {#if actions}<div class="ml-auto flex shrink-0 items-center gap-1">{@render actions()}</div>{/if}
</header>
