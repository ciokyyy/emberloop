<!-- Settings section: voice input. Per-device, stored in localStorage. -->
<script>
  import Switch from './ui/switch.svelte';
  import { readVoicePrefs, writeVoicePrefs } from './voicePrefs.js';

  let prefs = $state(readVoicePrefs());

  function save(next) {
    prefs = writeVoicePrefs(next);
  }
</script>

<section class="animate-rise flex flex-col gap-2" data-testid="voice-settings">
  <div class="px-1">
    <h2 class="text-sm font-semibold">Voice</h2>
    <p class="text-xs text-muted-foreground">Record a voice message, check the transcript, then send. The model reads the transcript only. This device only.</p>
  </div>
  <div class="divide-y divide-border overflow-hidden rounded-2xl border bg-card">
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Voice messages</div>
        <div class="text-xs text-muted-foreground">Show the microphone button in the composer.</div>
      </div>
      <Switch aria-label="Voice messages" checked={prefs.enabled} onCheckedChange={(v) => save({ ...prefs, enabled: v })} />
    </div>
    <div class="grid items-center gap-3 p-4 md:grid-cols-[1fr_auto]">
      <div class="min-w-0">
        <div class="text-sm font-medium">Clean up transcripts</div>
        <div class="text-xs text-muted-foreground">Fix punctuation, casing and filler words with a small cloud model. Adds a short delay.</div>
      </div>
      <Switch aria-label="Clean up transcripts" checked={prefs.cleanup} disabled={!prefs.enabled} onCheckedChange={(v) => save({ ...prefs, cleanup: v })} />
    </div>
  </div>
</section>
