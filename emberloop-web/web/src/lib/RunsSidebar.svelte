<!-- Rail 2 for Runs: spec discovery on top (repo directory, spec list),
     then every Run newest first. A spec pick repaints the main RunsView
     through the shared discovery store; the view owns the preview table,
     the Start button and the live Run. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import Button from './ui/button.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import { listRuns } from './api.js';
  import { watch } from './live.js';
  import {
    discovery,
    ensureDiscoveryContext,
    initDiscoveryRepo,
    loadDiscovery,
    selectDiscovery,
  } from './runsDiscovery.svelte.js';

  let { workDir = '', runId = '', ticketId = '', onNavigate = () => {} } = $props();

  let runs = $state([]);
  // False until the first list answers, so a cold load never reads "No Runs yet".
  let loaded = $state(false);
  let stop = null;

  async function load() {
    try {
      const body = await listRuns();
      runs = [...(body.runs ?? [])].sort((a, b) => String(b.updatedAt ?? '').localeCompare(String(a.updatedAt ?? '')));
    } catch {
      // Keep the last list; the next live change refetches.
    } finally {
      loaded = true;
    }
  }

  // A deep link seeds the picker from the list summary. The full Run and its
  // Ticket bodies are fetched by the main view only when that Run opens.
  let seededFor = '';
  $effect(() => {
    const id = runId;
    if (!id || seededFor === id) return;
    const summary = runs.find((run) => run.id === id);
    if (!summary) return;
    seededFor = id;
    void ensureDiscoveryContext(summary);
  });

  // A newly selected Session re-defaults the repo directory.
  $effect(() => {
    if (workDir) discovery.repo = workDir;
  });

  onMount(() => {
    initDiscoveryRepo(workDir);
    // A cold deep link waits for the list summary to seed the picker.
    if (!runId && discovery.repo && !discovery.specs.length && !discovery.selected) void loadDiscovery();
    load();
    stop = watch('runs', load);
  });
  onDestroy(() => stop?.());

  const needsYouRuns = $derived(runs.filter((run) => run.counts?.needsYou > 0));
  const needsYouCount = $derived(runs.reduce((count, run) => count + (run.counts?.needsYou ?? 0), 0));
  const dot = (status) => (status === 'done' ? 'bg-green-500' : status === 'failed' ? 'bg-red-500' : status === 'paused' ? 'bg-amber-500' : 'bg-blue-400 animate-pulse');
  const specLabel = (run) => String(run.spec ?? run.id).split('/').filter(Boolean).at(-2) || run.id;
  const merged = (run) => run.counts?.merged ?? 0;
  const total = (run) => run.counts?.total ?? 0;
</script>

<div class="flex min-h-0 flex-1 flex-col gap-3" data-testid="runs-sidebar">
  {#if needsYouCount}
    <div class="animate-rise flex flex-col gap-1 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-2" data-testid="needs-you-strip">
      <span class="px-2 pt-0.5 text-[11px] font-medium uppercase tracking-wide text-amber-300">Needs you ({needsYouCount})</span>
      {#each needsYouRuns as run (run.id)}
        <button
          type="button"
          class="truncate rounded-lg px-2 py-1 text-left text-sm transition-colors hover:bg-amber-500/15"
          onclick={() => onNavigate(`/runs/${encodeURIComponent(run.id)}`)}
        >{specLabel(run)} · {run.counts.needsYou} Ticket{run.counts.needsYou === 1 ? '' : 's'} need attention</button>
      {/each}
    </div>
  {/if}
  <div class="flex flex-col gap-2">
    <div class="flex items-center justify-between px-2">
      <span class="text-xs font-medium text-muted-foreground">Specs</span>
    </div>
    <form
      class="flex flex-col gap-2"
      onsubmit={(e) => {
        e.preventDefault();
        loadDiscovery();
      }}
    >
      <FloatingInput class="min-w-0 flex-1" label="Repo directory" bind:value={discovery.repo} placeholder="/path/to/repo" />
      <Button type="submit" size="sm" class="w-full">Find specs</Button>
    </form>
    {#if discovery.error}
      <p role="alert" class="rounded-xl bg-destructive/10 px-3 py-1.5 text-xs text-destructive">{discovery.error}</p>
    {/if}
    {#if discovery.message}
      <p class="rounded-xl border px-3 py-1.5 text-xs text-muted-foreground">{discovery.message}</p>
    {/if}
    {#if discovery.specs.length}
      <ul class="flex min-h-0 flex-col gap-0.5 overflow-auto">
        {#each discovery.specs as s (s.spec)}
          <li>
            <Button
              variant={s.spec === discovery.selected ? 'secondary' : 'ghost'}
              class="w-full justify-start truncate"
              onclick={() => selectDiscovery(s.spec)}
            >
              {s.title || s.spec} ({s.tickets})
            </Button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
  <div class="flex items-center justify-between px-2">
    <span class="text-xs font-medium text-muted-foreground">Runs</span>
    <button type="button" class="rounded-md px-1.5 py-0.5 text-xs text-muted-foreground transition-colors hover:text-foreground" onclick={() => onNavigate('/runs')}>New run</button>
  </div>
  <ul class="flex min-h-0 flex-1 flex-col gap-0.5 overflow-auto">
    {#each runs as r (r.id)}
      <li class="animate-rise">
        <button
          type="button"
          onclick={() => onNavigate(`/runs/${encodeURIComponent(r.id)}`)}
          class="flex w-full items-center gap-2 rounded-xl px-2.5 py-2 text-left text-sm transition-colors hover:bg-accent {r.id === runId ? 'bg-accent' : ''}"
        >
          <span class="size-2 shrink-0 rounded-full {dot(r.status)}"></span>
          <span class="min-w-0 flex-1 truncate">{specLabel(r)}</span>
          <span class="shrink-0 text-xs tabular-nums text-muted-foreground">{merged(r)}/{total(r)}</span>
        </button>
      </li>
    {:else}
      <li class="px-2.5 py-2 text-xs text-muted-foreground">{loaded ? 'No Runs yet.' : 'Loading Runs…'}</li>
    {/each}
  </ul>
</div>
