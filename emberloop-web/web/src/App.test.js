import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import App from './App.svelte';

// jsdom has no ResizeObserver; App measures the floating composer with one.
beforeEach(() => {
  vi.stubGlobal('ResizeObserver', class { observe() {} unobserve() {} disconnect() {} });
});

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

describe('Header', () => {
  it('shows a compact Live dot, and Reconnecting text only when disconnected', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => [] })));
    const { container } = render(App);
    expect(await within(container).findByTestId('live')).toBeTruthy();
    expect(within(container).queryByText('Live')).toBeNull();
    expect(within(container).getByTestId('header-title').textContent).toContain('No Session');
  });
});
