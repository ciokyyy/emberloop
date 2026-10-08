<!-- New-Session dialog: harness, model (filtered), working directory. -->
<script>
  import { HARNESSES, modelsForHarness, DEFAULT_WORK_DIR } from './harnesses.js';
  import Button from './ui/button.svelte';

  let { open = false, busy = false, error = '', onClose = () => {}, onCreate = () => {} } = $props();
  let harness = $state(HARNESSES[0].id);
  let model = $state(modelsForHarness(HARNESSES[0].id)[0] ?? '');
  let workDir = $state(DEFAULT_WORK_DIR);

  const models = $derived(modelsForHarness(harness));

  function pickHarness(id) {
    harness = id;
    model = modelsForHarness(id)[0] ?? '';
  }

  function submit(ev) {
    ev.preventDefault();
    if (!harness || !model.trim() || !workDir.trim()) return;
    onCreate({ harness, model: model.trim(), workDir: workDir.trim() });
  }
</script>

{#if open}
  <div role="presentation" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onclick={(e) => e.target === e.currentTarget && onClose()}>
    <div role="dialog" aria-modal="true" aria-label="New Session" class="w-full max-w-md rounded-2xl border bg-card p-6 shadow-xl">
      <h2 class="text-lg font-semibold">New Session</h2>
      <p class="mb-4 text-sm text-muted-foreground">Pick what runs and where.</p>
      <form onsubmit={submit} class="flex flex-col gap-4">
        <label class="flex flex-col gap-1 text-sm">
          <span class="font-medium">Harness</span>
          <select aria-label="harness" bind:value={harness} onchange={(e) => pickHarness(e.currentTarget.value)} class="rounded-lg border bg-background px-3 py-2">
            {#each HARNESSES as h (h.id)}
              <option value={h.id}>{h.label}</option>
            {/each}
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm">
          <span class="font-medium">Model</span>
          <select aria-label="model" bind:value={model} class="rounded-lg border bg-background px-3 py-2">
            {#each models as m (m)}
              <option value={m}>{m}</option>
            {/each}
          </select>
        </label>
        <label class="flex flex-col gap-1 text-sm">
          <span class="font-medium">Working directory</span>
          <input aria-label="working directory" bind:value={workDir} placeholder={DEFAULT_WORK_DIR} class="rounded-lg border bg-background px-3 py-2 font-mono" />
        </label>
        {#if error}
          <p role="alert" class="text-sm text-red-600 dark:text-red-400">{error}</p>
        {/if}
        <div class="flex justify-end gap-2">
          <Button type="button" variant="ghost" onclick={onClose}>Cancel</Button>
          <Button type="submit" disabled={busy || !model.trim() || !workDir.trim()}>
            {busy ? 'Creating…' : 'Create Session'}
          </Button>
        </div>
      </form>
    </div>
  </div>
{/if}
