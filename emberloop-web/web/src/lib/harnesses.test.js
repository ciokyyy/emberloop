import { describe, it, expect } from 'vitest';
import { HARNESSES, modelsForHarness, splitModel, modelItems } from './harnesses.js';

describe('modelsForHarness', () => {
  it('lists a model per harness and filters by harness', () => {
    for (const h of HARNESSES) {
      const models = modelsForHarness(h.id);
      expect(models.length).toBeGreaterThan(0);
    }
    expect(modelsForHarness('claude')).not.toEqual(modelsForHarness('pig'));
  });
  it('falls back to a free-text model for unknown harnesses', () => {
    expect(modelsForHarness('nope')).toEqual([]);
  });
});

describe('modelItems', () => {
  it('splits pig models into name and provider, sorted by provider', () => {
    const items = modelItems('pig', ['openai/zulu', 'anthropic/alpha', 'openai/alpha']);
    expect(items.map((i) => i.value)).toEqual(['anthropic/alpha', 'openai/alpha', 'openai/zulu']);
    expect(items[0]).toMatchObject({ label: 'alpha', detail: 'anthropic' });
  });
  it('keeps the current selection first when the Daemon drops it', () => {
    const items = modelItems('pig', ['openai/alpha'], 'anthropic/alpha');
    expect(items[0]).toMatchObject({ value: 'anthropic/alpha', label: 'alpha', detail: 'anthropic' });
    expect(items).toHaveLength(2);
  });
  it('leaves the claude short list in order with no provider rows', () => {
    const items = modelItems('claude', ['opus', 'sonnet', 'haiku']);
    expect(items.map((i) => i.value)).toEqual(['opus', 'sonnet', 'haiku']);
    expect(items.every((i) => !i.detail)).toBe(true);
  });
  it('splits bare ids without a provider', () => {
    expect(splitModel('auto')).toEqual({ provider: '', name: 'auto' });
    expect(splitModel('openrouter/aurora')).toEqual({ provider: 'openrouter', name: 'aurora' });
  });
});

describe('harnessStyle', () => {
  it('marks claude terracotta and pig pig-logo teal', async () => {
    const { harnessStyle } = await import('./harnesses.js');
    expect(harnessStyle('claude').mark).toBe('C');
    expect(harnessStyle('pig').accent).toBe('#4fb08f');
    expect(harnessStyle('pig').tint).toBe('#6fcaa9');
    expect(harnessStyle('claude').accent).not.toBe(harnessStyle('pig').accent);
  });
  it('falls back to an initial mark for unknown harnesses', async () => {
    const { harnessStyle } = await import('./harnesses.js');
    expect(harnessStyle('mystery').mark).toBe('M');
  });
});
