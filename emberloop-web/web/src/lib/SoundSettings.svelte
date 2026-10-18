<!-- Settings section: interface sounds. Per-device, stored in localStorage. -->
<script>
  import { Volume2 } from '@lucide/svelte';
  import Button from './ui/button.svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import SettingRow from './ui/setting-row.svelte';
  import Select from './ui/select.svelte';
  import Slider from './ui/slider.svelte';
  import Switch from './ui/switch.svelte';
  import { PACKS, SOUND_EVENTS, DEFAULT_PREFS, soundPlayer } from './sound.js';

  let { player = soundPlayer() } = $props();

  const blank = () => JSON.parse(JSON.stringify(DEFAULT_PREFS));
  let prefs = $state(player?.preference() ?? blank());
  let volumePct = $state(Math.round(prefs.volume * 100));

  const packItems = PACKS.map((p) => ({ value: p, label: p[0].toUpperCase() + p.slice(1) }));

  function save(next) {
    prefs = next;
    volumePct = Math.round(next.volume * 100);
    try {
      player?.setPreference(next);
    } catch {
      // Sound is never worth an error on the settings page.
    }
  }

  function preview() {
    try {
      player?.preview(prefs.pack);
    } catch {
      // A failed preview is silence, not an error.
    }
  }
</script>

<Section
  title="Sounds"
  icon={Volume2}
  hint="Short sounds when a turn finishes, an approval waits, or a Run needs you. This device only."
  class="animate-rise"
  data-testid="sound-settings"
>
  <Panel>
    <SettingRow label="Sound pack" hint="Minimal is dry and quiet; the rest are louder personalities." width="field" controlClass="flex items-center gap-2">
      <Select aria-label="Sound pack" value={prefs.pack} items={packItems} onValueChange={(v) => v && save({ ...prefs, pack: v })} />
      <Button variant="outline" size="sm" onclick={preview}>Preview</Button>
    </SettingRow>
    <SettingRow label="Volume" hint="Master volume for every sound on this device." width="field" controlClass="flex items-center gap-3">
      <Slider
        label="Master volume"
        value={volumePct}
        min={0}
        max={100}
        step={1}
        onValueChange={(v) => save({ ...prefs, volume: v / 100 })}
      />
      <span class="w-10 shrink-0 text-right text-caption tabular-nums text-fg-3">{volumePct}%</span>
    </SettingRow>
    <SettingRow label="Mute" hint="Silence every sound without losing the volume." inline>
      <Switch aria-label="Mute all sounds" checked={prefs.muted} onCheckedChange={(v) => save({ ...prefs, muted: v })} />
    </SettingRow>
    {#each SOUND_EVENTS as ev (ev.id)}
      <SettingRow label={ev.label} hint={ev.hint} inline>
        <Switch
          aria-label={`${ev.label} sound`}
          checked={prefs.events[ev.id] ?? true}
          onCheckedChange={(v) => save({ ...prefs, events: { ...prefs.events, [ev.id]: v } })}
        />
      </SettingRow>
    {/each}
  </Panel>
</Section>
