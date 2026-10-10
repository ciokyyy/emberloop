import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, fireEvent, cleanup } from '@testing-library/svelte';
import Rail from './Rail.svelte';

afterEach(cleanup);

describe('rail collapse toggle', () => {
  it('offers to collapse and calls back', async () => {
    const onToggleSidebar = vi.fn();
    const { getByRole } = render(Rail, { props: { view: 'chat', collapsed: false, onToggleSidebar } });
    const btn = getByRole('button', { name: 'Collapse sidebar' });
    expect(btn.getAttribute('title')).toMatch(/Ctrl/);
    await fireEvent.click(btn);
    expect(onToggleSidebar).toHaveBeenCalledOnce();
  });

  it('offers to expand when collapsed', () => {
    const { getByRole } = render(Rail, { props: { view: 'chat', collapsed: true, onToggleSidebar: vi.fn() } });
    expect(getByRole('button', { name: 'Expand sidebar' }).getAttribute('aria-pressed')).toBe('true');
  });

  it('offers a Quota area', () => {
    const onSelect = vi.fn();
    const { getByRole } = render(Rail, { props: { view: 'chat', onSelect } });
    const btn = getByRole('button', { name: 'Quota' });
    btn.click();
    expect(onSelect).toHaveBeenCalledWith('quota');
  });
});
