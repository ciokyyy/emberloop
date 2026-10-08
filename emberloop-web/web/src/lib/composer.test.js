import { describe, it, expect } from 'vitest';
import { resolveComposerMode, availableModes, MODES } from './composer.js';

describe('resolveComposerMode', () => {
  it('idle session always sends a prompt', () => {
    expect(resolveComposerMode({ status: 'idle', selected: 'steer' })).toBe('prompt');
    expect(resolveComposerMode({ status: 'idle', selected: 'followup' })).toBe('prompt');
  });
  it('working session defaults to steer', () => {
    expect(resolveComposerMode({ status: 'working', selected: null })).toBe('steer');
  });
  it('working session honours an explicit follow-up choice', () => {
    expect(resolveComposerMode({ status: 'working', selected: 'followup' })).toBe('followup');
  });
  it('ended session always sends a prompt', () => {
    expect(resolveComposerMode({ status: 'ended', selected: 'steer' })).toBe('prompt');
  });
  it('exposes exactly the three documented modes', () => {
    expect(MODES.map((m) => m.id).sort()).toEqual(['followup', 'prompt', 'steer']);
  });
});

describe('availableModes', () => {
  it('offers only prompt when idle', () => {
    expect(availableModes('idle').map((m) => m.id)).toEqual(['prompt']);
  });
  it('offers steer and follow-up while working', () => {
    expect(availableModes('working').map((m) => m.id).sort()).toEqual(['followup', 'steer']);
  });
});
