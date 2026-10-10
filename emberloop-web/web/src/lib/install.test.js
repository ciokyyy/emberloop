import { describe, it, expect } from 'vitest';
import { isIOSDevice, isStandalone, installState } from './install.js';

describe('install helpers', () => {
  it('detects iOS devices by user agent', () => {
    expect(isIOSDevice('Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)')).toBe(true);
    expect(isIOSDevice('Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X)')).toBe(true);
    expect(isIOSDevice('Mozilla/5.0 (Linux; Android 14; Pixel 8)')).toBe(false);
    expect(isIOSDevice('')).toBe(false);
  });

  it('detects standalone display mode', () => {
    expect(isStandalone({ standalone: true })).toBe(true);
    expect(isStandalone({ matchMedia: () => ({ matches: true }) })).toBe(true);
    expect(isStandalone({ matchMedia: () => ({ matches: false }) })).toBe(false);
    expect(isStandalone({})).toBe(false);
  });

  it('picks the settings card state', () => {
    expect(installState({ standalone: true, hasPrompt: true })).toBe('installed');
    expect(installState({ hasPrompt: true })).toBe('prompt');
    expect(installState({ ios: true })).toBe('ios-hint');
    expect(installState({})).toBe('unavailable');
  });
});
