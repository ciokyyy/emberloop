import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import Notifications from './Notifications.svelte';
import { getMemoryDigestPreference, setMemoryDigestPreference } from './api.js';

vi.mock('./api.js', async (orig) => ({
  ...(await orig()),
  getMemoryDigestPreference: vi.fn(async () => ({ enabled: false })),
  setMemoryDigestPreference: vi.fn(async (enabled) => ({ enabled })),
}));
vi.mock('./push.js', () => ({ pushUnsupported: () => '', pushEnabled: async () => false, enablePush: vi.fn(), disablePush: vi.fn() }));

describe('Notifications daily digest preference', () => {
  it('loads off by default and persists opt-in', async () => {
    const { findByRole } = render(Notifications);
    const toggle = await findByRole('switch', { name: 'Daily Memory digest' });
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(toggle);
    expect(setMemoryDigestPreference).toHaveBeenCalledWith(true);
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    expect(getMemoryDigestPreference).toHaveBeenCalledOnce();
  });
});
