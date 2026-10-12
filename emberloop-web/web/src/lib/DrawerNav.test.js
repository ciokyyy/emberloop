import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, cleanup } from '@testing-library/svelte';
import DrawerNav from './DrawerNav.svelte';

afterEach(cleanup);

describe('DrawerNav', () => {
  it('marks the current area and selects another', async () => {
    const onSelect = vi.fn();
    const { getByRole } = render(DrawerNav, { props: { view: 'runs', onSelect } });
    expect(getByRole('button', { name: 'Runs' }).getAttribute('aria-current')).toBe('page');
    expect(getByRole('button', { name: 'Chat' }).getAttribute('aria-current')).toBeNull();
    await fireEvent.click(getByRole('button', { name: 'Quota' }));
    expect(onSelect).toHaveBeenCalledWith('quota');
  });
});
