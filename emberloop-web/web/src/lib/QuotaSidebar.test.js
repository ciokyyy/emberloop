import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import QuotaSidebar from './QuotaSidebar.svelte';
import { createQuotaStore } from './quotaStore.js';

afterEach(cleanup);

function sourceWith(initialProviders) {
  let publish;
  const fetch = vi.fn(async () => ({ providers: initialProviders }));
  const store = createQuotaStore({
    fetch,
    subscribe: (onUpdate) => {
      publish = onUpdate;
      return vi.fn();
    },
  });
  return { store, fetch, publish: (body) => publish(body) };
}

describe('QuotaSidebar', () => {
  it('shows the Quota navigation and each provider highest pct with threshold tones', async () => {
    const { store } = sourceWith([
      { key: 'claude', label: 'Claude', windows: [{ pct: 36 }, { pct: 8 }] },
      { key: 'router', label: 'OpenRouter', windows: [{ pct: 40, detail: '$99 of $100' }, { pct: 60 }] },
      { key: 'agy', label: 'Antigravity', windows: [] },
    ]);
    const onNavigate = vi.fn();
    const { container } = render(QuotaSidebar, { props: { store, onNavigate } });

    expect(await screen.findByRole('heading', { name: 'Quota and Providers' })).toBeTruthy();
    expect(screen.getByRole('heading', { name: 'Live status' })).toBeTruthy();
    const nav = screen.getByRole('navigation', { name: 'Quota and Providers navigation' });
    const quota = within(nav).getByRole('link', { name: 'Quota' });
    expect(quota.getAttribute('href')).toBe('/quota');
    expect(quota.getAttribute('aria-current')).toBe('page');
    await fireEvent.click(quota);
    expect(onNavigate).toHaveBeenCalledWith('/quota');

    const rows = within(nav).getAllByTestId('quota-status-row');
    expect(rows.map((row) => row.dataset.key)).toEqual(['claude', 'router', 'agy']);
    expect(within(rows[0]).getByTestId('quota-status-percent').textContent.trim()).toBe('36%');
    expect(within(rows[0]).getByTestId('quota-status-dot').className).toContain('bg-green-500');
    expect(within(rows[1]).getByTestId('quota-status-percent').textContent.trim()).toBe('60%');
    expect(within(rows[1]).getByTestId('quota-status-dot').className).toContain('bg-amber-500');
    expect(within(rows[2]).getByTestId('quota-status-percent').textContent.trim()).toBe('—');
    expect(within(rows[2]).getByTestId('quota-status-dot').className).toContain('bg-input');
    expect(container.textContent).not.toContain('$99');
  });

  it('patches the shared list when its replaceable source publishes an update', async () => {
    const { store, publish } = sourceWith([{ key: 'claude', label: 'Claude', windows: [{ pct: 36 }] }]);
    render(QuotaSidebar, { props: { store } });
    const row = await screen.findByTestId('quota-status-row');
    expect(within(row).getByTestId('quota-status-percent').textContent.trim()).toBe('36%');

    publish({ providers: [{ key: 'claude', label: 'Claude', windows: [{ pct: 82 }] }] });
    await waitFor(() => {
      expect(within(row).getByTestId('quota-status-percent').textContent.trim()).toBe('82%');
    });
    expect(within(row).getByTestId('quota-status-dot').className).toContain('bg-red-500');
  });
  it('shows Providers in the same sidebar and marks its route active', async () => {
    const { store } = sourceWith([]);
    const onNavigate = vi.fn();
    render(QuotaSidebar, { props: { store, current: 'providers', onNavigate } });
    const nav = screen.getByRole('navigation', { name: 'Quota and Providers navigation' });
    const providers = within(nav).getByRole('link', { name: 'Providers' });
    expect(providers.getAttribute('href')).toBe('/providers');
    expect(providers.getAttribute('aria-current')).toBe('page');
    expect(within(nav).getByRole('link', { name: 'Quota' }).getAttribute('aria-current')).toBeNull();
    await fireEvent.click(providers);
    expect(onNavigate).toHaveBeenCalledWith('/providers');
  });

});
