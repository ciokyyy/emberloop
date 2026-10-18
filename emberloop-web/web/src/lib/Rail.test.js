import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, cleanup } from '@testing-library/svelte';
import Rail from './Rail.svelte';

afterEach(cleanup);

describe('rail collapse toggle', () => {
  it('offers to collapse and calls back', async () => {
    const onToggleSidebar = vi.fn();
    const { getByRole } = render(Rail, { props: { view: 'chat', collapsed: false, onToggleSidebar } });
    const btn = getByRole('button', { name: 'Collapse sidebar' });
    expect(btn.getAttribute('aria-keyshortcuts')).toMatch(/Control\+B/);
    await fireEvent.click(btn);
    expect(onToggleSidebar).toHaveBeenCalledOnce();
  });

  it('offers to expand when collapsed', () => {
    const { getByRole } = render(Rail, { props: { view: 'chat', collapsed: true, onToggleSidebar: vi.fn() } });
    expect(getByRole('button', { name: 'Expand sidebar' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('offers the Quota and Providers area label', () => {
    const onSelect = vi.fn();
    const { getByRole } = render(Rail, { props: { view: 'chat', onSelect } });
    const btn = getByRole('button', { name: 'Quota and Providers' });
    btn.click();
    expect(onSelect).toHaveBeenCalledWith('quota');
  });

  it('offers a Memory area', () => {
    const onSelect = vi.fn();
    const { getByRole } = render(Rail, { props: { view: 'chat', onSelect } });
    const btn = getByRole('button', { name: 'Memory' });
    btn.click();
    expect(onSelect).toHaveBeenCalledWith('memory');
  });
});
