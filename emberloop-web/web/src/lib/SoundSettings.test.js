import { describe, it, expect } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import SoundSettings from './SoundSettings.svelte';
import { DEFAULT_PREFS } from './sound.js';

function fakePlayer(over = {}) {
  const prefs = JSON.parse(JSON.stringify({ ...DEFAULT_PREFS, ...over }));
  return {
    prefs,
    calls: [],
    previews: [],
    preference() {
      return JSON.parse(JSON.stringify(this.prefs));
    },
    setPreference(next) {
      this.calls.push(next);
      Object.assign(this.prefs, JSON.parse(JSON.stringify(next)));
    },
    preview(pack) {
      this.previews.push(pack);
      return true;
    },
  };
}

describe('SoundSettings', () => {
  it('shows the pack picker with minimal selected and previews on demand', async () => {
    const player = fakePlayer();
    const { container } = render(SoundSettings, { props: { player } });
    const section = within(container).getByTestId('sound-settings');

    expect(within(section).getByText('Minimal')).toBeTruthy();
    await fireEvent.click(within(section).getByRole('button', { name: 'Preview' }));
    expect(player.previews).toEqual(['minimal']);
  });

  it('mutes through the switch and persists the preference', async () => {
    const player = fakePlayer();
    const { container } = render(SoundSettings, { props: { player } });
    const section = within(container).getByTestId('sound-settings');

    await fireEvent.click(within(section).getByRole('switch', { name: 'Mute all sounds' }));
    await tick();
    expect(player.calls.at(-1).muted).toBe(true);
    expect(player.prefs.muted).toBe(true);
  });

  it('toggles one event without touching the others', async () => {
    const player = fakePlayer();
    const { container } = render(SoundSettings, { props: { player } });
    const section = within(container).getByTestId('sound-settings');

    await fireEvent.click(within(section).getByRole('switch', { name: 'Sudo request sound' }));
    await tick();
    const last = player.calls.at(-1);
    expect(last.events.sudo).toBe(false);
    expect(last.events.turnDone).toBe(true);
  });

  it('moves the master volume through the slider', async () => {
    const player = fakePlayer();
    const { container } = render(SoundSettings, { props: { player } });
    const section = within(container).getByTestId('sound-settings');

    const slider = within(section).getByRole('slider', { name: 'Master volume' });
    expect(slider).toBeTruthy();
    slider.focus();
    await fireEvent.keyDown(slider, { key: 'ArrowRight' });
    await tick();
    const last = player.calls.at(-1);
    expect(last).toBeTruthy();
    expect(last.volume).toBeCloseTo(0.71);
  });
});
