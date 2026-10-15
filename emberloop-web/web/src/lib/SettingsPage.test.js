import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import SettingsPage from './SettingsPage.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

beforeEach(() => {
  if (!Element.prototype.scrollIntoView) Element.prototype.scrollIntoView = () => {};
});

function stubSettings({ settings, failGet = false, failPut = false, seen = {} }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      if (url.includes('/api/models')) {
        const harness = new URL(url, 'http://x').searchParams.get('harness');
        const models = harness === 'claude' ? ['opus'] : harness === 'grok' ? ['grok-4.7'] : ['auto'];
        return { ok: true, json: async () => ({ models }) };
      }
      if (url.includes('/api/settings') && opts?.method === 'PUT') {
        seen.body = JSON.parse(opts.body);
        if (failPut) return { ok: false, status: 400 };
        return { ok: true, json: async () => ({ roles: seen.body.roles ?? {}, max_tickets: seen.body.max_tickets ?? 3, max_tickets_from_env: false }) };
      }
      if (url.includes('/api/settings') && failGet) return { ok: false, status: 500 };
      return { ok: true, json: async () => settings };
    }),
  );
}

const base = { roles: {}, max_tickets: 2, max_tickets_from_env: false };
let section = 'agents';
function renderSettings(options = {}) {
  return render(SettingsPage, { ...options, props: { section, ...(options.props ?? {}) } });
}

describe('SettingsPage max_tickets', () => {
  beforeEach(() => { section = 'runs'; });
  it('shows the saved value', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    expect(input.value).toBe('2');
  });

  it('saves an edit with the roles', async () => {
    const seen = {};
    stubSettings({ settings: base, seen });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    await fireEvent.input(input, { target: { value: '5' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.max_tickets).toBe(5);
    expect(seen.body.roles).toBeTruthy();
  });

  it('locks the field with a note under an environment override', async () => {
    stubSettings({ settings: { roles: {}, max_tickets: 1, max_tickets_from_env: true } });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    expect(input.disabled).toBe(true);
    expect(within(container).getByText(/EL_MAX_TICKETS/)).toBeTruthy();
  });

  it('surfaces a rejected value as an error', async () => {
    stubSettings({ settings: base, failPut: true });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    await fireEvent.input(input, { target: { value: '0' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect((await within(container).findByRole('alert')).textContent).toMatch(/request failed: 400/);
  });

  it('locked save omits max_tickets so the file cap survives', async () => {
    const seen = {};
    stubSettings({
      settings: {
        roles: { worker: { harness: 'pig', model: 'auto', effort: 'medium' } },
        max_tickets: 1,
        max_tickets_from_env: true,
      },
      seen,
    });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    // Disabled inputs take no typing, but the event still flips the binding:
    // enough to make the form dirty and reach Save.
    await fireEvent.input(input, { target: { value: '9' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect('max_tickets' in seen.body).toBe(false);
  });
  it('keeps the save bar hidden while settings are loading', async () => {
    let resolveSettings;
    vi.stubGlobal(
      'fetch',
      vi.fn(async (u) => {
        const url = String(u);
        if (url.includes('/api/settings')) {
          return new Promise((resolve) => {
            resolveSettings = () => resolve({ ok: true, json: async () => base });
          });
        }
        const harness = new URL(url, 'http://x').searchParams.get('harness');
        const models = harness === 'claude' ? ['opus'] : harness === 'grok' ? ['grok-4.7'] : ['auto'];
        return { ok: true, json: async () => ({ models }) };
      }),
    );
    const { container } = renderSettings();
    expect(within(container).queryByRole('button', { name: 'Save' })).toBeNull();
    expect(within(container).queryByRole('button', { name: 'Revert' })).toBeNull();

    expect(resolveSettings).toBeDefined();
    resolveSettings();
    await within(container).findByLabelText('Maximum tickets');
    expect(within(container).queryByRole('button', { name: 'Save' })).toBeNull();
  });

  it('keeps the save bar hidden when settings fail to load', async () => {
    stubSettings({ settings: base, failGet: true });
    const { container } = renderSettings();
    expect((await within(container).findByRole('alert')).textContent).toMatch(/request failed: 500/);
    expect(within(container).queryByRole('button', { name: 'Save' })).toBeNull();
    expect(within(container).queryByRole('button', { name: 'Revert' })).toBeNull();
  });

  it('shows Revert and Save on the first change and Revert restores the saved value', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets');
    expect(within(container).queryByRole('button', { name: 'Save' })).toBeNull();

    await fireEvent.input(input, { target: { value: '5' } });
    await tick();
    expect(await within(container).findByText('Unsaved changes')).toBeTruthy();
    expect(within(container).getByRole('button', { name: 'Revert' })).toBeTruthy();
    expect(within(container).getByRole('button', { name: 'Save' })).toBeTruthy();

    await fireEvent.click(within(container).getByRole('button', { name: 'Revert' }));
    expect(input.value).toBe('2');
    expect(within(container).queryByText('Unsaved changes')).toBeNull();
  });

});

describe('SettingsPage session names', () => {
  beforeEach(() => { section = 'context'; });
  async function dirtyAndSave(container) {
    const input = await within(container).findByLabelText('Summary focus');
    await fireEvent.input(input, { target: { value: 'preserve paths' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
  }

  it('shows the saved pig naming model and sends it back', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, roles: { 'pig-title': { harness: 'pig', model: 'auto' } }, session_titles: true }, seen });
    const { container } = renderSettings();
    const model = await within(container).findByLabelText('Pig naming model');
    expect(model.textContent).toMatch(/auto/i);
    await dirtyAndSave(container);
    expect(seen.body.roles['pig-title']).toEqual({ harness: 'pig', model: 'auto' });
    expect(seen.body.session_titles).toBe(true);
  });

  it('leaves unconfigured roles out of the save', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, roles: { worker: { harness: 'claude', model: 'opus', effort: 'high' } } }, seen });
    const { container } = renderSettings();
    await dirtyAndSave(container);
    expect(Object.keys(seen.body.roles).sort()).toEqual(['pig-title', 'worker']);
  });

  it('no pig model clears the role and claude naming off is saved', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, session_titles: false }, seen });
    const { container } = renderSettings();
    await within(container).findByLabelText('Name claude Sessions');
    await dirtyAndSave(container);
    expect(seen.body.roles['pig-title']).toEqual({ harness: '', model: '' });
    expect(seen.body.session_titles).toBe(false);
  });

  it('an environment override locks the toggle and stays out of the save', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, session_titles: true, session_titles_from_env: true }, seen });
    const { container } = renderSettings();
    expect(await within(container).findByText(/EL_SESSION_TITLES/)).toBeTruthy();
    await dirtyAndSave(container);
    expect('session_titles' in seen.body).toBe(false);
  });
});

describe('SettingsPage context compression', () => {
  beforeEach(() => { section = 'context'; });
  it('shows saved values and sends the edited ones', async () => {
    const seen = {};
    stubSettings({
      settings: { ...base, compress: { auto_compress: false, compress_threshold_percent: 70, compress_threshold_tokens: 120000, compress_cooldown: 3, compress_instructions: 'keep ids' } },
      seen,
    });
    const { container } = renderSettings();
    const threshold = await within(container).findByLabelText('Threshold %');
    expect(threshold.value).toBe('70');
    expect(within(container).getByLabelText('Threshold tokens').value).toBe('120000');
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
      compress_threshold_tokens: 120000,
      compress_cooldown: 3,
      compress_instructions: 'keep paths',
      compress_model: '',
    });
  });

  it('uses the defaults when the Daemon sends none', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings();
    expect((await within(container).findByLabelText('Threshold %')).value).toBe('80');
    expect(within(container).getByLabelText('Threshold tokens').value).toBe('300000');
    expect(within(container).getByLabelText('Turns').value).toBe('5');
    expect(within(container).getByLabelText('Auto-compress').getAttribute('aria-checked')).toBe('true');
  });

  it('disables threshold and cooldown while auto-compress is off', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: false } } });
    const { container } = renderSettings();
    expect((await within(container).findByLabelText('Threshold %')).disabled).toBe(true);
    expect(within(container).getByLabelText('Threshold tokens').disabled).toBe(true);
    expect(within(container).getByLabelText('Turns').disabled).toBe(true);
  });
});

describe('SettingsPage compress model', () => {
  beforeEach(() => { section = 'context'; });
  it('shows the saved compress model and sends it back', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, compress: { compress_model: 'auto' } }, seen });
    const { container } = renderSettings();
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
    const { container } = renderSettings();
    await within(container).findByLabelText('Compress model');
    await fireEvent.input(within(container).getByLabelText('Summary focus'), { target: { value: 'keep paths' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.compress.compress_model).toBe('');
  });

  it('shows why Compress is off until a compress model is set', async () => {
    stubSettings({ settings: { ...base, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = renderSettings();
    expect(await within(container).findByText('no compress model: pick one in Settings')).toBeTruthy();
  });

  it('warns that auto-compress needs a compress model', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: true }, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = renderSettings();
    expect(await within(container).findByText('Auto-compress needs a compress model.')).toBeTruthy();
  });

  it('no warning while auto-compress is off', async () => {
    stubSettings({ settings: { ...base, compress: { auto_compress: false }, compress_note: 'no compress model: pick one in Settings' } });
    const { container } = renderSettings();
    await within(container).findByLabelText('Compress model');
    expect(within(container).queryByText('Auto-compress needs a compress model.')).toBeNull();
  });
});

describe('SettingsPage accounts', () => {
  beforeEach(() => { section = 'access'; });
  it('shows the sign-up switch to the admin and saves it', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, signup_enabled: true }, seen });
    const { container } = renderSettings({ props: { admin: true } });
    const toggle = await within(container).findByRole('switch', { name: 'Allow sign-up' });
    expect(toggle.getAttribute('aria-checked')).toBe('true');
    await fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-checked')).toBe('false');
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.signup_enabled).toBe(false);
  });

  it('shows access settings as admin-only to other Users', async () => {
    stubSettings({ settings: { ...base, signup_enabled: false } });
    const { container } = renderSettings();
    expect(await within(container).findByText('Only admins can change these settings.')).toBeTruthy();
    expect(within(container).queryByRole('switch', { name: 'Allow sign-up' })).toBeNull();
  });
});

describe('SettingsPage memory deciders', () => {
  beforeEach(() => { section = 'memory'; });
  it('shows saved values and sends the edited ones', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        decider: { decider_mode: 'ensemble', memory_model: 'auto', decider_confirm_low: 0.4, decider_confirm_high: 0.8 },
      },
      seen,
    });
    const { container } = renderSettings({ props: { admin: true } });
    expect(await within(container).findByLabelText('Decider mode')).toBeTruthy();
    expect(within(container).getByLabelText('Memory model').textContent).toMatch(/auto/i);
    expect(within(container).getByLabelText('Low').value).toBe('0.4');
    expect(within(container).getByLabelText('High').value).toBe('0.8');

    await fireEvent.input(within(container).getByLabelText('High'), { target: { value: '0.9' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.decider).toEqual({
      decider_mode: 'ensemble',
      memory_model: 'auto',
      decider_confirm_low: 0.4,
      decider_confirm_high: 0.9,
    });
  });

  it('uses the defaults when the Daemon sends none', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings({ props: { admin: true } });
    expect((await within(container).findByLabelText('Low')).value).toBe('0.35');
    expect(within(container).getByLabelText('High').value).toBe('0.7');
  });

  it('shows why confirmation falls back to the heuristic', async () => {
    stubSettings({ settings: { ...base, decider_note: 'no memory model: confirmation falls back to the heuristic' } });
    const { container } = renderSettings({ props: { admin: true } });
    expect(await within(container).findByText('no memory model: confirmation falls back to the heuristic')).toBeTruthy();
  });

  it('shows memory decider settings as admin-only to other Users', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings();
    expect(await within(container).findByText('Only admins can change these settings.')).toBeTruthy();
    expect(within(container).queryByTestId('decider-settings')).toBeNull();
  });
});

describe('SettingsPage jev decider', () => {
  beforeEach(() => { section = 'memory'; });
  it('shows saved values and sends the edited ones without the key', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        decider: { decider_mode: 'jev', memory_model: 'none', decider_confirm_low: 0.35, decider_confirm_high: 0.7 },
        jev: { base_url: 'http://localhost:8000/v1/systemone', model: 'jev-latest', api_key_set: true },
      },
      seen,
    });
    const { container } = renderSettings({ props: { admin: true } });
    expect((await within(container).findByLabelText('Endpoint URL')).value).toBe('http://localhost:8000/v1/systemone');
    expect(within(container).getByLabelText('Model').value).toBe('jev-latest');
    expect(await within(container).findByText(/leave this host/)).toBeTruthy();

    await fireEvent.input(within(container).getByLabelText('Endpoint URL'), {
      target: { value: 'https://jev-api.org/api/v1/decisions' },
    });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.jev).toEqual({
      base_url: 'https://jev-api.org/api/v1/decisions',
      model: 'jev-latest',
    });
  });

  it('sends a typed key and keeps it out of later saves', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, jev: { base_url: '', model: 'jev-latest', api_key_set: false } }, seen });
    const { container } = renderSettings({ props: { admin: true } });
    await fireEvent.input(await within(container).findByLabelText('API key'), { target: { value: 'k' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.jev.api_key).toBe('k');
  });

  it('keeps the Jev endpoint hidden from other Users', async () => {
    stubSettings({ settings: base });
    const { container } = renderSettings();
    expect(await within(container).findByText('Only admins can change these settings.')).toBeTruthy();
    expect(within(container).queryByLabelText('Endpoint URL')).toBeNull();
  });
});

describe('SettingsPage Run Chains', () => {
  beforeEach(() => { section = 'agents'; });
  it('edits each per-role Chain, including its order, and saves the result', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        chains: {
          worker: [{ backend: 'pig', model: 'auto' }, { backend: 'claude', model: 'opus' }],
          'reviewer-spec': [{ backend: 'claude', model: 'sonnet' }],
          'reviewer-standards': [{ backend: 'grok', model: 'grok-4.7' }],
        },
      },
      seen,
    });
    const { container } = renderSettings();
    const chains = await within(container).findByTestId('run-chains-settings');
    expect(within(chains).getByTestId('run-chain-worker')).toBeTruthy();
    expect(within(chains).getByTestId('run-chain-reviewer-spec')).toBeTruthy();
    expect(within(chains).getByTestId('run-chain-reviewer-standards')).toBeTruthy();
    expect(within(chains).getByLabelText('Worker Chain entry 1 Backend').textContent).toMatch(/pig/i);
    expect(within(chains).getByLabelText('Worker Chain entry 2 model').textContent).toMatch(/opus/i);

    await fireEvent.click(within(chains).getByLabelText('Move Worker Chain entry 2 up'));
    await fireEvent.click(within(chains).getByLabelText('Remove Worker Chain entry 2'));
    await fireEvent.click(within(chains).getByLabelText('Add Worker Chain entry'));
    await fireEvent.click(within(chains).getByLabelText('Worker Chain entry 2 Backend'));
    await fireEvent.click(await screen.findByRole('option', { name: 'Grok' }));
    await fireEvent.click(within(chains).getByLabelText('Worker Chain entry 2 model'));
    await fireEvent.click(await screen.findByRole('option', { name: /grok-4.7/i }));
    expect(within(chains).getByLabelText('Worker Chain entry 2 model').textContent).toMatch(/grok-4.7/i);
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.chains).toEqual({
      worker: [{ backend: 'claude', model: 'opus' }, { backend: 'grok', model: 'grok-4.7' }],
    });
  });

  it('uses existing role settings as the single-entry Chain fallback', async () => {
    stubSettings({
      settings: {
        ...base,
        roles: {
          implementer: { harness: 'pig', model: 'auto', effort: 'high' },
          'spec-review': { harness: 'claude', model: 'sonnet' },
          'code-review': { harness: 'grok', model: 'grok-4.7' },
        },
      },
    });
    const { container } = renderSettings();
    const chains = await within(container).findByTestId('run-chains-settings');
    expect(within(chains).getByLabelText('Worker Chain entry 1 model').textContent).toMatch(/auto/i);
    expect(within(chains).getByLabelText('Spec reviewer Chain entry 1 model').textContent).toMatch(/sonnet/i);
    expect(within(chains).getByLabelText('Standards reviewer Chain entry 1 model').textContent).toMatch(/grok-4.7/i);
    expect(within(chains).getAllByText(/Inherited from the role setting/)).toHaveLength(3);
  });

  it.each([
    ['worker', 'Worker'],
    ['implementer', 'Implementer'],
  ])('does not pin an inherited Chain when the %s role model changes', async (role, label) => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        roles: { [role]: { harness: 'claude', model: 'sonnet', effort: 'high' } },
      },
      seen,
    });
    const { container } = renderSettings();
    const chains = await within(container).findByTestId('run-chains-settings');
    expect(within(chains).getByLabelText('Worker Chain entry 1 model').textContent).toMatch(/sonnet/i);

    await fireEvent.click(within(container).getByLabelText(`${label} model`));
    await fireEvent.click(await screen.findByRole('option', { name: /opus/i }));
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.roles[role]).toMatchObject({ harness: 'claude', model: 'opus' });
    expect(seen.body.chains).toBeUndefined();
    expect(within(chains).getByLabelText('Worker Chain entry 1 model').textContent).toMatch(/opus/i);
  });

  it('preserves legacy Chain aliases during unrelated settings saves', async () => {
    const seen = {};
    stubSettings({
      settings: { ...base, chains: { implementer: [{ backend: 'claude', model: 'sonnet' }] } },
      seen,
    });
    const { container } = renderSettings();
    await within(container).findByTestId('run-chains-settings');
    await fireEvent.click(within(container).getByLabelText('Orchestrator model'));
    await fireEvent.click(await screen.findByRole('option', { name: /auto/i }));
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.chains).toBeUndefined();
  });

  it('clears a legacy alias only when its Chain is edited', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        chains: {
          implementer: [{ backend: 'claude', model: 'sonnet' }],
          'spec-review': [{ backend: 'claude', model: 'sonnet' }],
        },
      },
      seen,
    });
    const { container } = renderSettings();
    const chains = await within(container).findByTestId('run-chains-settings');
    await fireEvent.click(within(chains).getByLabelText('Remove Worker Chain entry 1'));
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.chains).toEqual({ worker: [], implementer: [] });
  });
});
