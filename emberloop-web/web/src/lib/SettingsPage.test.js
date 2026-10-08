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
    expect(within(container).getByText(/EMBERLOOP_MAX_WORKERS/)).toBeTruthy();
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
