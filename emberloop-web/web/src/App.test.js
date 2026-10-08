import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import App from './App.svelte';

describe('Runs navigation', () => {
  it('opens the Runs view from the sidebar', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => ({
        ok: true,
        json: async () => {
          if (String(u).includes('/api/specs')) return { specs: [] };
          return [];
        },
      })),
    );
    const { container } = render(App);
    await fireEvent.click(await within(container).findByText('Runs'));
    expect(await within(container).findByText(/repo directory/i)).toBeTruthy();
  });
});
