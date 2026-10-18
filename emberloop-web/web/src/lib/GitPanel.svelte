<!-- Git view of the active Project: branch, changed files with diffs, recent commits.
     Wide: a side panel with the file list above the open diff. Under lg it floats as
     an overlay card in two steps (the file list, then the diff with a Back button);
     under md it goes full bleed with safe-area insets. The diff is read-only. -->
<script>
  import { ArrowDown, ArrowLeft, ArrowUp, FileDiff } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import Chip from './ui/chip.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import { icons } from './icons.js';
  import { getGitDiff } from './api.js';

  let { projectId = null, git = null, onClose = () => {}, onRefresh = () => {} } = $props();

  let picked = $state(null);
  let diff = $state('');
  let diffError = $state('');
  let loading = $state(false);

  const LETTER = { M: 'text-status-needs-you', A: 'text-status-done', D: 'text-status-failed', R: 'text-status-working', '?': 'text-fg-3', U: 'text-status-failed' };
  const code = (f) => (f.work !== ' ' ? f.work : f.index);
  const split = (path) => {
    const i = path.lastIndexOf('/');
    return i < 0 ? { name: path, dir: '' } : { name: path.slice(i + 1), dir: path.slice(0, i) };
  };

  async function pick(f) {
    picked = f;
    loading = true;
    diffError = '';
    try {
      const staged = f.work === ' ' && f.index !== ' ' && f.index !== '?';
      diff = (await getGitDiff(projectId, f.path, staged)).diff || '(no textual changes)';
    } catch {
      diff = '';
      diffError = 'Could not load the diff.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    projectId;
    picked = null;
    diff = '';
    diffError = '';
  });

  const lineClass = (l) =>
    l.startsWith('+') && !l.startsWith('+++') ? 'bg-status-done/10 text-status-done' : l.startsWith('-') && !l.startsWith('---') ? 'bg-status-failed/10 text-status-failed' : l.startsWith('@@') ? 'text-status-working' : 'text-fg-3';
</script>

<aside aria-label="Git" class="animate-in fade-in-0 slide-in-from-right-4 flex h-full w-96 shrink-0 flex-col gap-3 overflow-hidden rounded-2xl border bg-card p-4 text-body shadow-elev-2 motion-emphasis max-lg:fixed max-lg:inset-x-2 max-lg:bottom-2 max-lg:top-14 max-lg:z-40 max-lg:h-auto max-lg:w-auto max-lg:shadow-elev-3 max-md:inset-x-0 max-md:bottom-0 max-md:top-0 max-md:z-50 max-md:rounded-none max-md:border-0 max-md:pb-[max(1rem,env(safe-area-inset-bottom))] max-md:pt-[max(1rem,env(safe-area-inset-top))]">
  <div class="flex min-h-11 shrink-0 items-center gap-2">
    <icons.git class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
    <h2 class="min-w-0 flex-1 truncate text-title text-fg-1">{git?.isRepo ? git.branch : 'Git'}</h2>
    {#if git?.isRepo && (git.ahead || git.behind)}
      <span class="flex items-center gap-1" title={git.upstream}>
        {#if git.ahead}<Chip tone="outline" icon={ArrowUp} class="tabular-nums" aria-label={`${git.ahead} ahead`}>{git.ahead}</Chip>{/if}
        {#if git.behind}<Chip tone="outline" icon={ArrowDown} class="tabular-nums" aria-label={`${git.behind} behind`}>{git.behind}</Chip>{/if}
      </span>
    {/if}
    {#if git?.isRepo}<IconButton icon={icons.refresh} label="Refresh" onclick={onRefresh} />{/if}
    <IconButton icon={icons.dismiss} label="Close git" class="-mr-2" onclick={onClose} />
  </div>

  {#if !git}
    <div role="status" aria-label="Loading git status" class="flex flex-col gap-2">
      <Skeleton class="h-11 rounded-lg" />
      <Skeleton class="h-11 rounded-lg" />
      <Skeleton class="h-11 rounded-lg" />
    </div>
  {:else if !git.isRepo}
    <EmptyState icon={icons.git} title="Not a git repository" description="This folder has no git history." />
  {:else}
    <div class="flex min-h-0 flex-1 flex-col gap-3">
      <div class="flex min-h-0 flex-col {picked ? 'max-lg:hidden lg:max-h-[30%]' : 'lg:max-h-[55%]'} max-lg:flex-1">
        <h3 class="mb-1 shrink-0 text-caption font-medium text-fg-3">Changes <span class="tabular-nums">({git.files.length})</span></h3>
        {#if git.files.length === 0}
          <EmptyState compact icon={icons.git} title="Working tree clean" description="No changed files in this Project." />
        {:else}
          <ul class="flex min-h-0 flex-col gap-0.5 overflow-auto overscroll-contain">
            {#each git.files as f (f.path)}
              {@const parts = split(f.path)}
              <li>
                <button
                  type="button"
                  class="motion-standard flex min-h-11 w-full items-center gap-3 rounded-lg px-2 py-1 text-left outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-ring/40 lg:min-h-9 {picked?.path === f.path ? 'bg-surface-3' : ''}"
                  onclick={() => pick(f)}
                  title={f.path}
                >
                  <span class="w-4 shrink-0 text-center font-mono text-caption font-semibold {LETTER[code(f)] ?? ''}">{code(f)}</span>
                  <span class="flex min-w-0 flex-1 flex-col">
                    <span class="truncate font-mono text-caption text-fg-1">{parts.name}</span>
                    {#if parts.dir}<span class="truncate font-mono text-caption text-fg-3">{parts.dir}</span>{/if}
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      {#if picked}
        <div class="flex min-h-0 flex-1 flex-col rounded-xl border bg-background/50">
          <div class="flex shrink-0 items-center gap-1 border-b py-1 pl-1 pr-3 text-caption text-fg-3">
            <IconButton icon={ArrowLeft} label="Back to changes" class="lg:hidden" onclick={() => (picked = null)} />
            <FileDiff class="ml-2 size-3.5 shrink-0 lg:ml-2" aria-hidden="true" /><span class="truncate font-mono">{picked.path}</span>
          </div>
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <div class="min-h-0 flex-1 overflow-auto overscroll-contain p-2 font-mono text-caption leading-snug" tabindex="0" role="region" aria-label={`Diff of ${picked.path}`}>
            {#if loading}
              <div role="status" aria-label="Loading diff" class="flex flex-col gap-1.5"><Skeleton class="h-3" /><Skeleton class="h-3 w-3/4" /><Skeleton class="h-3 w-1/2" /></div>
            {:else if diffError}
              <ErrorState compact title={diffError} onRetry={() => pick(picked)} />
            {:else}
              {#each diff.split('\n') as l}<div class="whitespace-pre-wrap break-all px-1 {lineClass(l)}">{l || ' '}</div>{/each}
            {/if}
          </div>
        </div>
      {/if}

      <div class="flex min-h-0 flex-col {picked ? 'max-lg:hidden lg:max-h-[20%]' : 'max-lg:shrink-0 lg:flex-1'}">
        <h3 class="mb-1 shrink-0 text-caption font-medium text-fg-3">Recent commits</h3>
        <ul class="flex min-h-0 max-h-48 flex-col gap-1 overflow-auto overscroll-contain lg:max-h-none">
          {#each git.log as c (c.hash)}
            <li class="flex items-baseline gap-2 px-2">
              <span class="shrink-0 font-mono text-caption text-fg-3">{c.hash}</span>
              <span class="min-w-0 flex-1 truncate text-body" title="{c.author} · {c.date}">{c.subject}</span>
            </li>
          {/each}
        </ul>
      </div>
    </div>
  {/if}
</aside>
