<!-- Settings page: harness, model and effort per role, grouped like Apple Settings. -->
<script>
  import { ArrowLeft, Check } from '@lucide/svelte';
  import { HARNESSES, modelsForHarness, modelLabel } from './harnesses.js';
  import { getModels, getSettings, putSettings } from './api.js';
  import Button from './ui/button.svelte';
  import Input from './ui/input.svelte';
  import Select from './ui/select.svelte';
  let { onBack = () => {} } = $props();

  const GROUPS = [
    {
      title: 'Orchestration',
      note: 'Who plans the work and who runs the spawned agents.',
      roles: [
        { id: 'orchestrator', label: 'Orchestrator', hint: 'Plans, delegates, and reviews results.' },
        { id: 'worker', label: 'Worker', hint: 'Subagent that edits code.' },
        { id: 'scout', label: 'Scout', hint: 'Read-only subagent that explores and reports.' },
      ],
    },
    {
      title: 'Build and review',
      note: 'Who writes the change and who checks it.',
      roles: [
        { id: 'implementer', label: 'Implementer', hint: 'Writes the change for a ticket.' },
        { id: 'code-review', label: 'Code review', hint: 'Reviews the diff for defects.' },
        { id: 'spec-review', label: 'Spec review', hint: 'Checks the change against the spec.' },
      ],
    },
  ];
  const ROLES = GROUPS.flatMap((g) => g.roles);
  const harnessItems = HARNESSES.filter((h) => h.id !== 'fake').map((h) => ({ value: h.id, label: h.label }));
  const effortItems = ['low', 'medium', 'high', 'max'].map((e) => ({ value: e, label: e[0].toUpperCase() + e.slice(1) }));

  let roles = $state({});
  let maxWorkers = $state('3');
  let locked = $state(false);
  let saved = $state('');
  let catalogue = $state({});
  let loading = $state(true);
  let saving = $state(false);
  let justSaved = $state(false);
  let error = $state('');

  const snapshot = () => JSON.stringify({ roles, maxWorkers });
  const dirty = $derived(snapshot() !== saved);

  async function models(harness) {
    let list = modelsForHarness(harness);
    try {
      const body = await getModels(harness);
      if (body.models?.length) list = body.models;
    } catch {
      // Static fallback.
    }
    catalogue[harness] = list;
  }

  async function load() {
    try {
      const body = await getSettings();
      const next = {};
      for (const r of ROLES) {
        next[r.id] = { harness: 'pig', model: '', effort: 'medium', ...(body.roles?.[r.id] ?? {}) };
      }
      await Promise.all(harnessItems.map((h) => models(h.value)));
      roles = next;
      maxWorkers = body.max_workers != null ? String(body.max_workers) : '3';
      locked = !!body.max_workers_from_env;
      saved = snapshot();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
  load();

  const itemsFor = (harness, current) => {
    const list = catalogue[harness] ?? [];
    const all = current && !list.includes(current) ? [current, ...list] : list;
    return all.map((m) => ({ value: m, label: modelLabel(harness, m) }));
  };

  function pickHarness(role, harness) {
    roles[role].harness = harness;
    roles[role].model = catalogue[harness]?.[0] ?? '';
  }

  async function save() {
    saving = true;
    error = '';
    try {
      // Locked means the field holds the env value, not the file value:
      // leave max_workers out so the save never writes the env cap
      // into the file.
      const payload = locked ? { roles } : { roles, max_workers: Number(maxWorkers) };
      const savedBody = await putSettings(payload);
      maxWorkers = savedBody.max_workers != null ? String(savedBody.max_workers) : maxWorkers;
      locked = !!savedBody.max_workers_from_env;
      saved = snapshot();
      justSaved = true;
      setTimeout(() => (justSaved = false), 1800);
    } catch (e) {
      error = e.message;
    } finally {
      saving = false;
    }
  }

  function revert() {
    const prev = JSON.parse(saved);
    roles = prev.roles;
    maxWorkers = prev.maxWorkers;
  }
</script>

<div class="relative flex min-h-0 flex-1 flex-col" data-testid="settings-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-32">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-8">
      <div class="flex flex-col gap-1">
        <Button variant="ghost" size="sm" class="-ml-3 w-fit text-muted-foreground" onclick={onBack}><ArrowLeft />Back</Button>
        <h1 class="text-3xl font-semibold tracking-tight">Settings</h1>
        <p class="text-sm text-muted-foreground">Choose the harness, model, and effort each role runs with. Changes apply to new agents.</p>
      </div>

      {#if error}
        <p role="alert" class="rounded-xl bg-destructive/10 px-4 py-2 text-sm text-destructive">{error}</p>
      {/if}

      {#if loading}
        <div class="flex flex-col gap-3">
          {#each [0, 1, 2] as i (i)}
            <div class="h-20 animate-pulse rounded-2xl bg-card"></div>
          {/each}
        </div>
      {:else}
        <section class="animate-rise flex flex-col gap-2">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Concurrency</h2>
            <p class="text-xs text-muted-foreground">How many Sessions run at once. Chat and Runs share the one cap.</p>
          </div>
          <div class="flex flex-col gap-2 rounded-2xl border bg-card p-4">
            <label class="flex flex-col gap-1 text-sm font-medium" for="max-workers">Maximum workers</label>
            <Input
              id="max-workers"
              aria-label="Maximum workers"
              type="number"
              min="1"
              step="1"
              class="h-10 max-w-40"
              bind:value={maxWorkers}
              disabled={locked}
            />
            {#if locked}
              <p class="text-xs text-muted-foreground">Set by the EMBERLOOP_MAX_WORKERS environment variable. Unset it to edit here.</p>
            {:else}
              <p class="text-xs text-muted-foreground">Applies immediately, no restart.</p>
            {/if}
          </div>
        </section>
        {#each GROUPS as g (g.title)}
          <section class="animate-rise flex flex-col gap-2">
            <div class="px-1">
              <h2 class="text-sm font-semibold">{g.title}</h2>
              <p class="text-xs text-muted-foreground">{g.note}</p>
            </div>
            <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
              {#each g.roles as r (r.id)}
                <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem_minmax(0,16rem)_7rem]">
                  <div class="min-w-0">
                    <div class="text-sm font-medium">{r.label}</div>
                    <div class="text-xs text-muted-foreground">{r.hint}</div>
                  </div>
                  <Select
                    class="h-10"
                    aria-label={`${r.label} harness`}
                    value={roles[r.id].harness}
                    items={harnessItems}
                    onValueChange={(v) => pickHarness(r.id, v)}
                  />
                  <Select
                    class="h-10"
                    aria-label={`${r.label} model`}
                    bind:value={roles[r.id].model}
                    items={itemsFor(roles[r.id].harness, roles[r.id].model)}
                    placeholder="Select a model"
                  />
                  <Select class="h-10" aria-label={`${r.label} effort`} bind:value={roles[r.id].effort} items={effortItems} />
                </div>
              {/each}
            </div>
          </section>
        {/each}
      {/if}
    </div>
  </div>

  {#if dirty || justSaved}
    <div class="pointer-events-none absolute inset-x-0 bottom-0 flex justify-center px-4 pb-5">
      <div class="animate-rise pointer-events-auto flex items-center gap-3 rounded-full border bg-popover/90 py-2 pr-2 pl-5 shadow-2xl backdrop-blur-xl">
        {#if justSaved && !dirty}
          <span class="flex items-center gap-1.5 text-sm text-green-300"><Check class="size-4" />Saved</span>
        {:else}
          <span class="text-sm text-muted-foreground">Unsaved changes</span>
          <Button variant="ghost" size="sm" onclick={revert} disabled={saving}>Revert</Button>
          <Button size="sm" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
        {/if}
      </div>
    </div>
  {/if}
</div>
