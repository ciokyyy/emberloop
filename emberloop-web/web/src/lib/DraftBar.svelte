<!-- Draft chat header: the project is already chosen, pick harness and model
     for the chat the first message will start. The choice is remembered. -->
<script>
  import { HARNESSES, modelsForHarness, modelItems } from './harnesses.js';
  import { getModels } from './api.js';
  import Combobox from './ui/combobox.svelte';

  let { harness = $bindable('claude'), model = $bindable('sonnet') } = $props();

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

<div class="flex items-center gap-2">
  <div class="w-32"><Combobox aria-label="Harness" value={harness} items={harnessItems} onValueChange={pickHarness} /></div>
  <div class="w-56"><Combobox aria-label="Model" bind:value={model} items={modelOptions} placeholder="Model" /></div>
</div>
