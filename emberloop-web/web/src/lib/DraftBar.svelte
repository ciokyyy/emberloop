<!-- Draft chat header: the project is already chosen, pick harness and model
     for the chat the first message will start. The choice is remembered. -->
<script>
  import { HARNESSES, modelsForHarness, modelItems } from './harnesses.js';
  import { getModels } from './api.js';
  import Combobox from './ui/combobox.svelte';

  let { harness = $bindable('claude'), model = $bindable('sonnet'), effort = $bindable('default') } = $props();

  // Reasoning effort applies to claude and pig Sessions; 'default' keeps the model default.
  const effortItems = [
    { value: 'default', label: 'Default effort' },
    { value: 'low', label: 'Low effort' },
    { value: 'medium', label: 'Medium effort' },
    { value: 'high', label: 'High effort' },
    { value: 'xhigh', label: 'Extra high effort' },
    { value: 'max', label: 'Max effort' },
  ];

  let models = $state(modelsForHarness(harness));
  const harnessItems = $derived(HARNESSES.filter((h) => h.id !== 'fake' || harness === 'fake').map((h) => ({ value: h.id, label: h.label })));
  const modelOptions = $derived(modelItems(harness, models, model));

  async function loadModels(id) {
    let list = modelsForHarness(id);
    try {
      const body = await getModels(id, id === harness ? model : '');
      if (body.models?.length) list = body.models;
    } catch {
      // Keep the static fallback.
    }
    if (id !== harness) return;
    models = list;
    if (!model) model = list[0] ?? '';
  }

  function pickHarness(id) {
    harness = id;
    models = modelsForHarness(id);
    model = models[0] ?? '';
    loadModels(id);
  }

  $effect(() => {
    loadModels(harness);
  });
</script>

<div class="flex flex-wrap items-center gap-2">
  <div class="w-32 max-w-full"><Combobox aria-label="Harness" value={harness} items={harnessItems} onValueChange={pickHarness} /></div>
  <div class="w-56 max-w-full"><Combobox aria-label="Model" bind:value={model} items={modelOptions} placeholder="Model" /></div>
  {#if harness === 'claude' || harness === 'pig'}
    <div class="w-44 max-w-full"><Combobox aria-label="Reasoning effort" bind:value={effort} items={effortItems} placeholder="Effort" /></div>
  {/if}
</div>
