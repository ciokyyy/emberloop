import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ContextMeter from './ContextMeter.svelte';

afterEach(() => {
  document.body.innerHTML = '';
});

const mount = (props = {}) => render(ContextMeter, { props: { model: 'm', usage: { used: 1, size: 2 }, ...props } });
const openSheet = async () => {
  await fireEvent.click(screen.getByTestId('context-meter'));
  return screen.findByTestId('context-sheet');
};

describe('ContextMeter ring', () => {
  it('is one icon button named by the percent, with no visible text', () => {
    const { container } = mount({ usage: { used: 150000, size: 1000000 } });
    const ring = container.querySelector('[data-testid="context-meter"]');
    expect(ring.tagName).toBe('BUTTON');
    expect(ring.getAttribute('aria-label')).toBe('Context 15% used');
    expect(ring.textContent.trim()).toBe('');
    expect(ring.querySelector('svg')).toBeTruthy();
  });
  it('names the empty state when there is no usage yet', () => {
    const { container } = mount({ usage: null });
    expect(container.querySelector('[data-testid="context-meter"]').getAttribute('aria-label')).toBe('Context: no usage yet');
  });
  it('warms the ring near the limit', () => {
    const { container } = mount({ usage: { used: 95, size: 100 } });
    expect(container.querySelector('[data-testid="context-meter"]').className).toContain('text-status-failed');
  });
});

describe('ContextMeter detail sheet', () => {
  it('stays closed until the ring is tapped, then shows model and tokens', async () => {
    mount({ model: 'gpt-x', usage: { used: 150000, size: 1000000 } });
    expect(screen.queryByTestId('context-sheet')).toBeNull();
    const sheet = await openSheet();
    expect(sheet.textContent).toContain('gpt-x');
    expect(screen.getByTestId('context-pct').textContent).toBe('15%');
    expect(screen.getByTestId('context-tokens').textContent).toBe('150k of 1M tokens');
  });
  it('hides Compress when the Session cannot compress', async () => {
    mount();
    await openSheet();
    expect(screen.queryByRole('button', { name: 'Compress' })).toBeNull();
  });
  it('calls onCompress when idle and closes the sheet', async () => {
    const onCompress = vi.fn();
    mount({ onCompress });
    await openSheet();
    await fireEvent.click(screen.getByRole('button', { name: 'Compress' }));
    expect(onCompress).toHaveBeenCalledOnce();
    await vi.waitFor(() => expect(screen.queryByTestId('context-sheet')).toBeNull());
  });
  it('disables Compress while the Session works', async () => {
    mount({ usage: null, onCompress: () => {}, busy: true });
    await openSheet();
    expect(screen.getByRole('button', { name: 'Compress' }).disabled).toBe(true);
    expect(screen.getByTestId('compress-hint').textContent).toContain('idle');
  });
  it('shows progress while compressing', async () => {
    mount({ usage: null, onCompress: () => {}, compressing: true });
    await openSheet();
    expect(screen.getByRole('button', { name: 'Compress' }).disabled).toBe(true);
    expect(screen.getByTestId('compress-hint').textContent).toContain('Compressing');
  });
  it('disables Compress and shows the reason it cannot run', async () => {
    const onCompress = vi.fn();
    const reason = 'no compress model: pick one in Settings';
    mount({ usage: null, onCompress, compressNote: reason });
    await openSheet();
    expect(screen.getByRole('button', { name: 'Compress' }).disabled).toBe(true);
    expect(screen.getByTestId('compress-hint').textContent).toBe(reason);
  });
  it('shows the usage even when the Session cannot compress', async () => {
    mount({ usage: { used: 150000, size: 1000000 } });
    await openSheet();
    expect(screen.getByTestId('context-pct').textContent).toBe('15%');
    expect(screen.queryByRole('button', { name: 'Compress' })).toBeNull();
  });
});
