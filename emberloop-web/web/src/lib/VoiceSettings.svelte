<!-- Settings section: voice input. Per-device, stored in localStorage. -->
<script>
  import { Mic } from '@lucide/svelte';
  import Panel from './ui/panel.svelte';
  import Section from './ui/section.svelte';
  import SettingRow from './ui/setting-row.svelte';
  import Switch from './ui/switch.svelte';
  import { readVoicePrefs, writeVoicePrefs } from './voicePrefs.js';

  let prefs = $state(readVoicePrefs());

  function save(next) {
    prefs = writeVoicePrefs(next);
  }
</script>

<Section
  title="Voice"
  icon={Mic}
  hint="Record a voice message, check the transcript, then send. The model reads the transcript only. This device only."
  class="animate-rise"
  data-testid="voice-settings"
>
  <Panel>
    <SettingRow label="Voice messages" hint="Show the microphone button in the composer." inline>
      <Switch aria-label="Voice messages" checked={prefs.enabled} onCheckedChange={(v) => save({ ...prefs, enabled: v })} />
    </SettingRow>
    <SettingRow label="Clean up transcripts" hint="Fix punctuation, casing and filler words with a small cloud model. Adds a short delay." inline>
      <Switch aria-label="Clean up transcripts" checked={prefs.cleanup} disabled={!prefs.enabled} onCheckedChange={(v) => save({ ...prefs, cleanup: v })} />
    </SettingRow>
  </Panel>
</Section>
