<!-- Settings page: one concern per page as Sections of Panels of SettingRows. Help text
     sits behind InfoHint; only the one-line page description shows. -->
<script>
  import { Check, ChevronDown, LockKeyhole, ShieldCheck } from '@lucide/svelte';
  import { onMount } from 'svelte';
  import { HARNESSES, modelsForHarness, modelItems } from './harnesses.js';
  import { getModels, getSettings, putSettings } from './api.js';
  import Switch from './ui/switch.svelte';
  import Button from './ui/button.svelte';
  import PageHeader from './ui/page-header.svelte';
  import FloatingInput from './ui/floating-input.svelte';
  import Combobox from './ui/combobox.svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import SettingRow from './ui/setting-row.svelte';
  import Skeleton from './ui/skeleton.svelte';
  import ErrorState from './ui/error-state.svelte';
  import ChainEditor from './ChainEditor.svelte';
  import { icons } from './icons.js';
  import Notifications from './Notifications.svelte';
  import InstallApp from './InstallApp.svelte';
  import ProfileSettings from './ProfileSettings.svelte';
  import SoundSettings from './SoundSettings.svelte';
  import VoiceSettings from './VoiceSettings.svelte';
  import { SETTINGS_PAGES } from './settings.js';
  import { settingsIcons } from './settingsIcons.js';
  let { section = 'agents', onBack = () => {}, admin = false } = $props();

  const DATA_SECTIONS = ['agents', 'runs', 'context', 'memory', 'access'];
  const descriptions = {
    agents: 'One ordered chain per role: a Primary and up to two fallbacks.',
    runs: 'Set the global ticket concurrency cap for each Run.',
    context: 'Keep long Sessions small and readable.',
    memory: 'Redacted user turns leave this host for the extractor and embedder when these features are on.',
    access: 'Manage who can sign in to this Daemon.',
    profile: 'Standing instructions agents can use about you.',
    notifications: 'Choose whether this device receives push notifications.',
    'sound-voice': 'Tune interface sounds, voice input and app installation for this device.',
  };
  const page = $derived(SETTINGS_PAGES[section] ?? SETTINGS_PAGES.agents);
  const Icon = $derived(settingsIcons[page.icon]);
  const pageDescription = $derived(descriptions[section] ?? '');

  const GROUPS = [
    {
      title: 'Session agents',
      note: 'Agents that plan work and assist a Session.',
      roles: [
        { id: 'orchestrator', label: 'Orchestrator', hint: 'Plans, delegates, and reviews results.' },
        { id: 'subagent', label: 'Subagent', hint: 'Spawned agent that edits code.' },
        { id: 'scout', label: 'Scout', hint: 'Read-only subagent that explores and reports.' },
      ],
    },
    {
      title: 'Run agents',
      note: 'Agents that implement tickets and review each change.',
      roles: [
        { id: 'worker', label: 'Ticket worker', hint: 'Implements a ticket in its Run worktree.' },
        { id: 'worker-design', label: 'Designer worker', hint: 'Implements a design ticket, judging the rendered page.' },
        { id: 'reviewer-spec', label: 'Spec reviewer', hint: 'Checks the change against the spec.' },
        { id: 'reviewer-standards', label: 'Standards reviewer', hint: 'Reviews the diff for defects and standards.' },
        { id: 'reviewer-design', label: 'Design reviewer', hint: 'Judges the rendered result of a design ticket.' },
      ],
    },
  ];
  const CHAIN_ROLES = GROUPS.flatMap((group) => group.roles);
  const LEGACY_ROLE_KEYS = {
    orchestrator: ['orchestrator'],
    subagent: ['worker'],
    scout: ['scout'],
    worker: ['implementer'],
    'reviewer-spec': ['reviewer-spec', 'spec-review'],
    'reviewer-standards': ['reviewer-standards', 'code-review'],
    'worker-design': ['worker-design'],
    'reviewer-design': ['reviewer-design'],
  };
  const harnessItems = HARNESSES.filter((h) => h.id !== 'fake').map((h) => ({ value: h.id, label: h.label }));
  const chainValues = (entries = []) => entries.map(({ backend, model, effort }) => ({ backend, model, ...(effort ? { effort } : {}) }));
  const cloneChains = (source) => Object.fromEntries(CHAIN_ROLES.map(({ id }) => [id, chainValues(source[id] ?? [])]));
  const effortLabel = (effort) => effort ? effort[0].toUpperCase() + effort.slice(1) : 'Default';
  // The pig naming model is optional: "None" clears the role, so pig Sessions stay unnamed.

  let roles = $state({});
  let chains = $state({});
  let maxTickets = $state('3');
  let diskCap = $state('8');
  let pigTitle = $state('none');
  let sessionTitles = $state('on');
  let compress = $state({ auto: 'on', threshold: '80', tokens: '300000', cooldown: '5', instructions: '', model: 'none' });
  let compressNote = $state('');
  let decider = $state({ mode: 'heuristic', model: 'none', low: '0.35', high: '0.7' });
  let jev = $state({ baseURL: '', apiKey: '', model: 'jev-latest' });
  let jevOpen = $state(false);
  let jevKeySet = $state(false);
  let findModel = $state('');
  let findInfo = $state({ default: 'typesafe/jev-1.13', fromEnv: false, suggestions: [] });
  let deciderNote = $state('');
  let signupOpen = $state('on');
  let memoryExtractModel = $state('inclusionai/ling-3.0-flash');
  let memoryExtractEffort = $state('low');
  let memoryEmbeddingModel = $state('qwen/qwen3-embedding-4b');
  let memoryAutoExtract = $state(true);
  let memoryRecallWeights = $state({ vector: '0.5', fts: '0.3', importance: '0.2' });
  let titlesLocked = $state(false);
  let locked = $state(false);
  let saved = $state(null);
  let catalogue = $state({});
  let imageCatalogue = $state({});
  let effortCatalogue = $state({});
  let loading = $state(true);
  let saving = $state(false);
  let justSaved = $state(false);
  let error = $state('');

  const snapshot = () => JSON.stringify({ roles, chains, maxTickets, diskCap, pigTitle, sessionTitles, compress, decider, jev, findModel, signupOpen, memoryExtractModel, memoryExtractEffort, memoryEmbeddingModel, memoryAutoExtract, memoryRecallWeights });
  const dirty = $derived(saved !== null && DATA_SECTIONS.includes(section) && snapshot() !== saved);

  async function models(harness) {
    let list = modelsForHarness(harness);
    let efforts = null;
    let imageInput = {};
    try {
      const body = await getModels(harness);
      if (body.models?.length) list = body.models;
      efforts = body.model_efforts ?? {};
      imageInput = body.image_input ?? {};
    } catch {
      // Static model fallback; capabilities stay unknown.
    }
    catalogue = { ...catalogue, [harness]: list };
    effortCatalogue = { ...effortCatalogue, [harness]: efforts };
    imageCatalogue = { ...imageCatalogue, [harness]: imageInput };
  }

  function lacksImageInput(role, entry) {
    return ['worker-design', 'reviewer-design'].includes(role) && imageCatalogue[entry.backend]?.[entry.model] === false;
  }

  function supportedEffortsFor(harness, model) {
    const byModel = effortCatalogue[harness];
    if (!byModel || !Object.prototype.hasOwnProperty.call(byModel, model)) return null;
    return byModel[model] ?? [];
  }

  function effortItemsFor(harness, model) {
    const efforts = supportedEffortsFor(harness, model) ?? [];
    return [{ value: '', label: 'Default' }, ...efforts.map((effort) => ({ value: effort, label: effortLabel(effort) }))];
  }

  function chainsFromSettings(body) {
    return Object.fromEntries(CHAIN_ROLES.map((role) => {
      const entries = (body.chains?.[role.id] ?? []).map((entry) => {
        const backend = entry.backend ?? entry.harness ?? '';
        const model = entry.model ?? '';
        let effort = entry.effort ?? '';
        const supported = supportedEffortsFor(backend, model);
        if (effort && supported && !supported.includes(effort)) effort = '';
        return { backend, model, effort };
      });
      return [role.id, entries];
    }));
  }

  async function load() {
    try {
      const body = await getSettings();
      await Promise.all(harnessItems.map((h) => models(h.value)));
      roles = body.roles ?? {};
      chains = chainsFromSettings(body);
      maxTickets = body.max_tickets != null ? String(body.max_tickets) : '3';
      diskCap = body.disk_go_cache_cap_gb != null ? String(body.disk_go_cache_cap_gb) : '8';
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
      jevOpen = !!j.base_url?.trim();
      jevKeySet = !!j.api_key_set;
      memoryExtractModel = body.memory_extract_model ?? 'inclusionai/ling-3.0-flash';
      memoryExtractEffort = body.memory_extract_effort ?? 'low';
      memoryEmbeddingModel = body.memory_embedding_model ?? 'qwen/qwen3-embedding-4b';
      memoryAutoExtract = body.memory_auto_extract !== false;
      const weights = body.memory_recall_weights ?? { vector: 0.5, fts: 0.3, importance: 0.2 };
      memoryRecallWeights = { vector: String(weights.vector ?? 0.5), fts: String(weights.fts ?? 0.3), importance: String(weights.importance ?? 0.2) };
      const f = body.find ?? {};
      findModel = f.decision_model ?? '';
      findInfo = { default: f.default || 'typesafe/jev-1.13', fromEnv: !!f.from_env, suggestions: f.suggestions ?? [] };
      saved = snapshot();
    } catch (e) {
      error = e.message;
    } finally {
      loading = false;
    }
  }

  onMount(() => { if (DATA_SECTIONS.includes(section)) void load(); });

  const itemsFor = (harness, current) => modelItems(harness, catalogue[harness] ?? [], current);

  const pigTitleItems = () => [{ value: 'none', label: 'None' }, ...itemsFor('pig', pigTitle === 'none' ? '' : pigTitle)];
  const compressModelItems = () => [{ value: 'none', label: 'None' }, ...itemsFor('pig', compress.model === 'none' ? '' : compress.model)];

  const EFFORT_OPTIONS = ['off', 'low', 'medium'].map((value) => ({ value, label: value }));

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
    const backend = entries.at(-1)?.backend ?? 'claude';
    const model = catalogue[backend]?.[0] ?? modelsForHarness(backend)[0] ?? '';
    chains[role] = [...entries, { backend, model, effort: '' }];
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
    entry.effort = '';
  }

  function pickChainModel(role, index, model) {
    const entry = chains[role][index];
    entry.model = model;
    const supported = supportedEffortsFor(entry.backend, model);
    if (!supported || !supported.includes(entry.effort)) entry.effort = '';
  }

  function chainsValid() {
    return CHAIN_ROLES.every(({ id }) => (chains[id] ?? []).length <= 3 && (chains[id] ?? []).every((entry) => entry.backend && entry.model));
  }

  function rolesForSave(pigTitleRole) {
    const next = { ...Object.fromEntries(Object.entries(roles).filter(([, setting]) => setting?.model)), 'pig-title': pigTitleRole };
    for (const { id } of CHAIN_ROLES) {
      const entry = chains[id]?.[0];
      for (const legacyRole of LEGACY_ROLE_KEYS[id]) {
        next[legacyRole] = entry
          ? { harness: entry.backend, model: entry.model, ...(entry.effort ? { effort: entry.effort } : {}) }
          : { harness: '', model: '' };
      }
    }
    return next;
  }

  function chainsForSave() {
    return cloneChains(chains);
  }
  async function save() {
    if (!chainsValid()) {
      error = 'Each chain entry needs a harness and model; each role can have at most three entries.';
      return;
    }
    saving = true;
    error = '';
    try {
      // Locked means the field holds the env value, not the file value:
      // leave the env-owned keys out so a save never writes them into the file.
      const pigTitleRole = pigTitle === 'none' ? { harness: '', model: '' } : { harness: 'pig', model: pigTitle };
      const payload = { roles: rolesForSave(pigTitleRole), chains: chainsForSave() };
      if (!locked) payload.max_tickets = Number(maxTickets);
      if (admin) payload.disk_go_cache_cap_gb = Number(diskCap);
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
      if (admin) {
        payload.decider = {
          decider_mode: decider.mode,
          memory_model: decider.model === 'none' ? '' : decider.model,
          decider_confirm_low: Number(decider.low),
          decider_confirm_high: Number(decider.high),
        };
        payload.jev = { base_url: jev.baseURL.trim(), model: jev.model.trim() };
        if (jev.apiKey) payload.jev.api_key = jev.apiKey;
        payload.memory_extract_model = memoryExtractModel.trim();
        payload.memory_extract_effort = memoryExtractEffort;
        payload.memory_embedding_model = memoryEmbeddingModel.trim();
        payload.memory_auto_extract = memoryAutoExtract;
        payload.memory_recall_weights = { vector: Number(memoryRecallWeights.vector), fts: Number(memoryRecallWeights.fts), importance: Number(memoryRecallWeights.importance) };
        if (!findInfo.fromEnv) payload.find = { decision_model: findModel.trim() };
      }
      const savedBody = await putSettings(payload);
      roles = savedBody.roles ?? payload.roles;
      chains = chainsFromSettings(savedBody);
      maxTickets = savedBody.max_tickets != null ? String(savedBody.max_tickets) : maxTickets;
      if (savedBody.disk_go_cache_cap_gb != null) diskCap = String(savedBody.disk_go_cache_cap_gb);
      locked = !!savedBody.max_tickets_from_env;
      compressNote = savedBody.compress_note ?? '';
      deciderNote = savedBody.decider_note ?? '';
      const savedBaseURL = savedBody.jev?.base_url ?? jev.baseURL;
      jev = { baseURL: savedBaseURL, apiKey: '', model: savedBody.jev?.model || 'jev-latest' };
      jevOpen = !!savedBaseURL.trim();
      jevKeySet = !!savedBody.jev?.api_key_set;
      memoryExtractModel = savedBody.memory_extract_model ?? memoryExtractModel;
      memoryExtractEffort = savedBody.memory_extract_effort ?? memoryExtractEffort;
      memoryEmbeddingModel = savedBody.memory_embedding_model ?? memoryEmbeddingModel;
      memoryAutoExtract = savedBody.memory_auto_extract ?? memoryAutoExtract;
      if (savedBody.memory_recall_weights) memoryRecallWeights = Object.fromEntries(Object.entries(savedBody.memory_recall_weights).map(([key, value]) => [key, String(value)]));
      findModel = savedBody.find?.decision_model ?? findModel;
      findInfo = { default: savedBody.find?.default || findInfo.default, fromEnv: !!savedBody.find?.from_env, suggestions: savedBody.find?.suggestions ?? findInfo.suggestions };
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
    if (saved === null) return;
    const prev = JSON.parse(saved);
    roles = prev.roles;
    maxTickets = prev.maxTickets;
    diskCap = prev.diskCap;
    pigTitle = prev.pigTitle;
    sessionTitles = prev.sessionTitles;
    compress = prev.compress;
    decider = prev.decider;
    jev = prev.jev;
    findModel = prev.findModel;
    signupOpen = prev.signupOpen;
    memoryExtractModel = prev.memoryExtractModel;
    memoryExtractEffort = prev.memoryExtractEffort;
    memoryEmbeddingModel = prev.memoryEmbeddingModel;
    memoryAutoExtract = prev.memoryAutoExtract;
    memoryRecallWeights = prev.memoryRecallWeights;
    chains = prev.chains;
  }
</script>

{#snippet segmented(label, options, current, pick)}
  <div role="group" aria-label={label} class="flex w-full gap-0.5 rounded-full bg-muted p-1 md:w-auto">
    {#each options as option (option.value)}
      <button
        type="button"
        class="motion-standard min-h-9 flex-1 rounded-full px-3 text-caption outline-none focus-visible:ring-[3px] focus-visible:ring-ring/40 max-md:min-h-11 max-md:text-body md:flex-none {current === option.value ? 'bg-surface-3 text-fg-1 shadow-elev-1' : 'text-fg-2 hover:text-fg-1'}"
        aria-pressed={current === option.value}
        onclick={() => pick(option.value)}
      >{option.label}</button>
    {/each}
  </div>
{/snippet}

<div class="relative flex min-h-0 flex-1 flex-col" data-testid="settings-page">
  <div class="flex-1 overflow-auto px-4 pt-6 pb-32 md:px-6">
    <div class="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <div class="flex flex-col gap-2">
        <PageHeader title={page.label} icon={Icon} scope={page.scope} scopeAttrs={{ 'data-testid': 'settings-scope' }} onBack={onBack} backLabel="Settings" backOn="phone" class="animate-rise" />
        {#if pageDescription}<p class="text-caption text-fg-3">{pageDescription}</p>{/if}
      </div>
      {#if DATA_SECTIONS.includes(section) && error}
        <ErrorState compact title={error} onRetry={load} class="animate-rise" />
      {/if}

      {#if !admin && (section === 'memory' || section === 'access')}
        <div class="flex items-start gap-2 rounded-xl bg-muted p-3 text-caption text-fg-3">
          <LockKeyhole class="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          <p role="status">Only admins can change these settings.</p>
        </div>
      {/if}
      {#if DATA_SECTIONS.includes(section) && loading}
        <div role="status" aria-label="Loading settings" class="flex flex-col gap-3">
          {#each [0, 1, 2] as i (i)}
            <Skeleton class="h-20 rounded-2xl" />
          {/each}
        </div>
      {:else}
        {#if section === 'runs'}
        <Section title="Concurrency" icon={icons.run} class="animate-rise">
          <Panel>
            <SettingRow
              label="Tickets per Run"
              hint="Global max_tickets caps concurrent tickets in each Run. A spec's max_tickets value can only lower this cap, never raise it. Sessions and subagents are uncapped. Applies immediately, no restart."
              note={locked ? 'Set by the EL_MAX_TICKETS environment variable. Unset it to edit here.' : ''}
              width="narrow"
            >
              <FloatingInput
                id="max-tickets"
                label="Maximum tickets per Run (spec)"
                type="number"
                min="1"
                step="1"
                bind:value={maxTickets}
                disabled={locked}
              />
            </SettingRow>
            {#if admin}
              <SettingRow label="Go build cache cap" hint="The janitor trims the cache past this size, oldest first, in GiB. 0 turns trimming off." width="narrow">
                <FloatingInput
                  id="disk-cap"
                  label="Go build cache cap (GiB, 0 disables trimming)"
                  type="number"
                  min="0"
                  step="1"
                  bind:value={diskCap}
                />
              </SettingRow>
            {/if}
          </Panel>
        </Section>
        {/if}
        {#if section === 'agents'}
        <div class="flex flex-col gap-6" data-testid="agent-chains-settings">
          {#each GROUPS as group (group.title)}
            <Section title={group.title} hint={group.note} class="animate-rise" data-testid={`agent-group-${group.title.toLowerCase().replaceAll(' ', '-')}`}>
              <Panel>
                {#each group.roles as role (role.id)}
                  <ChainEditor
                    {role}
                    entries={chains[role.id]}
                    {harnessItems}
                    {itemsFor}
                    {effortItemsFor}
                    {saving}
                    lacksImages={(entry) => lacksImageInput(role.id, entry)}
                    onadd={() => addChainEntry(role.id)}
                    onremove={(index) => removeChainEntry(role.id, index)}
                    onmove={(index, delta) => moveChainEntry(role.id, index, delta)}
                    onbackend={(index, value) => pickChainBackend(role.id, index, value)}
                    onmodel={(index, value) => pickChainModel(role.id, index, value)}
                    oneffort={(index, value) => (chains[role.id][index].effort = value)}
                  />
                {/each}
              </Panel>
            </Section>
          {/each}
        </div>
        {/if}
        {#if section === 'context'}
        <Section
          title="Context compression"
          icon={icons.compress}
          hint="Compress writes the Handoff for a Session with the compress model: one pig call, no tools. Run it by hand from the context ring in the chat header."
          class="animate-rise"
          data-testid="compress-settings"
        >
          <Panel>
            <SettingRow
              label="Compress model"
              hint="Writes the Handoff. Only authed pig models are listed."
              note={compressNote}
              noteTone="warn"
              width="field"
            >
              <Combobox class="md:h-10" aria-label="Compress model" bind:value={compress.model} items={compressModelItems()} placeholder="Select a model" />
            </SettingRow>
            <SettingRow
              label="Auto-compress"
              hint="Runs after a turn when the context passes the threshold."
              note={compress.auto === 'on' && compressNote ? 'Auto-compress needs a compress model.' : ''}
              noteTone="warn"
              inline
            >
              <Switch aria-label="Auto-compress" checked={compress.auto === 'on'} onCheckedChange={(v) => (compress.auto = v ? 'on' : 'off')} />
            </SettingRow>
            <SettingRow label="Threshold" hint="Percent of the model's context window, 10 to 99." width="narrow">
              <FloatingInput id="compress-threshold" label="Threshold %" type="number" min="10" max="99" step="1" bind:value={compress.threshold} disabled={compress.auto === 'off'} />
            </SettingRow>
            <SettingRow
              label="Threshold tokens"
              hint="Absolute token count that also triggers auto-compress; 0 means off. The percent gate never fires on 1M-window models, so keep this near 300000 for those."
              width="narrow"
            >
              <FloatingInput id="compress-threshold-tokens" label="Threshold tokens" type="number" min="0" step="1000" inputClass="text-base sm:text-sm" bind:value={compress.tokens} disabled={compress.auto === 'off'} />
            </SettingRow>
            <SettingRow label="Cooldown" hint="Turns to wait after a compression before the next automatic one." width="narrow">
              <FloatingInput id="compress-cooldown" label="Turns" type="number" min="0" step="1" bind:value={compress.cooldown} disabled={compress.auto === 'off'} />
            </SettingRow>
            <SettingRow label="Summary focus" hint="Optional instructions for every summary, such as what to always keep." stack width="full">
              <textarea
                aria-label="Summary focus"
                name="compress_instructions"
                rows="3"
                maxlength="2000"
                bind:value={compress.instructions}
                placeholder="Keep file paths, open decisions and failing tests."
                class="w-full resize-y rounded-xl border border-input bg-transparent px-3.5 py-2.5 text-base shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/30 md:text-sm"
              ></textarea>
            </SettingRow>
          </Panel>
        </Section>
        {/if}
        {#if section === 'memory' && admin}
        <Section title="Automatic memory" icon={icons.memory} class="animate-rise" data-testid="memory-retention-settings">
          <Panel>
            <SettingRow
              label="Automatic extraction"
              hint="Extract durable memories after settled turns. Defaults on when an OpenRouter key is configured."
              inline
            >
              <Switch aria-label="Automatic memory extraction" checked={memoryAutoExtract} onCheckedChange={(value) => (memoryAutoExtract = value)} />
            </SettingRow>
            <SettingRow label="Extractor model" hint="Empty disables model extraction." width="field">
              <FloatingInput id="memory-extract-model" label="Model" bind:value={memoryExtractModel} placeholder="inclusionai/ling-3.0-flash" />
            </SettingRow>
            <SettingRow label="Extractor effort" hint="Controls the reasoning effort for each extraction.">
              {@render segmented('Memory extractor effort', EFFORT_OPTIONS, memoryExtractEffort, (value) => (memoryExtractEffort = value))}
            </SettingRow>
            <SettingRow label="Embedding model" hint="Used for Memory and Episode vectors; empty disables embeddings." width="field">
              <FloatingInput id="memory-embedding-model" label="Model" bind:value={memoryEmbeddingModel} placeholder="qwen/qwen3-embedding-4b" />
            </SettingRow>
            <SettingRow
              label="Recall weights"
              hint="Vector, full-text and importance scores; each weight is 0 to 1 and the total must be positive."
              stack
              width="full"
            >
              <div class="grid grid-cols-3 gap-2">
                <FloatingInput id="memory-weight-vector" label="Vector" type="number" min="0" max="1" step="0.05" bind:value={memoryRecallWeights.vector} />
                <FloatingInput id="memory-weight-fts" label="Full text" type="number" min="0" max="1" step="0.05" bind:value={memoryRecallWeights.fts} />
                <FloatingInput id="memory-weight-importance" label="Importance" type="number" min="0" max="1" step="0.05" bind:value={memoryRecallWeights.importance} />
              </div>
            </SettingRow>
          </Panel>
        </Section>
        <Section
          title="Memory deciders"
          hint="The heuristic proposes mined notes; the confirming decider judges them. Ensemble confirms with the memory model, or with Jev when a custom endpoint is configured, against the thresholds."
          class="animate-rise"
          data-testid="decider-settings"
        >
          <Panel>
            <SettingRow
              label="Decider mode"
              hint="Heuristic is local. LLM uses the memory model; Ensemble uses the memory model or a custom Jev URL. Jev uses OpenRouter by default."
            >
              {@render segmented('Decider mode', deciderModeItems, decider.mode, (value) => (decider.mode = value))}
            </SettingRow>
            <SettingRow label="Memory model" hint="Confirms candidates. Only authed pig models are listed." note={deciderNote} noteTone="warn" width="field">
              <Combobox class="md:h-10" aria-label="Memory model" bind:value={decider.model} items={deciderModelItems()} placeholder="Select a model" />
            </SettingRow>
            <div>
              <button
                type="button"
                class="motion-standard flex min-h-14 w-full items-center justify-between gap-3 p-4 text-left outline-none hover:bg-surface-3/60 focus-visible:ring-[3px] focus-visible:ring-inset focus-visible:ring-ring/40"
                aria-expanded={jevOpen}
                aria-controls="custom-jev-settings"
                onclick={() => (jevOpen = !jevOpen)}
              >
                <span class="flex min-w-0 flex-col">
                  <span class="text-body text-fg-1">Custom Jev endpoint</span>
                  <span class="text-caption text-fg-3">Blank uses OpenRouter in Jev mode.</span>
                </span>
                <ChevronDown class={`motion-standard size-4 shrink-0 text-fg-3 ${jevOpen ? 'rotate-180' : ''}`} aria-hidden="true" />
              </button>
              <div id="custom-jev-settings" class="divide-y divide-border border-t" hidden={!jevOpen}>
                <SettingRow label="Jev endpoint" hint="Overrides the default OpenRouter endpoint." width="field">
                  <FloatingInput id="jev-endpoint" label="Endpoint URL" bind:value={jev.baseURL} placeholder="https://jev-api.org/api/v1/decisions" />
                </SettingRow>
                <SettingRow
                  label="Jev API key"
                  hint="Leave empty for a local server without one."
                  note={jevKeySet ? 'Custom endpoint key set. Type a new one to replace it; empty keeps it.' : ''}
                  width="field"
                >
                  <FloatingInput id="jev-key" label="API key" type="password" bind:value={jev.apiKey} placeholder={jevKeySet ? '••••••' : 'Key'} />
                </SettingRow>
                <SettingRow
                  label="Jev model"
                  note="When on, redacted user turns leave this host for the endpoint."
                  width="field"
                >
                  <FloatingInput id="jev-model" label="Model" bind:value={jev.model} placeholder="jev-latest" />
                </SettingRow>
              </div>
            </div>
            <SettingRow label="Confirm low" hint="Below this the candidate is dropped, 0 to 1." width="narrow">
              <FloatingInput id="decider-low" label="Low" type="number" min="0" max="1" step="0.05" bind:value={decider.low} />
            </SettingRow>
            <SettingRow label="Confirm high" hint="At or above this the candidate is queue-eligible, 0 to 1." width="narrow">
              <FloatingInput id="decider-high" label="High" type="number" min="0" max="1" step="0.05" bind:value={decider.high} />
            </SettingRow>
          </Panel>
        </Section>
        <Section
          title="Find tool"
          hint="The decisions model that ranks find results. Pick a cheaper one to cut cost. Empty uses the pinned default."
          class="animate-rise"
          data-testid="find-settings"
        >
          <Panel>
            <SettingRow
              label="Find decision model"
              hint={findInfo.fromEnv ? '' : `Any OpenRouter decisions model id. Default ${findInfo.default}.`}
              note={findInfo.fromEnv ? 'Set by the EL_DECISION_MODEL environment variable. Unset it to edit here.' : ''}
              width="field"
            >
              <FloatingInput id="find-decision-model" label="Model" bind:value={findModel} placeholder={findInfo.default} disabled={findInfo.fromEnv} />
            </SettingRow>
            {#if findInfo.suggestions.length && !findInfo.fromEnv}
              <div class="flex flex-wrap gap-2 p-4" role="group" aria-label="Find model suggestions">
                {#each findInfo.suggestions as id (id)}
                  <Button
                    size="sm"
                    variant={findModel.trim() === id ? 'secondary' : 'outline'}
                    aria-pressed={findModel.trim() === id}
                    onclick={() => (findModel = id)}
                  >{id}</Button>
                {/each}
              </div>
            {/if}
          </Panel>
        </Section>
        {/if}
        {#if section === 'profile'}
          <ProfileSettings />
        {:else if section === 'notifications'}
          <Notifications />
        {:else if section === 'sound-voice'}
          <div class="flex flex-col gap-6"><SoundSettings /><VoiceSettings /><InstallApp /></div>
        {/if}
        {#if section === 'context'}
        <Section title="Session names" icon={icons.session} class="animate-rise">
          <Panel>
            <SettingRow
              label="Name claude Sessions"
              hint="Each new Session gets a short title from its first prompt, in one cheap call with no tools. Titled by Haiku; turn off to keep the harness and id label."
              note={titlesLocked ? 'Set by the EL_SESSION_TITLES environment variable. Unset it to edit here.' : ''}
              inline
            >
              <Switch aria-label="Name claude Sessions" checked={sessionTitles === 'on'} onCheckedChange={(v) => (sessionTitles = v ? 'on' : 'off')} disabled={titlesLocked} />
            </SettingRow>
            <SettingRow label="Pig naming model" hint="Pig Sessions stay unnamed until you pick a model." width="field">
              <Combobox class="md:h-10" aria-label="Pig naming model" bind:value={pigTitle} items={pigTitleItems()} placeholder="Select a model" />
            </SettingRow>
          </Panel>
        </Section>
        {/if}
        {#if section === 'access' && admin}
        <Section title="Accounts" icon={ShieldCheck} class="animate-rise" data-testid="accounts-settings">
          <Panel>
            <SettingRow
              label="Allow sign-up"
              hint="Sign-up lets new people create accounts on the sign-in screen. Off refuses new accounts; existing accounts keep signing in."
              inline
            >
              <Switch aria-label="Allow sign-up" checked={signupOpen === 'on'} onCheckedChange={(v) => (signupOpen = v ? 'on' : 'off')} />
            </SettingRow>
          </Panel>
        </Section>
        {/if}
      {/if}
    </div>
  </div>

  {#if dirty || justSaved}
    <div class="pointer-events-none absolute inset-x-0 bottom-0 flex justify-center px-4 pb-[max(1.25rem,env(safe-area-inset-bottom))]">
      <div class="animate-rise pointer-events-auto flex items-center gap-3 rounded-full border bg-surface-2 py-2 pr-2 pl-5 shadow-elev-3">
        {#if justSaved && !dirty}
          <span class="flex items-center gap-1.5 text-body text-status-done"><Check class="size-4" aria-hidden="true" />Saved</span>
        {:else}
          <span class="text-body text-fg-2">Unsaved changes</span>
          <Button variant="ghost" size="sm" onclick={revert} disabled={saving}>Revert</Button>
          <Button size="sm" onclick={save} disabled={saving}>{saving ? 'Saving…' : 'Save'}</Button>
        {/if}
      </div>
    </div>
  {/if}
</div>
