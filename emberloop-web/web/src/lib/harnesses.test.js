import { describe, it, expect } from 'vitest';
import { HARNESSES, modelsForHarness } from './harnesses.js';

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
