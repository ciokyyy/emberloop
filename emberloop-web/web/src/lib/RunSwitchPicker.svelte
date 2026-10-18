<!-- Backend/model selector for a paused Run or Ticket. -->
<script>
  import Button from './ui/button.svelte';
  import Combobox from './ui/combobox.svelte';
  import { HARNESSES, modelsForHarness, modelItems } from './harnesses.js';
  import { getModels } from './api.js';

  let {
    label = 'Run',
    description = '',
    initialBackend = 'pig',
    initialModel = '',
    onSwitch = null,
    disabled = false,
  } = $props();

  const backendItems = HARNESSES.filter((harness) => harness.id !== 'fake').map((harness) => ({ value: harness.id, label: harness.label }));
  let backend = $state('pig');
  let model = $state('');
  let catalogue = $state({});
  let switching = $state(false);
  let modelRequest = 0;
  let syncedSelection = '';

  const modelOptions = $derived(modelItems(backend, catalogue[backend] ?? modelsForHarness(backend), model));

  async function loadModels(harness, current = '', chooseFirst = false) {
    const request = ++modelRequest;
    let list = modelsForHarness(harness);
    try {
      const body = await getModels(harness, current);
      if (body.models?.length) list = body.models;
    } catch {
      // Use the local model catalogue when a Backend is temporarily unavailable.
    }
    if (request !== modelRequest) return;
    catalogue[harness] = list;
    if (chooseFirst && backend === harness && !list.includes(model)) model = list[0] ?? '';
  }

  $effect(() => {
    const nextBackend = backendItems.some((item) => item.value === initialBackend) ? initialBackend : 'pig';
    const nextModel = initialModel || modelsForHarness(nextBackend)[0] || '';
    const key = `${nextBackend}\u0000${nextModel}`;
    if (key === syncedSelection) return;
    syncedSelection = key;
    backend = nextBackend;
    model = nextModel;
    void loadModels(nextBackend, nextModel);
  });

  function pickBackend(value) {
    backend = value;
    model = catalogue[value]?.[0] ?? modelsForHarness(value)[0] ?? '';
    void loadModels(value, model, true);
  }

  async function switchBackend() {
    if (!backend || !model || !onSwitch || switching) return;
    switching = true;
    try {
      await onSwitch({ backend, model });
    } finally {
      switching = false;
    }
  }
</script>

<div class="flex min-w-[18rem] max-w-full flex-col gap-2" role="group" aria-label={`${label} Backend and model`} data-testid="run-switch-picker">
  {#if description}<p class="text-caption text-fg-3">{description}</p>{/if}
  <div class="flex flex-wrap items-center gap-2">
    <Combobox
      class="h-9 min-w-28 flex-1 max-md:h-11"
      aria-label={`${label} Backend`}
      value={backend}
      items={backendItems}
      disabled={disabled || switching}
      onValueChange={pickBackend}
    />
    <Combobox
      class="h-9 min-w-36 flex-1 max-md:h-11"
      aria-label={`${label} model`}
      bind:value={model}
      items={modelOptions}
      disabled={disabled || switching}
      placeholder="Select model"
    />
    <Button size="sm" variant="outline" disabled={disabled || switching || !backend || !model} onclick={switchBackend}>
      {switching ? 'Switching…' : `Switch ${label}`}
    </Button>
  </div>
</div>
