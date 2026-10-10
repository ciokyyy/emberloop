import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import InstallApp from './InstallApp.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubEnv({ ua = 'Mozilla/5.0 (Linux; Android 14; Pixel 8)', standalone = false } = {}) {
  Object.defineProperty(window.navigator, 'userAgent', { value: ua, configurable: true });
  Object.defineProperty(window.navigator, 'standalone', { value: standalone, configurable: true });
  window.matchMedia = vi.fn(() => ({ matches: standalone }));
}

const installButton = (container) => within(container).queryByRole('button', { name: 'Install app' });

describe('InstallApp', () => {
  it('shows the install button once beforeinstallprompt fires', async () => {
    stubEnv();
    const { container } = render(InstallApp);
    expect(installButton(container)).toBeNull();
    await fireEvent(window, new Event('beforeinstallprompt'));
    expect(await within(container).findByRole('button', { name: 'Install app' })).toBeTruthy();
  });

  it('prompts and clears on click', async () => {
    stubEnv();
    const { container } = render(InstallApp);
    const prompt = vi.fn();
    const evt = new Event('beforeinstallprompt');
    evt.preventDefault = () => {};
    evt.prompt = prompt;
    window.dispatchEvent(evt);
    await fireEvent.click(await within(container).findByRole('button', { name: 'Install app' }));
    expect(prompt).toHaveBeenCalled();
  });

  it('shows the iOS hint with no install event', async () => {
    stubEnv({ ua: 'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X)' });
    const { container } = render(InstallApp);
    expect(await within(container).findByText(/Add to Home Screen/)).toBeTruthy();
    expect(installButton(container)).toBeNull();
  });
});
