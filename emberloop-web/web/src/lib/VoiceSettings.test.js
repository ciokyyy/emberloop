import { describe, it, expect, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import VoiceSettings from './VoiceSettings.svelte';
import { readVoicePrefs } from './voicePrefs.js';

describe('VoiceSettings', () => {
  beforeEach(() => localStorage.clear());

  it('turns voice messages off and on, persisted', async () => {
    const { container } = render(VoiceSettings);
    const q = within(container);
    const sw = q.getByLabelText('Voice messages');
    expect(sw.getAttribute('aria-checked')).toBe('true');
    await fireEvent.click(sw);
    await tick();
    expect(readVoicePrefs().enabled).toBe(false);
    expect(q.getByLabelText('Clean up transcripts').disabled).toBe(true);
  });

  it('toggles transcript cleanup', async () => {
    const { container } = render(VoiceSettings);
    const q = within(container);
    await fireEvent.click(q.getByLabelText('Clean up transcripts'));
    await tick();
    expect(readVoicePrefs().cleanup).toBe(true);
  });
});
