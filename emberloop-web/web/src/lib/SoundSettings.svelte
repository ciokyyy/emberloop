<!-- Settings section: interface sounds. Per-device, stored in localStorage. -->
<script>
  import Button from './ui/button.svelte';
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

<section class="animate-rise flex flex-col gap-2" data-testid="sound-settings">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Sounds</h2>
    <p class="text-xs text-muted-foreground">Short sounds when a turn finishes, an approval waits, or a Run needs you. This device only.</p>
  </div>
  <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Sound pack</div>
        <div class="text-xs text-muted-foreground">Minimal is dry and quiet; the rest are louder personalities.</div>
      </div>
      <Select aria-label="Sound pack" value={prefs.pack} items={packItems} onValueChange={(v) => v && save({ ...prefs, pack: v })} />
      <Button variant="outline" size="sm" onclick={preview}>Preview</Button>
    </div>
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_minmax(0,16rem)_3rem]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Volume</div>
        <div class="text-xs text-muted-foreground">Master volume for every sound on this device.</div>
      </div>
      <Slider
        label="Master volume"
        value={volumePct}
        min={0}
        max={100}
        step={1}
        onValueChange={(v) => save({ ...prefs, volume: v / 100 })}
      />
      <span class="text-right text-xs tabular-nums text-muted-foreground">{volumePct}%</span>
    </div>
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Mute</div>
        <div class="text-xs text-muted-foreground">Silence every sound without losing the volume.</div>
      </div>
      <Switch aria-label="Mute all sounds" checked={prefs.muted} onCheckedChange={(v) => save({ ...prefs, muted: v })} />
    </div>
    {#each SOUND_EVENTS as ev (ev.id)}
      <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
        <div class="min-w-0">
          <div class="text-sm font-medium">{ev.label}</div>
          <div class="text-xs text-muted-foreground">{ev.hint}</div>
        </div>
        <Switch
          aria-label={`${ev.label} sound`}
          checked={prefs.events[ev.id] ?? true}
          onCheckedChange={(v) => save({ ...prefs, events: { ...prefs.events, [ev.id]: v } })}
        />
      </div>
    {/each}
  </div>
</section>
