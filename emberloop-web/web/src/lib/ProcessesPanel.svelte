<!-- Processes of the active Session: the commands the background tool (or this
     panel) runs outside the turn, with their output tail and a Stop. Wide it is
     a side panel; under lg it floats as an overlay card; under md it goes full
     bleed with safe-area insets, one step (the list, then a picked process with
     a Back button). Live through the Session's background stream. -->
<script>
  import { ArrowLeft, Play, Square } from '@lucide/svelte';
  import IconButton from './ui/icon-button.svelte';
  import Button from './ui/button.svelte';
  import EmptyState from './ui/empty-state.svelte';
  import ErrorState from './ui/error-state.svelte';
  import { icons } from './icons.js';
  import { formatAge, oneLine, tailOf } from './processes.js';

  let { processes = [], error = '', starting = false, onStart = async () => {}, onStop = () => {}, onClose = () => {} } = $props();

  let picked = $state(null);
  let command = $state('');

  const pickedProcess = $derived(processes.find((p) => p.id === picked) ?? null);

  async function start() {
    const text = oneLine(command);
    if (!text || starting) return;
    command = '';
    await onStart(text);
  }

  function open(p) {
    picked = p.id;
  }
</script>

<aside aria-label="Processes" class="animate-in fade-in-0 slide-in-from-right-4 flex h-full w-96 shrink-0 flex-col gap-3 overflow-hidden rounded-2xl border bg-card p-4 text-body shadow-elev-2 motion-emphasis max-lg:fixed max-lg:inset-x-2 max-lg:bottom-2 max-lg:top-14 max-lg:z-40 max-lg:h-auto max-lg:w-auto max-lg:shadow-elev-3 max-md:inset-x-0 max-md:bottom-0 max-md:top-0 max-md:z-50 max-md:rounded-none max-md:border-0 max-md:pb-[max(1rem,env(safe-area-inset-bottom))] max-md:pt-[max(1rem,env(safe-area-inset-top))]">
  <div class="flex min-h-11 shrink-0 items-center gap-2">
    <icons.process class="size-4 shrink-0 text-fg-3" aria-hidden="true" />
    <h2 class="min-w-0 flex-1 truncate text-title text-fg-1">Processes</h2>
    <span class="shrink-0 text-caption tabular-nums text-fg-3">{processes.length}</span>
    <IconButton icon={icons.dismiss} label="Close processes" class="-mr-2" onclick={onClose} />
  </div>

  <form class="flex shrink-0 items-center gap-2" onsubmit={(e) => { e.preventDefault(); start(); }}>
    <label class="sr-only" for="process-command">Command to run in the background</label>
    <input
      id="process-command"
      class="min-h-11 min-w-0 flex-1 rounded-lg border bg-background px-3 font-mono text-caption text-fg-1 outline-none placeholder:text-fg-3 focus-visible:ring-[3px] focus-visible:ring-ring/40 lg:min-h-9"
      placeholder=".emberloop/preview"
      bind:value={command}
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
    />
    <Button type="submit" variant="secondary" icon={Play} disabled={!oneLine(command) || starting}>{starting ? 'Starting' : 'Run'}</Button>
  </form>

  {#if error}
    <ErrorState compact title={error} />
  {/if}

  {#if processes.length === 0}
    <EmptyState compact icon={icons.process} title="Nothing running" description="Commands the agent starts in the background, a dev server or a watcher, run here until the Session ends." />
  {:else if pickedProcess}
    <div class="flex min-h-0 flex-1 flex-col rounded-xl border bg-background/50">
      <div class="flex shrink-0 items-center gap-1 border-b py-1 pl-1 pr-3 text-caption text-fg-3">
        <IconButton icon={ArrowLeft} label="Back to processes" onclick={() => (picked = null)} />
        <span class="ml-1 min-w-0 flex-1 truncate font-mono" title={pickedProcess.command}>{pickedProcess.command}</span>
        {#if pickedProcess.running}
          <IconButton icon={Square} label={`Stop ${pickedProcess.command}`} onclick={() => onStop(pickedProcess.id)} />
        {/if}
      </div>
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="min-h-0 flex-1 overflow-auto overscroll-contain p-2 font-mono text-caption leading-snug" tabindex="0" role="region" aria-label={`Output of ${pickedProcess.command}`} data-testid="process-output">
        {#if pickedProcess.output}
          {#each tailOf(pickedProcess.output).split('\n') as line}<div class="whitespace-pre-wrap break-all px-1 text-fg-2">{line || ' '}</div>{/each}
        {:else}
          <p class="px-1 text-fg-3">No output yet.</p>
        {/if}
      </div>
      {#if !pickedProcess.running}
        <p class="shrink-0 border-t px-3 py-1 text-caption text-fg-3" data-testid="process-exit">{pickedProcess.exit || 'exited'}</p>
      {/if}
    </div>
  {:else}
    <ul class="flex min-h-0 flex-1 flex-col gap-1 overflow-auto overscroll-contain" data-testid="process-list">
      {#each processes as p (p.id)}
        <li class="flex items-center gap-2 rounded-lg hover:bg-surface-3/60">
          <button type="button" class="motion-standard flex min-h-11 min-w-0 flex-1 flex-col justify-center rounded-lg px-2 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 lg:min-h-9" onclick={() => open(p)} title={p.command}>
            <span class="flex min-w-0 items-center gap-2">
              <span class="size-2 shrink-0 rounded-full {p.running ? 'bg-status-working' : 'bg-fg-3'}" aria-hidden="true"></span>
              <span class="min-w-0 flex-1 truncate font-mono text-caption text-fg-1">{p.command}</span>
              <span class="shrink-0 text-caption tabular-nums text-fg-3">{p.running ? formatAge(p.startedAt, Date.now()) : p.exit || 'exited'}</span>
            </span>
          </button>
          {#if p.running}
            <IconButton icon={Square} label={`Stop ${p.command}`} onclick={() => onStop(p.id)} />
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</aside>
