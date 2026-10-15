import { afterEach, describe, it, expect, vi } from 'vitest';
import { cleanup, render, screen, within, fireEvent } from '@testing-library/svelte';
import SettingsHub from './SettingsHub.svelte';
import { SETTINGS_GROUPS, settingsPath } from './settings.js';

afterEach(cleanup);

describe('SettingsHub', () => {
  it('groups every page under Workspace and You with its route', () => {
    render(SettingsHub);
    const hub = screen.getByTestId('settings-hub');
    expect(within(hub).getByRole('heading', { name: 'Workspace' })).toBeTruthy();
    expect(within(hub).getByRole('heading', { name: 'You' })).toBeTruthy();

    for (const group of SETTINGS_GROUPS) {
      for (const item of group.items) {
        const link = within(hub).getByRole('link', { name: new RegExp(item.label) });
        expect(link.getAttribute('href')).toBe(settingsPath(item.id));
      }
    }
    expect(within(hub).getByText('Global')).toBeTruthy();
  });

  it('navigates through the selected concern link', async () => {
    const onNavigate = vi.fn();
    render(SettingsHub, { props: { onNavigate } });
    const link = screen.getByRole('link', { name: /Sound & voice/ });
    await fireEvent.click(link);
    expect(onNavigate).toHaveBeenCalledWith('/settings/sound-voice');
  });
});
