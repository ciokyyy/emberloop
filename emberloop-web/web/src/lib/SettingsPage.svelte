<!-- Settings page: harness, model and effort per role, grouped like Apple Settings. -->
<script>
  import { ArrowDown, ArrowLeft, ArrowUp, Check, Trash2 } from '@lucide/svelte';
  import { HARNESSES, modelsForHarness, modelItems } from './harnesses.js';
  import { getModels, getSettings, putSettings } from './api.js';
  import Switch from './ui/switch.svelte';
  import Button from './ui/button.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import Combobox from './ui/combobox.svelte';
  import Notifications from './Notifications.svelte';
  import InstallApp from './InstallApp.svelte';
  import ProfileSettings from './ProfileSettings.svelte';
  import SoundSettings from './SoundSettings.svelte';
  import VoiceSettings from './VoiceSettings.svelte';
  let { onBack = () => {}, admin = false } = $props();

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

  const CHAIN_ROLES = [
    { id: 'worker', settingRoles: ['worker', 'implementer'], label: 'Worker' },
    { id: 'reviewer-spec', settingRoles: ['reviewer-spec', 'spec-review'], label: 'Spec reviewer' },
    { id: 'reviewer-standards', settingRoles: ['reviewer-standards', 'code-review'], label: 'Standards reviewer' },
  ];
  const harnessItems = HARNESSES.filter((h) => h.id !== 'fake').map((h) => ({ value: h.id, label: h.label }));
  const effortItems = ['low', 'medium', 'high', 'max'].map((e) => ({ value: e, label: e[0].toUpperCase() + e.slice(1) }));
  const chainValues = (entries = []) => entries.map(({ backend, model, effort }) => ({ backend, model, ...(effort ? { effort } : {}) }));
  const cloneChains = (source) => Object.fromEntries(CHAIN_ROLES.map(({ id }) => [id, chainValues(source[id])]));
  const sameChain = (left, right) => JSON.stringify(chainValues(left)) === JSON.stringify(chainValues(right));
  // The pig naming model is optional: "None" clears the role, so pig Sessions stay unnamed.

  let roles = $state({});
  let chains = $state({});
  let inheritedChains = $state({});
  let chainBaseline = {};
  let chainAliases = {};
  let maxTickets = $state('3');
  let pigTitle = $state('none');
  let sessionTitles = $state('on');
  let compress = $state({ auto: 'on', threshold: '80', tokens: '300000', cooldown: '5', instructions: '', model: 'none' });
  let compressNote = $state('');
  let decider = $state({ mode: 'heuristic', model: 'none', low: '0.35', high: '0.7' });
  let jev = $state({ baseURL: '', apiKey: '', model: 'jev-latest' });
  let jevKeySet = $state(false);
  let deciderNote = $state('');
  let signupOpen = $state('on');
  let titlesLocked = $state(false);
  let locked = $state(false);
  let saved = $state('');
  let catalogue = $state({});
  let loading = $state(true);
  let saving = $state(false);
  let justSaved = $state(false);
  let error = $state('');

  function fallbackChain(role) {
    const fallback = role.settingRoles.map((name) => roles[name]).find((setting) => setting?.model);
    return fallback?.model ? [{ backend: fallback.harness, model: fallback.model, ...(fallback.effort ? { effort: fallback.effort } : {}) }] : [];
  }
  const snapshot = () => JSON.stringify({ roles, chains, maxTickets, pigTitle, sessionTitles, compress, decider, jev, signupOpen });
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
      const storedChains = body.chains ?? {};
      const nextChains = {};
      const nextInheritedChains = {};
      const nextChainAliases = {};
      for (const role of CHAIN_ROLES) {
        const explicitName = role.settingRoles.find((name) => Array.isArray(storedChains[name]) && storedChains[name].length > 0);
        const fallback = role.settingRoles.map((name) => next[name]).find((setting) => setting?.model);
        const entries = explicitName
          ? storedChains[explicitName]
          : fallback?.model
            ? [{ backend: fallback.harness, model: fallback.model, ...(fallback.effort ? { effort: fallback.effort } : {}) }]
            : [];
        nextChains[role.id] = entries.map((entry) => ({
          backend: entry.backend ?? entry.harness ?? '',
          model: entry.model ?? '',
          ...(entry.effort ? { effort: entry.effort } : {}),
        }));
        nextInheritedChains[role.id] = !explicitName;
        nextChainAliases[role.id] = role.settingRoles.filter((name) => name !== role.id && Array.isArray(storedChains[name]));
      }
      chains = nextChains;
      inheritedChains = nextInheritedChains;
      chainBaseline = cloneChains(nextChains);
      chainAliases = nextChainAliases;
      maxTickets = body.max_tickets != null ? String(body.max_tickets) : '3';
      locked = !!body.max_tickets_from_env;
      const t = body.roles?.['pig-title'];
      pigTitle = t?.harness === 'pig' && t?.model ? t.model : 'none';
      sessionTitles = body.session_titles === false ? 'off' : 'on';
      signupOpen = body.signup_enabled === false ? 'off' : 'on';
      titlesLocked = !!body.session_titles_from_env;
      const c = body.compress ?? {};
      compress = {
        auto: c.auto_compress === false ? 'off' : 'on',
        threshold: String(c.compress_threshold_percent ?? 80),
        tokens: String(c.compress_threshold_tokens ?? 300000),
        cooldown: String(c.compress_cooldown ?? 5),
        instructions: c.compress_instructions ?? '',
        model: c.compress_model || 'none',
      };
      compressNote = body.compress_note ?? '';
      const md = body.decider ?? {};
      decider = {
        mode: md.decider_mode || 'heuristic',
        model: md.memory_model || 'none',
        low: String(md.decider_confirm_low ?? 0.35),
        high: String(md.decider_confirm_high ?? 0.7),
      };
      deciderNote = body.decider_note ?? '';
      const j = body.jev ?? {};
      jev = { baseURL: j.base_url ?? '', apiKey: '', model: j.model || 'jev-latest' };
      jevKeySet = !!j.api_key_set;
      saved = snapshot();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }
  load();

  const itemsFor = (harness, current) => modelItems(harness, catalogue[harness] ?? [], current);

  function pickHarness(role, harness) {
    roles[role].harness = harness;
    roles[role].model = catalogue[harness]?.[0] ?? '';
  }

  const pigTitleItems = () => [{ value: 'none', label: 'None' }, ...itemsFor('pig', pigTitle === 'none' ? '' : pigTitle)];
  const compressModelItems = () => [{ value: 'none', label: 'None' }, ...itemsFor('pig', compress.model === 'none' ? '' : compress.model)];

  const deciderModeItems = [
    { value: 'heuristic', label: 'Heuristic' },
    { value: 'llm', label: 'LLM' },
    { value: 'jev', label: 'Jev' },
    { value: 'ensemble', label: 'Ensemble' },
  ];

  const deciderModelItems = () => [{ value: 'none', label: 'None' }, ...itemsFor('pig', decider.model === 'none' ? '' : decider.model)];
  function addChainEntry(role) {
    const entries = chains[role] ?? [];
    if (entries.length >= 3) return;
    const roleSettings = CHAIN_ROLES.find((item) => item.id === role)?.settingRoles ?? [];
    const fallback = roleSettings.map((name) => roles[name]).find((setting) => setting?.model) ?? {};
    const backend = fallback.model ? fallback.harness : 'claude';
    const model = fallback.model || 'sonnet';
    chains[role] = [...entries, { backend, model, ...(fallback.effort ? { effort: fallback.effort } : {}) }];
  }

  function removeChainEntry(role, index) {
    chains[role] = chains[role].filter((_, at) => at !== index);
  }

  function moveChainEntry(role, index, delta) {
    const entries = [...chains[role]];
    const destination = index + delta;
    if (destination < 0 || destination >= entries.length) return;
    [entries[index], entries[destination]] = [entries[destination], entries[index]];
    chains[role] = entries;
  }

  function pickChainBackend(role, index, backend) {
    const entry = chains[role][index];
    entry.backend = backend;
    entry.model = catalogue[backend]?.[0] ?? modelsForHarness(backend)[0] ?? '';
  }

  function chainsValid() {
    return CHAIN_ROLES.every(({ id }) => (chains[id] ?? []).length <= 3 && (chains[id] ?? []).every((entry) => entry.backend && entry.model));
  }

  // An untouched role-setting fallback is presentation only. Send a Chain
  // update only when its entries differ, and clear legacy aliases only when
  // that role's Chain is explicitly edited.
  function chainsForSave() {
    const payload = {};
    for (const { id } of CHAIN_ROLES) {
      if (sameChain(chains[id], chainBaseline[id])) continue;
      payload[id] = chainValues(chains[id]);
      for (const alias of chainAliases[id] ?? []) payload[alias] = [];
    }
    return payload;
  }

  async function save() {
    if (!chainsValid()) {
      error = 'Each Run Chain entry needs a Backend and model; each role can have at most three entries.';
      return;
    }
    saving = true;
    error = '';
    try {
      // Locked means the field holds the env value, not the file value:
      // leave the env-owned keys out so a save never writes them into the file.
      const pigTitleRole = pigTitle === 'none' ? { harness: '', model: '' } : { harness: 'pig', model: pigTitle };
      // A role with no model was never configured: leave it out instead of failing the whole save.
      const configured = Object.fromEntries(Object.entries(roles).filter(([, r]) => r.model));
      const chainPayload = chainsForSave();
      const editedChainRoles = new Set(CHAIN_ROLES.filter(({ id }) => Object.prototype.hasOwnProperty.call(chainPayload, id)).map(({ id }) => id));
      const payload = { roles: { ...configured, 'pig-title': pigTitleRole } };
      if (Object.keys(chainPayload).length) payload.chains = chainPayload;
      if (!locked) payload.max_tickets = Number(maxTickets);
      if (!titlesLocked) payload.session_titles = sessionTitles === 'on';
      payload.compress = {
        auto_compress: compress.auto === 'on',
        compress_threshold_percent: Number(compress.threshold),
        compress_threshold_tokens: Number(compress.tokens),
        compress_cooldown: Number(compress.cooldown),
        compress_instructions: compress.instructions,
        compress_model: compress.model === 'none' ? '' : compress.model,
      };
      if (admin) payload.signup_enabled = signupOpen === 'on';
      if (admin)
        payload.decider = {
          decider_mode: decider.mode,
          memory_model: decider.model === 'none' ? '' : decider.model,
          decider_confirm_low: Number(decider.low),
          decider_confirm_high: Number(decider.high),
        };
      if (admin) {
        payload.jev = { base_url: jev.baseURL.trim(), model: jev.model.trim() };
        if (jev.apiKey) payload.jev.api_key = jev.apiKey;
      }
      const savedBody = await putSettings(payload);
      maxTickets = savedBody.max_tickets != null ? String(savedBody.max_tickets) : maxTickets;
      locked = !!savedBody.max_tickets_from_env;
      compressNote = savedBody.compress_note ?? '';
      deciderNote = savedBody.decider_note ?? '';
      jev = { baseURL: savedBody.jev?.base_url ?? jev.baseURL, apiKey: '', model: savedBody.jev?.model || 'jev-latest' };
      jevKeySet = !!savedBody.jev?.api_key_set;
      for (const role of CHAIN_ROLES) {
        if (editedChainRoles.has(role.id)) {
          inheritedChains[role.id] = false;
          chainAliases[role.id] = [];
        } else if (inheritedChains[role.id]) {
          chains[role.id] = fallbackChain(role);
        }
      }
      chainBaseline = cloneChains(chains);
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
    maxTickets = prev.maxTickets;
    pigTitle = prev.pigTitle;
    sessionTitles = prev.sessionTitles;
    compress = prev.compress;
    decider = prev.decider;
    jev = prev.jev;
    signupOpen = prev.signupOpen;
    chains = prev.chains;
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
            <p class="text-xs text-muted-foreground">How many Run tickets run at once. Chat sessions and subagents are uncapped.</p>
          </div>
          <div class="flex flex-col gap-2 rounded-2xl border bg-card p-4">
            <FloatingInput
              id="max-tickets"
              label="Maximum tickets"
              type="number"
              min="1"
              step="1"
              class="max-w-56"
              bind:value={maxTickets}
              disabled={locked}
            />
            {#if locked}
              <p class="text-xs text-muted-foreground">Set by the EL_MAX_TICKETS environment variable. Unset it to edit here.</p>
            {:else}
              <p class="text-xs text-muted-foreground">Applies immediately, no restart.</p>
            {/if}
          </div>
        </section>
        <section class="animate-rise flex flex-col gap-2" data-testid="run-chains-settings">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Run Chains</h2>
            <p class="text-xs text-muted-foreground">Ordered Backend and model fallbacks for Workers and Reviewers. Each new Run snapshots these Chains; failures advance to the next entry only when the provider is out of quota, credit, auth or session capacity.</p>
          </div>
          <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
            {#each CHAIN_ROLES as role (role.id)}
              <section class="p-4" data-testid={`run-chain-${role.id}`}>
                <div class="mb-3 flex items-center justify-between gap-3">
                  <div>
                    <h3 class="text-sm font-medium">{role.label}</h3>
                    {#if inheritedChains[role.id] && chains[role.id]?.length}
                      <p class="text-xs text-muted-foreground">Inherited from the role setting; it is saved as a Chain only when you edit it.</p>
                    {:else if !chains[role.id]?.length}
                      <p class="text-xs text-muted-foreground">No explicit Chain; the regular role setting or Run defaults are used.</p>
                    {/if}
                  </div>
                  <Button size="sm" variant="outline" aria-label={`Add ${role.label} Chain entry`} onclick={() => addChainEntry(role.id)} disabled={saving || chains[role.id].length >= 3}>Add entry</Button>
                </div>
                {#each chains[role.id] as entry, index (index)}
                  <div class="grid items-center gap-2 border-t py-3 first:border-t-0 md:grid-cols-[minmax(0,1fr)_minmax(0,12rem)_minmax(0,16rem)_auto]" data-testid={`chain-entry-${role.id}-${index + 1}`}>
                    <span class="text-xs font-medium text-muted-foreground">Entry {index + 1}</span>
                    <Combobox
                      class="md:h-10"
                      aria-label={`${role.label} Chain entry ${index + 1} Backend`}
                      value={entry.backend}
                      items={harnessItems}
                      onValueChange={(value) => pickChainBackend(role.id, index, value)}
                    />
                    <Combobox
                      class="md:h-10"
                      aria-label={`${role.label} Chain entry ${index + 1} model`}
                      bind:value={entry.model}
                      items={itemsFor(entry.backend, entry.model)}
                      placeholder="Select a model"
                    />
                    <div class="flex items-center justify-end gap-1">
                      <Button size="icon-sm" variant="ghost" aria-label={`Move ${role.label} Chain entry ${index + 1} up`} disabled={saving || index === 0} onclick={() => moveChainEntry(role.id, index, -1)}><ArrowUp /></Button>
                      <Button size="icon-sm" variant="ghost" aria-label={`Move ${role.label} Chain entry ${index + 1} down`} disabled={saving || index === chains[role.id].length - 1} onclick={() => moveChainEntry(role.id, index, 1)}><ArrowDown /></Button>
                      <Button size="icon-sm" variant="ghost" aria-label={`Remove ${role.label} Chain entry ${index + 1}`} disabled={saving} onclick={() => removeChainEntry(role.id, index)}><Trash2 /></Button>
                    </div>
                  </div>
                {/each}
              </section>
            {/each}
          </div>
        </section>
        <section class="animate-rise flex flex-col gap-2" data-testid="compress-settings">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Context compression</h2>
            <p class="text-xs text-muted-foreground">Compress writes the Handoff for a Session with the compress model: one pig call, no tools. Use the Compress button under the chat to run it by hand.</p>
          </div>
          <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Compress model</div>
                <div class="text-xs text-muted-foreground">Writes the Handoff. Only authed pig models are listed.</div>
                {#if compressNote}<div class="text-xs text-amber-300">{compressNote}</div>{/if}
              </div>
              <Combobox class="md:h-10" aria-label="Compress model" bind:value={compress.model} items={compressModelItems()} placeholder="Select a model" />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Auto-compress</div>
                <div class="text-xs text-muted-foreground">Runs after a turn when the context passes the threshold.</div>
                {#if compress.auto === 'on' && compressNote}<div class="text-xs text-amber-300">Auto-compress needs a compress model.</div>{/if}
              </div>
              <Switch aria-label="Auto-compress" checked={compress.auto === 'on'} onCheckedChange={(v) => (compress.auto = v ? 'on' : 'off')} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Threshold</div>
                <div class="text-xs text-muted-foreground">Percent of the model's context window, 10 to 99.</div>
              </div>
              <FloatingInput id="compress-threshold" label="Threshold %" type="number" min="10" max="99" step="1" bind:value={compress.threshold} disabled={compress.auto === 'off'} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Threshold tokens</div>
                <div class="text-xs text-muted-foreground">Absolute token count that also triggers auto-compress; 0 means off. The percent gate never fires on 1M-window models, so keep this near 300000 for those.</div>
              </div>
              <FloatingInput id="compress-threshold-tokens" label="Threshold tokens" type="number" min="0" step="1000" inputClass="text-base sm:text-sm" bind:value={compress.tokens} disabled={compress.auto === 'off'} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Cooldown</div>
                <div class="text-xs text-muted-foreground">Turns to wait after a compression before the next automatic one.</div>
              </div>
              <FloatingInput id="compress-cooldown" label="Turns" type="number" min="0" step="1" bind:value={compress.cooldown} disabled={compress.auto === 'off'} />
            </div>
            <div class="grid items-start gap-3 p-4">
              <div class="min-w-0">
                <div class="text-sm font-medium">Summary focus</div>
                <div class="text-xs text-muted-foreground">Optional instructions for every summary, such as what to always keep.</div>
              </div>
              <textarea
                aria-label="Summary focus"
                name="compress_instructions"
                rows="3"
                maxlength="2000"
                bind:value={compress.instructions}
                placeholder="Keep file paths, open decisions and failing tests."
                class="w-full resize-y rounded-xl border border-input bg-transparent px-3.5 py-2.5 text-sm shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30"
              ></textarea>
            </div>
          </div>
        </section>
        {#if admin}
        <section class="animate-rise flex flex-col gap-2" data-testid="decider-settings">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Memory deciders</h2>
            <p class="text-xs text-muted-foreground">The heuristic proposes mined notes; the confirming decider judges them. Ensemble confirms with the memory model, or with Jev when its endpoint is on, against the thresholds.</p>
          </div>
          <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Decider mode</div>
                <div class="text-xs text-muted-foreground">Heuristic is local. LLM and ensemble ask the memory model. Jev asks the decision endpoint.</div>
              </div>
              <Combobox class="md:h-10" aria-label="Decider mode" bind:value={decider.mode} items={deciderModeItems} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Memory model</div>
                <div class="text-xs text-muted-foreground">Confirms candidates. Only authed pig models are listed.</div>
                {#if deciderNote}<div class="text-xs text-amber-300">{deciderNote}</div>{/if}
              </div>
              <Combobox class="md:h-10" aria-label="Memory model" bind:value={decider.model} items={deciderModelItems()} placeholder="Select a model" />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Jev endpoint</div>
                <div class="text-xs text-muted-foreground">Decision endpoint URL, empty when off. A local Laya server plugs in by URL.</div>
              </div>
              <FloatingInput id="jev-endpoint" label="Endpoint URL" bind:value={jev.baseURL} placeholder="https://jev-api.org/api/v1/decisions" />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Jev API key</div>
                {#if jevKeySet}
                  <div class="text-xs text-muted-foreground">Key set. Type a new one to replace it; empty keeps it.</div>
                {:else}
                  <div class="text-xs text-muted-foreground">Empty for a local server without one.</div>
                {/if}
              </div>
              <FloatingInput id="jev-key" label="API key" type="password" bind:value={jev.apiKey} placeholder={jevKeySet ? '••••••' : 'Key'} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Jev model</div>
                <div class="text-xs text-muted-foreground">Sent with every call. When on, redacted user turns leave this host for the endpoint.</div>
              </div>
              <FloatingInput id="jev-model" label="Model" bind:value={jev.model} placeholder="jev-latest" />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Confirm low</div>
                <div class="text-xs text-muted-foreground">Below this the candidate is dropped, 0 to 1.</div>
              </div>
              <FloatingInput id="decider-low" label="Low" type="number" min="0" max="1" step="0.05" bind:value={decider.low} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Confirm high</div>
                <div class="text-xs text-muted-foreground">At or above this the candidate is queue-eligible, 0 to 1.</div>
              </div>
              <FloatingInput id="decider-high" label="High" type="number" min="0" max="1" step="0.05" bind:value={decider.high} />
            </div>
          </div>
        </section>
        {/if}
        <ProfileSettings />
        <Notifications />
        <InstallApp />
        <SoundSettings />
        <VoiceSettings />
        <section class="animate-rise flex flex-col gap-2">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Session names</h2>
            <p class="text-xs text-muted-foreground">Each new Session gets a short title from its first prompt, in one cheap call with no tools.</p>
          </div>
          <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Name claude Sessions</div>
                <div class="text-xs text-muted-foreground">
                  {#if titlesLocked}Set by the EL_SESSION_TITLES environment variable. Unset it to edit here.{:else}Titled by Haiku. Turn off to keep the harness and id label.{/if}
                </div>
              </div>
              <Switch aria-label="Name claude Sessions" checked={sessionTitles === 'on'} onCheckedChange={(v) => (sessionTitles = v ? 'on' : 'off')} disabled={titlesLocked} />
            </div>
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Pig naming model</div>
                <div class="text-xs text-muted-foreground">Pig Sessions stay unnamed until you pick a model.</div>
              </div>
              <Combobox class="md:h-10" aria-label="Pig naming model" bind:value={pigTitle} items={pigTitleItems()} placeholder="Select a model" />
            </div>
          </div>
        </section>
        {#if admin}
        <section class="animate-rise flex flex-col gap-2" data-testid="accounts-settings">
          <div class="px-1">
            <h2 class="text-sm font-semibold">Accounts</h2>
            <p class="text-xs text-muted-foreground">Sign-up lets new people create accounts on the sign-in screen.</p>
          </div>
          <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
            <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_9rem]">
              <div class="min-w-0">
                <div class="text-sm font-medium">Allow sign-up</div>
                <div class="text-xs text-muted-foreground">Off refuses new accounts. Existing accounts keep signing in.</div>
              </div>
              <Switch aria-label="Allow sign-up" checked={signupOpen === 'on'} onCheckedChange={(v) => (signupOpen = v ? 'on' : 'off')} />
            </div>
          </div>
        </section>
        {/if}
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
                  <Combobox
                    class="md:h-10"
                    aria-label={`${r.label} harness`}
                    value={roles[r.id].harness}
                    items={harnessItems}
                    onValueChange={(v) => pickHarness(r.id, v)}
                  />
                  <Combobox
                    class="md:h-10"
                    aria-label={`${r.label} model`}
                    bind:value={roles[r.id].model}
                    items={itemsFor(roles[r.id].harness, roles[r.id].model)}
                    placeholder="Select a model"
                  />
                  <Combobox class="md:h-10" aria-label={`${r.label} effort`} bind:value={roles[r.id].effort} items={effortItems} />
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
