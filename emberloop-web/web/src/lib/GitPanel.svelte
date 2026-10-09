<!-- Git view of the active Project: branch, changed files with diffs, recent commits. -->
<script>
  import { X, GitBranch, FileDiff, ArrowUp, ArrowDown } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import { getGitDiff } from './api.js';

  let { projectId = null, git = null, onClose = () => {}, onRefresh = () => {} } = $props();

  let picked = $state(null);
  let diff = $state('');
  let loading = $state(false);

  const LETTER = { M: 'text-amber-400', A: 'text-green-400', D: 'text-red-400', R: 'text-blue-400', '?': 'text-muted-foreground', U: 'text-red-400' };
  const code = (f) => (f.work !== ' ' ? f.work : f.index);

  async function pick(f) {
    picked = f;
    loading = true;
    try {
      const staged = f.work === ' ' && f.index !== ' ' && f.index !== '?';
      diff = (await getGitDiff(projectId, f.path, staged)).diff || '(no textual changes)';
    } catch {
      diff = 'Could not load the diff.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    projectId;
    picked = null;
    diff = '';
  });

  const lineClass = (l) =>
    l.startsWith('+') && !l.startsWith('+++') ? 'bg-green-500/10 text-green-300' : l.startsWith('-') && !l.startsWith('---') ? 'bg-red-500/10 text-red-300' : l.startsWith('@@') ? 'text-blue-300' : 'text-muted-foreground';
</script>

<aside aria-label="Git" class="animate-in fade-in-0 slide-in-from-right-4 duration-300 flex h-full w-96 shrink-0 max-lg:fixed max-lg:inset-x-2 max-lg:bottom-2 max-lg:top-14 max-lg:z-40 max-lg:h-auto max-lg:w-auto flex-col gap-3 overflow-hidden rounded-2xl border bg-card p-4 text-sm">
  <div class="flex items-center gap-2">
    <GitBranch class="size-4 text-muted-foreground" />
    <h2 class="min-w-0 flex-1 truncate font-semibold">{git?.isRepo ? git.branch : 'Git'}</h2>
    {#if git?.isRepo && (git.ahead || git.behind)}
      <span class="flex items-center gap-1.5 text-xs text-muted-foreground" title={git.upstream}>
        {#if git.ahead}<span class="flex items-center"><ArrowUp class="size-3" />{git.ahead}</span>{/if}
        {#if git.behind}<span class="flex items-center"><ArrowDown class="size-3" />{git.behind}</span>{/if}
      </span>
    {/if}
    <Button variant="ghost" size="icon-sm" onclick={onClose} aria-label="Close git"><X /></Button>
  </div>

  {#if !git}
    <p class="text-muted-foreground">Loading…</p>
  {:else if !git.isRepo}
    <p class="text-muted-foreground">This folder is not a git repository.</p>
  {:else}
    <div class="flex min-h-0 flex-1 flex-col gap-3">
      <div class="flex min-h-0 flex-col {picked ? 'max-h-[30%]' : 'max-h-[55%]'}">
        <h3 class="mb-1 shrink-0 text-xs font-medium text-muted-foreground">Changes ({git.files.length})</h3>
        {#if git.files.length === 0}
          <p class="text-muted-foreground">Working tree clean.</p>
        {:else}
          <ul class="flex min-h-0 flex-col gap-0.5 overflow-auto">
            {#each git.files as f (f.path)}
              <li>
                <button
                  class="flex w-full items-center gap-2 rounded-lg px-2 py-1 text-left hover:bg-accent/60 {picked?.path === f.path ? 'bg-accent' : ''}"
                  onclick={() => pick(f)}
                >
                  <span class="w-4 shrink-0 text-center font-mono text-xs font-semibold {LETTER[code(f)] ?? ''}">{code(f)}</span>
                  <span class="min-w-0 flex-1 truncate font-mono text-xs" title={f.path}>{f.path}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>

      {#if picked}
        <div class="flex min-h-0 flex-1 flex-col rounded-xl border bg-background/50">
          <div class="flex shrink-0 items-center gap-2 border-b px-3 py-1.5 text-xs text-muted-foreground"><FileDiff class="size-3.5" /><span class="truncate font-mono">{picked.path}</span></div>
          <pre class="min-h-0 flex-1 overflow-auto p-2 font-mono text-[11px] leading-snug">{#if loading}Loading…{:else}{#each diff.split('\n') as l}<div class="whitespace-pre-wrap break-all px-1 {lineClass(l)}">{l || ' '}</div>{/each}{/if}</pre>
        </div>
      {/if}

      <div class="flex min-h-0 flex-col {picked ? 'max-h-[20%]' : 'flex-1'}">
        <h3 class="mb-1 shrink-0 text-xs font-medium text-muted-foreground">Recent commits</h3>
        <ul class="flex min-h-0 flex-col gap-1 overflow-auto">
          {#each git.log as c (c.hash)}
            <li class="flex items-baseline gap-2 px-2">
              <span class="shrink-0 font-mono text-xs text-muted-foreground">{c.hash}</span>
              <span class="min-w-0 flex-1 truncate" title="{c.author} · {c.date}">{c.subject}</span>
            </li>
          {/each}
        </ul>
      </div>
    </div>
    <Button variant="ghost" size="sm" class="shrink-0" onclick={onRefresh}>Refresh</Button>
  {/if}
</aside>
