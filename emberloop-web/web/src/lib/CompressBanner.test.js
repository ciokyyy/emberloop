import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import CompressBanner from './CompressBanner.svelte';
import { compressSession, dismissCompress } from './api.js';

vi.mock('./api.js', () => ({
  compressSession: vi.fn(async () => ({ ok: true })),
  dismissCompress: vi.fn(async () => ({ dismissed: true })),
}));

beforeEach(() => {
  vi.mocked(compressSession).mockClear();
  vi.mocked(dismissCompress).mockClear();
});

const mount = (props = {}) =>
  render(CompressBanner, { props: { sessionId: 's1', onChangeModel: vi.fn(), onDismissed: vi.fn(), ...props } });

describe('CompressBanner', () => {
  it('names the block and offers Retry, Change compress model and Dismiss', () => {
    const { container } = mount();
    const banner = container.querySelector('[data-testid="compress-banner"]');
    expect(banner.textContent).toContain('compress needs pig credits');
    for (const name of ['Retry', 'Change compress model', 'Dismiss']) {
      expect(within(banner).getByRole('button', { name })).toBeTruthy();
    }
  });

  it('Retry reruns the compress of its Session', async () => {
    const { container } = mount();
    await fireEvent.click(within(container).getByRole('button', { name: 'Retry' }));
    expect(compressSession).toHaveBeenCalledWith('s1');
  });

  it('Change compress model opens Settings and sends nothing', async () => {
    const onChangeModel = vi.fn();
    const { container } = mount({ onChangeModel });
    await fireEvent.click(within(container).getByRole('button', { name: 'Change compress model' }));
    expect(onChangeModel).toHaveBeenCalledTimes(1);
    expect(compressSession).not.toHaveBeenCalled();
    expect(dismissCompress).not.toHaveBeenCalled();
  });

  it('Dismiss clears the block on its Session', async () => {
    const onDismissed = vi.fn();
    const { container } = mount({ onDismissed });
    await fireEvent.click(within(container).getByRole('button', { name: 'Dismiss' }));
    await vi.waitFor(() => expect(onDismissed).toHaveBeenCalledTimes(1));
    expect(dismissCompress).toHaveBeenCalledWith('s1');
  });

  it('a refused Retry shows its reason and keeps the block', async () => {
    vi.mocked(compressSession).mockRejectedValueOnce(new Error('compress model is not an authed pig model'));
    const onDismissed = vi.fn();
    const { container } = mount({ onDismissed });
    await fireEvent.click(within(container).getByRole('button', { name: 'Retry' }));
    await vi.waitFor(() =>
      expect(within(container).getByRole('alert').textContent).toContain('not an authed pig model'),
    );
    expect(onDismissed).not.toHaveBeenCalled();
  });

  it('a refused Dismiss shows its reason and keeps the block', async () => {
    vi.mocked(dismissCompress).mockRejectedValueOnce(new Error('session is working; dismiss when it is idle'));
    const onDismissed = vi.fn();
    const { container } = mount({ onDismissed });
    await fireEvent.click(within(container).getByRole('button', { name: 'Dismiss' }));
    await vi.waitFor(() =>
      expect(within(container).getByRole('alert').textContent).toContain('session is working'),
    );
    expect(onDismissed).not.toHaveBeenCalled();
  });

  it('Retry and Dismiss stay off while a request runs', async () => {
    let finish;
    vi.mocked(compressSession).mockImplementationOnce(() => new Promise((resolve) => (finish = resolve)));
    const { container } = mount();
    const retry = within(container).getByRole('button', { name: 'Retry' });
    const dismiss = within(container).getByRole('button', { name: 'Dismiss' });
    await fireEvent.click(retry);
    expect(retry.disabled).toBe(true);
    expect(dismiss.disabled).toBe(true);
    finish({ ok: true });
    await vi.waitFor(() => expect(retry.disabled).toBe(false));
  });
});
