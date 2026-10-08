<!-- Runs view: pick a repo, pick a spec, preview its tickets, start a Run and watch its live Ticket table. -->
<script>
  import { onMount, onDestroy } from 'svelte';
  import Button from './ui/button.svelte';
  import Badge from './ui/badge.svelte';
  import Input from './ui/input.svelte';
  import { listSpecs, previewSpec, startRun, getRun, openRunStream } from './api.js';

  let { workDir = '', onOpenSession = null } = $props();

  let repo = $state(workDir);
  let specs = $state([]);
  let selected = $state('');
  let previewTitle = $state('');
  let tickets = $state([]);
  let message = $state('');
  let error = $state('');

  // A newly selected Session re-defaults the repo directory.
  $effect(() => {
    if (workDir) repo = workDir;
  });

  let run = $state(null);
  let runError = $state('');
  let starting = $state(false);
  let pollTimer = null;
  let runSource = null;

  function closeRunStream() {
    if (pollTimer) clearInterval(pollTimer);
    pollTimer = null;
    if (runSource) runSource.close();
    runSource = null;
  }

  function liveDone(r) {
    return r && r.status && r.status !== 'running';
  }

  async function refreshRun() {
    if (!run?.id) return;
    try {
      const body = await getRun(run.id);
      run = body.run ?? body;
      if (liveDone(run)) closeRunStream();
    } catch {
      // The table keeps the last state; the next poll retries.
    }
  }

  function subscribeRun(id) {
    closeRunStream();
    pollTimer = setInterval(refreshRun, 1000);
    if (typeof EventSource === 'undefined') return;
    try {
      runSource = openRunStream(id, (body) => {
        run = body.run ?? body;
        if (liveDone(run)) closeRunStream();
      });
    } catch {
      // Polling already covers the table when SSE is unavailable.
    }
  }

  onDestroy(closeRunStream);

  async function start() {
    if (!selected || starting) return;
    starting = true;
    runError = '';
    try {
      const body = await startRun({ spec: selected, repo });
      run = body.run ?? null;
      if (run?.id) subscribeRun(run.id);
    } catch (e) {
      runError = e?.message?.includes('409') ? 'A run is already active for this spec.' : 'Could not start the Run.';
    } finally {
      starting = false;
    }
  }

  async function load() {
    error = '';
    message = '';
    specs = [];
    selected = '';
    tickets = [];
    previewTitle = '';
    run = null;
    runError = '';
    closeRunStream();
    if (!repo) {
      message = 'Enter a repo directory to find specs.';
      return;
    }
    try {
      const body = await listSpecs(repo);
      specs = body.specs ?? [];
      if (!specs.length) message = 'No specs found under this directory.';
    } catch {
      error = 'Could not list specs.';
    }
  }

  async function select(spec) {
    selected = spec;
    tickets = [];
    previewTitle = '';
    error = '';
    run = null;
    runError = '';
    closeRunStream();
    try {
      const body = await previewSpec(spec);
      // A reselect during the fetch wins: ignore the stale answer.
      if (selected !== spec) return;
      previewTitle = body.title ?? '';
      tickets = body.tickets ?? [];
    } catch {
      if (selected !== spec) return;
      error = 'Could not load the ticket preview.';
    }
  }

  onMount(() => {
    if (repo) load();
  });
</script>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-4 px-4 py-6">
  <h2 class="text-lg font-semibold tracking-tight">Runs</h2>

  <form
    class="flex gap-2"
    onsubmit={(e) => {
      e.preventDefault();
      load();
    }}
  >
    <label class="flex min-w-0 flex-1 flex-col gap-1 text-sm">
      Repo directory
      <Input bind:value={repo} placeholder="/path/to/repo" />
    </label>
    <Button type="submit" class="self-end">Find specs</Button>
  </form>

  {#if error}
    <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
  {/if}
  {#if message}
    <p class="rounded-xl border px-4 py-2 text-sm text-muted-foreground">{message}</p>
  {/if}

  {#if specs.length}
    <ul class="flex flex-col gap-2">
      {#each specs as s (s.spec)}
        <li>
          <Button
            variant={s.spec === selected ? 'secondary' : 'outline'}
            class="w-full justify-start"
            onclick={() => select(s.spec)}
          >
            {s.title || s.spec} ({s.tickets})
          </Button>
        </li>
      {/each}
    </ul>
  {/if}

  {#if selected}
    <div class="flex items-center gap-2">
      <h3 class="text-sm font-semibold tracking-tight">{previewTitle || selected}</h3>
      <Button size="sm" class="ml-auto" disabled={starting || !tickets.length} onclick={start}>
        {starting ? 'Starting…' : run ? 'Restart Run' : 'Start Run'}
      </Button>
    </div>
    {#if runError}
      <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{runError}</p>
    {/if}
    {#if tickets.length}
      <table class="w-full text-left text-sm">
        <thead>
          <tr class="text-muted-foreground">
            <th class="py-1 pr-2 font-medium">No.</th>
            <th class="py-1 pr-2 font-medium">ID</th>
            <th class="py-1 pr-2 font-medium">Title</th>
            <th class="py-1 pr-2 font-medium">Blocked by</th>
            <th class="py-1 font-medium">Boxes</th>
          </tr>
        </thead>
        <tbody>
          {#each tickets as t (t.id)}
            <tr class="border-t">
              <td class="py-1 pr-2">{t.num}</td>
              <td class="py-1 pr-2">{t.id}</td>
              <td class="py-1 pr-2">{t.title}</td>
              <td class="py-1 pr-2">{(t.blockers ?? []).join(', ') || '—'}</td>
              <td class="py-1">{`${t.done ?? 0}/${t.total ?? 0}`}</td>
            </tr>
            {#if t.error}
              <tr>
                <td colspan="5" class="pb-2">
                  <p role="alert" class="rounded-xl bg-destructive/10 px-3 py-1 text-sm text-destructive">{t.error}</p>
                </td>
              </tr>
            {/if}
          {/each}
        </tbody>
      </table>
    {/if}
    {#if run}
      <div class="flex items-center gap-2">
        <h4 class="text-sm font-semibold tracking-tight">Live Tickets</h4>
        <Badge>{run.status ?? ''}</Badge>
      </div>
      {#if run.tickets?.length}
        <table class="w-full text-left text-sm" data-testid="live-tickets">
          <thead>
            <tr class="text-muted-foreground">
              <th class="py-1 pr-2 font-medium">ID</th>
              <th class="py-1 pr-2 font-medium">Title</th>
              <th class="py-1 pr-2 font-medium">State</th>
              <th class="py-1 pr-2 font-medium">Worker</th>
              <th class="py-1 pr-2 font-medium">Spec review</th>
              <th class="py-1 pr-2 font-medium">Standards</th>
              <th class="py-1 pr-2 font-medium">Tip</th>
              <th class="py-1 font-medium">Refusal</th>
            </tr>
          </thead>
          <tbody>
            {#each run.tickets as t (t.id)}
              <tr class="border-t">
                <td class="py-1 pr-2">{t.id}</td>
                <td class="py-1 pr-2">{t.title}</td>
                <td class="py-1 pr-2">{t.state}</td>
                <td class="py-1 pr-2">
                  {#if t.workerSession}
                    <Button variant="link" size="sm" class="h-auto p-0" onclick={() => onOpenSession?.(t.workerSession)}>
                      {t.workerSession.slice(0, 8)}
                    </Button>
                  {:else}
                    <span class="text-muted-foreground">—</span>
                  {/if}
                </td>
                <td class="py-1 pr-2">{t.verdicts?.['reviewer-spec']?.verdict ?? '—'}{t.verdicts?.['reviewer-spec'] ? (t.verdicts['reviewer-spec'].fresh ? '' : ' (stale)') : ''}</td>
                <td class="py-1 pr-2">{t.verdicts?.['reviewer-standards']?.verdict ?? '—'}{t.verdicts?.['reviewer-standards'] ? (t.verdicts['reviewer-standards'].fresh ? '' : ' (stale)') : ''}</td>
                <td class="py-1 pr-2">{t.tip ? t.tip.slice(0, 8) : '—'}</td>
                <td class="py-1">{t.lastRefusal || '—'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
    {/if}
  {/if}
</div>
