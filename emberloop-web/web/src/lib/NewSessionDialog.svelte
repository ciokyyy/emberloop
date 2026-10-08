<!-- New-Session dialog: harness, model (from the Daemon), working directory. -->
<script>
  import { HARNESSES, modelsForHarness, modelLabel, DEFAULT_WORK_DIR } from './harnesses.js';
  import { getModels } from './api.js';
  import Button from './ui/button.svelte';
  import Dialog from './ui/dialog.svelte';
  import Select from './ui/select.svelte';
  import Input from './ui/input.svelte';

  let { open = false, busy = false, error = '', onClose = () => {}, onCreate = () => {} } = $props();
  let harness = $state(HARNESSES[0].id);
  let models = $state(modelsForHarness(HARNESSES[0].id));
  let model = $state(models[0] ?? '');
  let workDir = $state(DEFAULT_WORK_DIR);

  const harnessItems = HARNESSES.map((h) => ({ value: h.id, label: h.label }));
  const modelItems = $derived(models.map((m) => ({ value: m, label: modelLabel(harness, m) })));

  // The Daemon owns the model catalogue: ask it, fall back to the static list.
  async function loadModels(id) {
    let list = modelsForHarness(id);
    try {
      const body = await getModels(id);
      if (body.models?.length) list = body.models;
    } catch {
      // Keep the static fallback.
    }
    if (id !== harness) return;
    models = list;
    if (!list.includes(model)) model = list[0] ?? '';
  }

  function pickHarness(id) {
    harness = id;
    models = modelsForHarness(id);
    model = models[0] ?? '';
    loadModels(id);
  }

  $effect(() => {
    if (open) loadModels(harness);
  });

  function submit(ev) {
    ev.preventDefault();
    if (!harness || !model.trim() || !workDir.trim()) return;
    onCreate({ harness, model: model.trim(), workDir: workDir.trim() });
  }
</script>

<Dialog open={open} onOpenChange={(v) => !v && onClose()} title="New Session" description="Pick what runs and where.">
  <form onsubmit={submit} class="flex flex-col gap-4">
    <div class="flex flex-col gap-1.5">
      <span id="harness-label" class="text-sm font-medium">Harness</span>
      <Select aria-labelledby="harness-label" value={harness} items={harnessItems} onValueChange={pickHarness} />
    </div>
    <div class="flex flex-col gap-1.5">
      <span id="model-label" class="text-sm font-medium">Model</span>
      <Select aria-labelledby="model-label" bind:value={model} items={modelItems} placeholder="Select a model" />
    </div>
    <div class="flex flex-col gap-1.5">
      <label for="workdir" class="text-sm font-medium">Working directory</label>
      <Input id="workdir" aria-label="working directory" bind:value={workDir} placeholder={DEFAULT_WORK_DIR} class="font-mono" />
    </div>
    {#if error}
      <p role="alert" class="text-sm text-destructive">{error}</p>
    {/if}
    <div class="mt-1 flex justify-end gap-2">
      <Button type="button" variant="ghost" onclick={onClose}>Cancel</Button>
      <Button type="submit" disabled={busy || !model.trim() || !workDir.trim()}>
        {busy ? 'Creating…' : 'Create Session'}
      </Button>
    </div>
  </form>
</Dialog>
