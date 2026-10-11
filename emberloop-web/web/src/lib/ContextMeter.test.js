import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import ContextMeter from './ContextMeter.svelte';

describe('ContextMeter compress button', () => {
  it('hides the button when the Session cannot compress', () => {
    const { container } = render(ContextMeter, { props: { model: 'm', usage: { used: 1, size: 2 } } });
    expect(within(container).queryByText('Compress')).toBeNull();
  });
  it('calls onCompress when idle', async () => {
    const onCompress = vi.fn();
    const { container } = render(ContextMeter, { props: { model: 'm', usage: { used: 1, size: 2 }, onCompress } });
    await fireEvent.click(within(container).getByText('Compress'));
    expect(onCompress).toHaveBeenCalledOnce();
  });
  it('disables the button while the Session works', () => {
    const { container } = render(ContextMeter, { props: { model: 'm', usage: null, onCompress: () => {}, busy: true } });
    expect(within(container).getByText('Compress').closest('button').disabled).toBe(true);
  });
  it('shows progress while compressing', () => {
    const { container } = render(ContextMeter, { props: { model: 'm', usage: null, onCompress: () => {}, compressing: true } });
    const btn = within(container).getByText('Compressing…').closest('button');
    expect(btn.disabled).toBe(true);
  });
});

describe('ContextMeter compress reason', () => {
  it('disables Compress and shows the reason it cannot run', () => {
    const onCompress = vi.fn();
    const reason = 'no compress model: pick one in Settings';
    const { container } = render(ContextMeter, { props: { model: 'm', usage: null, onCompress, compressNote: reason } });
    const btn = within(container).getByText('Compress').closest('button');
    expect(btn.disabled).toBe(true);
    expect(btn.title).toBe(reason);
    expect(within(container).getByText(reason)).toBeTruthy();
  });
});
