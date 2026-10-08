<!-- Runs view: pick a repo, pick a spec, preview its tickets. Plain markup. -->
<script>
  import { onMount } from 'svelte';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import { listSpecs, previewSpec } from './api.js';

  let { workDir = '' } = $props();

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

  async function load() {
    error = '';
    message = '';
    specs = [];
    selected = '';
    tickets = [];
    previewTitle = '';
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
    <h3 class="text-sm font-semibold tracking-tight">{previewTitle || selected}</h3>
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
  {/if}
</div>
