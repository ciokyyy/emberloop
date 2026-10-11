import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import SettingsPage from './SettingsPage.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubSettings({ settings, failPut = false, seen = {} }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      if (url.includes('/api/models')) {
        const harness = new URL(url, 'http://x').searchParams.get('harness');
        return { ok: true, json: async () => ({ models: harness === 'claude' ? ['opus'] : ['auto'] }) };
      }
      if (url.includes('/api/settings') && opts?.method === 'PUT') {
        seen.body = JSON.parse(opts.body);
        if (failPut) return { ok: false, status: 400 };
        return { ok: true, json: async () => ({ roles: seen.body.roles ?? {}, max_workers: seen.body.max_workers ?? 3, max_workers_from_env: false }) };
      }
      return { ok: true, json: async () => settings };
    }),
  );
}

const base = { roles: {}, max_workers: 2, max_workers_from_env: false };

describe('SettingsPage max_workers', () => {
  it('shows the saved value', async () => {
    stubSettings({ settings: base });
    const { container } = render(SettingsPage);
    const input = await within(container).findByLabelText('Maximum workers');
    expect(input.value).toBe('2');
  });

  it('saves an edit with the roles', async () => {
    const seen = {};
    stubSettings({ settings: base, seen });
    const { container } = render(SettingsPage);
    const input = await within(container).findByLabelText('Maximum workers');
    await fireEvent.input(input, { target: { value: '5' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.max_workers).toBe(5);
    expect(seen.body.roles).toBeTruthy();
  });

  it('locks the field with a note under an environment override', async () => {
    stubSettings({ settings: { roles: {}, max_workers: 1, max_workers_from_env: true } });
    const { container } = render(SettingsPage);
    const input = await within(container).findByLabelText('Maximum workers');
    expect(input.disabled).toBe(true);
    expect(within(container).getByText(/EL_MAX_WORKERS/)).toBeTruthy();
  });

  it('surfaces a rejected value as an error', async () => {
    stubSettings({ settings: base, failPut: true });
    const { container } = render(SettingsPage);
    const input = await within(container).findByLabelText('Maximum workers');
    await fireEvent.input(input, { target: { value: '0' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect((await within(container).findByRole('alert')).textContent).toMatch(/request failed: 400/);
  });

  it('locked save omits max_workers so the file cap survives', async () => {
    const seen = {};
    stubSettings({
      settings: {
        roles: { worker: { harness: 'pig', model: 'auto', effort: 'medium' } },
        max_workers: 1,
        max_workers_from_env: true,
      },
      seen,
    });
    const { container } = render(SettingsPage);
    const input = await within(container).findByLabelText('Maximum workers');
    // Disabled inputs take no typing, but the event still flips the binding:
    // enough to make the form dirty and reach Save.
    await fireEvent.input(input, { target: { value: '9' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect('max_workers' in seen.body).toBe(false);
  });

});

describe('SettingsPage session names', () => {
  async function dirtyAndSave(container) {
    const input = await within(container).findByLabelText('Maximum workers');
    await fireEvent.input(input, { target: { value: '4' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
  }

  it('shows the saved pig naming model and sends it back', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, roles: { 'pig-title': { harness: 'pig', model: 'auto' } }, session_titles: true }, seen });
    const { container } = render(SettingsPage);
    const model = await within(container).findByLabelText('Pig naming model');
    expect(model.textContent).toMatch(/auto/i);
    await dirtyAndSave(container);
    expect(seen.body.roles['pig-title']).toEqual({ harness: 'pig', model: 'auto' });
    expect(seen.body.session_titles).toBe(true);
  });

  it('leaves unconfigured roles out of the save', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, roles: { worker: { harness: 'claude', model: 'opus', effort: 'high' } } }, seen });
    const { container } = render(SettingsPage);
    await dirtyAndSave(container);
    expect(Object.keys(seen.body.roles).sort()).toEqual(['pig-title', 'worker']);
  });

  it('no pig model clears the role and claude naming off is saved', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, session_titles: false }, seen });
    const { container } = render(SettingsPage);
    await within(container).findByLabelText('Name claude Sessions');
    await dirtyAndSave(container);
    expect(seen.body.roles['pig-title']).toEqual({ harness: '', model: '' });
    expect(seen.body.session_titles).toBe(false);
  });

  it('an environment override locks the toggle and stays out of the save', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, session_titles: true, session_titles_from_env: true }, seen });
    const { container } = render(SettingsPage);
    expect(await within(container).findByText(/EL_SESSION_TITLES/)).toBeTruthy();
    await dirtyAndSave(container);
    expect('session_titles' in seen.body).toBe(false);
  });
});

describe('SettingsPage context compression', () => {
  it('shows saved values and sends the edited ones', async () => {
    const seen = {};
    stubSettings({
      settings: { ...base, compress: { auto_compress: false, compress_threshold_percent: 70, compress_cooldown: 3, compress_instructions: 'keep ids' } },
      seen,
    });
    const { container } = render(SettingsPage);
    const threshold = await within(container).findByLabelText('Threshold %');
    expect(threshold.value).toBe('70');
    expect(within(container).getByLabelText('Turns').value).toBe('3');
    expect(within(container).getByLabelText('Summary focus').value).toBe('keep ids');
    expect(within(container).getByLabelText('Auto-compress').getAttribute('aria-checked')).toBe('false');

    await fireEvent.input(within(container).getByLabelText('Summary focus'), { target: { value: 'keep paths' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.compress).toEqual({
      auto_compress: false,
      compress_threshold_percent: 70,
      compress_cooldown: 3,
      compress_instructions: 'keep paths',
      compress_model: '',
    });
  });

  it('uses the defaults when the Daemon sends none', async () => {
    stubSettings({ settings: base });
    const { container } = render(SettingsPage);
    expect((await within(container).findByLabelText('Threshold %')).value).toBe('80');
    expect(within(container).getByLabelText('Turns').value).toBe('5');
    expect(within(container).getByLabelText('Auto-compress').getAttribute('aria-checked')).toBe('true');
  });

  it('disables threshold and cooldown while auto-compress is off', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: false } } });
    const { container } = render(SettingsPage);
    expect((await within(container).findByLabelText('Threshold %')).disabled).toBe(true);
    expect(within(container).getByLabelText('Turns').disabled).toBe(true);
  });
});

describe('SettingsPage compress model', () => {
  it('shows the saved compress model and sends it back', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, compress: { compress_model: 'auto' } }, seen });
    const { container } = render(SettingsPage);
    const model = await within(container).findByLabelText('Compress model');
    expect(model.textContent).toMatch(/auto/i);
    await fireEvent.input(within(container).getByLabelText('Summary focus'), { target: { value: 'keep paths' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.compress.compress_model).toBe('auto');
  });

  it('sends an empty model when none is picked', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, compress: { auto_compress: true } }, seen });
    const { container } = render(SettingsPage);
    await within(container).findByLabelText('Compress model');
    await fireEvent.input(within(container).getByLabelText('Summary focus'), { target: { value: 'keep paths' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.compress.compress_model).toBe('');
  });

  it('shows why Compress is off until a compress model is set', async () => {
    stubSettings({ settings: { ...base, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = render(SettingsPage);
    expect(await within(container).findByText('no compress model: pick one in Settings')).toBeTruthy();
  });

  it('warns that auto-compress needs a compress model', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: true }, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = render(SettingsPage);
    expect(await within(container).findByText('Auto-compress needs a compress model.')).toBeTruthy();
  });

  it('no warning while auto-compress is off', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: false }, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = render(SettingsPage);
    await within(container).findByLabelText('Compress model');
    expect(within(container).queryByText('Auto-compress needs a compress model.')).toBeNull();
  });
});

describe('SettingsPage accounts', () => {
  it('shows the sign-up switch to the admin and saves it', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, signup_enabled: true }, seen });
    const { container } = render(SettingsPage, { props: { admin: true } });
    const toggle = await within(container).findByRole('switch', { name: 'Allow sign-up' });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    await fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.signup_enabled).toBe(false);
  });

  it('hides the switch from other Users and never sends the key', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, signup_enabled: false }, seen });
    const { container } = render(SettingsPage);
    const workers = await within(container).findByLabelText('Maximum workers');
    expect(within(container).queryByRole('switch', { name: 'Allow sign-up' })).toBeNull();
    await fireEvent.input(workers, { target: { value: '4' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect('signup_enabled' in seen.body).toBe(false);
  });
});
