import { describe, expect, it } from 'vitest';
import { findSettings } from './settings.js';

describe('findSettings', () => {
  it.each([
    'memory_extract_model',
    'memory_extract_effort',
    'memory_embedding_model',
    'memory_recall_weights',
    'memory_auto_extract',
  ])('finds the Memory control %s', (term) => {
    expect(findSettings(term).map((page) => page.id)).toContain('memory');
  });
});
