import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import DiskBanner from './DiskBanner.svelte';

describe('DiskBanner', () => {
  it('renders nothing when the disk is fine', () => {
    const { container } = render(DiskBanner, { props: { status: { low: false, critical: false, freeBytes: 50e9 } } });
    expect(container.querySelector('[data-testid="disk-banner"]')).toBeNull();
  });

  it('shows a one-line warning when low, critical when critical', () => {
    const { container, rerender } = render(DiskBanner, { props: { status: { low: true, critical: false, freeBytes: 15e9 } } });
    const banner = container.querySelector('[data-testid="disk-banner"]');
    expect(banner).toBeTruthy();
    expect(banner.textContent).toContain('Disk low');
    rerender({ status: { low: true, critical: true, freeBytes: 1e9 } });
    expect(container.querySelector('[data-testid="disk-banner"]').textContent).toContain('critically');
  });
});
