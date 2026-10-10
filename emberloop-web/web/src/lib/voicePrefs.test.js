import { describe, it, expect, beforeEach, vi } from 'vitest';
import { readVoicePrefs, writeVoicePrefs, onVoicePrefs, DEFAULT_VOICE, VOICE_KEY } from './voicePrefs.js';

describe('voice prefs', () => {
  beforeEach(() => localStorage.clear());

  it('defaults to voice on, cleanup off', () => {
    expect(readVoicePrefs()).toEqual({ enabled: true, cleanup: false });
    expect(DEFAULT_VOICE).toEqual({ enabled: true, cleanup: false });
  });
  it('round-trips and survives corrupt storage', () => {
    writeVoicePrefs({ enabled: false, cleanup: true });
    expect(readVoicePrefs()).toEqual({ enabled: false, cleanup: true });
    localStorage.setItem(VOICE_KEY, '{nope');
    expect(readVoicePrefs()).toEqual({ enabled: true, cleanup: false });
  });
  it('notifies subscribers in this tab until unsubscribed', () => {
    const cb = vi.fn();
    const off = onVoicePrefs(cb);
    writeVoicePrefs({ enabled: false, cleanup: false });
    expect(cb).toHaveBeenCalledWith({ enabled: false, cleanup: false });
    off();
    writeVoicePrefs({ enabled: true, cleanup: false });
    expect(cb).toHaveBeenCalledOnce();
  });
});
