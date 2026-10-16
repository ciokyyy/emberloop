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

function stubSettings({ settings, failGet = false, failPut = false, failModels = false, seen = {} }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      if (url.includes('/api/models')) {
        if (failModels) return { ok: false, status: 503 };
        const harness = new URL(url, 'http://x').searchParams.get('harness');
        const models = harness === 'claude' ? ['opus'] : harness === 'pig' ? ['auto'] : ['auto'];
        const model_efforts = { opus: ['low', 'high'], 'auto': ['medium'], auto: ['low'] };
        return { ok: true, json: async () => ({ models, model_efforts }) };
      }
      if (url.includes('/api/settings') && opts?.method === 'PUT') {
        seen.body = JSON.parse(opts.body);
        if (failPut) return { ok: false, status: 400 };
        return { ok: true, json: async () => ({ ...seen.body, max_tickets: seen.body.max_tickets ?? 3, max_tickets_from_env: false }) };
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
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
    expect(input.value).toBe('2');
    expect(within(container).getByText(/Global max_tickets caps concurrent tickets in each Run/i)).toBeTruthy();
    expect(within(container).getByText(/A spec's max_tickets value can only lower this cap, never raise it/i)).toBeTruthy();
  });

  it('saves an edit with the roles', async () => {
    const seen = {};
    stubSettings({ settings: base, seen });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
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
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
    expect(input.disabled).toBe(true);
    expect(within(container).getByText(/EL_MAX_TICKETS/)).toBeTruthy();
  });

  it('surfaces a rejected value as an error', async () => {
    stubSettings({ settings: base, failPut: true });
    const { container } = renderSettings();
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
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
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
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
        const models = harness === 'claude' ? ['opus'] : harness === 'pig' ? ['auto'] : ['auto'];
        return { ok: true, json: async () => ({ models }) };
      }),
    );
    const { container } = renderSettings();
    expect(within(container).queryByRole('button', { name: 'Save' })).toBeNull();
    expect(within(container).queryByRole('button', { name: 'Revert' })).toBeNull();

    expect(resolveSettings).toBeDefined();
    resolveSettings();
    await within(container).findByLabelText('Maximum tickets per Run (spec)');
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
    const input = await within(container).findByLabelText('Maximum tickets per Run (spec)');
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

  it('preserves server-migrated role models in both merged chains', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        roles: { worker: { harness: 'claude', model: 'opus', effort: 'high' } },
        chains: {
          orchestrator: [],
          subagent: [{ backend: 'claude', model: 'opus', effort: 'high' }],
          scout: [],
          worker: [{ backend: 'claude', model: 'opus', effort: 'high' }],
          'reviewer-spec': [],
          'reviewer-standards': [],
        },
      },
      seen,
    });
    const { container } = renderSettings();
    await dirtyAndSave(container);
    expect(seen.body.chains.subagent).toEqual([{ backend: 'claude', model: 'opus', effort: 'high' }]);
    expect(seen.body.chains.worker).toEqual([{ backend: 'claude', model: 'opus', effort: 'high' }]);
    expect(seen.body.roles.worker).toEqual({ harness: 'claude', model: 'opus', effort: 'high' });
    expect(seen.body.roles.implementer).toEqual({ harness: 'claude', model: 'opus', effort: 'high' });
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
    const modes = await within(container).findByRole('group', { name: 'Decider mode' });
    const ensemble = within(modes).getByRole('button', { name: 'Ensemble' });
    expect(ensemble.getAttribute('aria-pressed')).toBe('true');
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
    await fireEvent.click(within(modes).getByRole('button', { name: 'Jev' }));
    expect(within(modes).getByRole('button', { name: 'Jev' }).getAttribute('aria-pressed')).toBe('true');
    expect(ensemble.getAttribute('aria-pressed')).toBe('false');
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.decider.decider_mode).toBe('jev');
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
    const endpoint = await within(container).findByRole('button', { name: /Custom Jev endpoint/ });
    expect(endpoint.getAttribute('aria-expanded')).toBe('true');
    expect(within(endpoint).getByText(/In Jev mode, leave blank to use OpenRouter/i)).toBeTruthy();
    const endpointPanel = container.querySelector('#custom-jev-settings');
    expect(endpointPanel.hidden).toBe(false);
    expect((await within(endpointPanel).findByLabelText('Endpoint URL')).value).toBe('http://localhost:8000/v1/systemone');
    expect(within(endpointPanel).getByLabelText('Model').value).toBe('jev-latest');
    expect(await within(endpointPanel).findByText(/leave this host/)).toBeTruthy();

    await fireEvent.input(within(endpointPanel).getByLabelText('Endpoint URL'), {
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

  it('keeps a blank custom endpoint collapsed and explains the Jev-mode OpenRouter default', async () => {
    stubSettings({ settings: { ...base, jev: { base_url: '', model: 'jev-latest' } } });
    const { container } = renderSettings({ props: { admin: true } });
    const endpoint = await within(container).findByRole('button', { name: /Custom Jev endpoint/ });
    expect(endpoint.getAttribute('aria-expanded')).toBe('false');
    expect(container.querySelector('#custom-jev-settings').hidden).toBe(true);
    expect(within(endpoint).getByText(/In Jev mode, leave blank to use OpenRouter/i)).toBeTruthy();
  });

  it('saves a blank URL to use the OpenRouter default', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, jev: { base_url: 'http://localhost:8000/v1/systemone', model: 'jev-latest' } }, seen });
    const { container } = renderSettings({ props: { admin: true } });
    const endpoint = await within(container).findByRole('button', { name: /Custom Jev endpoint/ });
    expect(endpoint.getAttribute('aria-expanded')).toBe('true');
    await fireEvent.input(within(container).getByLabelText('Endpoint URL'), { target: { value: '' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(seen.body.jev.base_url).toBe('');
  });

  it('sends a typed key and keeps it out of later saves', async () => {
    const seen = {};
    stubSettings({ settings: { ...base, jev: { base_url: 'http://localhost:8000/v1/systemone', model: 'jev-latest', api_key_set: false } }, seen });
    const { container } = renderSettings({ props: { admin: true } });
    const endpoint = await within(container).findByRole('button', { name: /Custom Jev endpoint/ });
    expect(endpoint.getAttribute('aria-expanded')).toBe('true');
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

describe('SettingsPage agent chains', () => {
  beforeEach(() => { section = 'agents'; });

  it('edits ordered role chains and offers only the selected model effort levels', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        chains: {
          worker: [{ backend: 'pig', model: 'auto', effort: 'low' }, { backend: 'claude', model: 'opus', effort: 'high' }],
          'reviewer-spec': [{ backend: 'claude', model: 'opus', effort: 'low' }],
        },
      },
      seen,
    });
    const { container } = renderSettings();
    const agents = await within(container).findByTestId('agent-chains-settings');
    expect(within(agents).getByText('Session agents')).toBeTruthy();
    expect(within(agents).getByText('Run agents')).toBeTruthy();
    expect(within(agents).getByTestId('agent-chain-subagent')).toBeTruthy();
    expect(within(agents).getByTestId('agent-chain-worker')).toBeTruthy();
    expect(within(agents).getAllByText('Primary').length).toBeGreaterThan(0);
    expect(within(agents).getAllByText('Fallback 1').length).toBeGreaterThan(0);

    await fireEvent.click(within(agents).getByLabelText('Ticket worker Primary effort'));
    expect(await screen.findByRole('option', { name: 'Default' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Low' })).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'Medium' })).toBeNull();
    expect(screen.queryByRole('option', { name: 'High' })).toBeNull();
    await fireEvent.click(screen.getByRole('option', { name: 'Low' }));

    await fireEvent.click(within(agents).getByLabelText('Move Ticket worker Fallback 1 up'));
    await fireEvent.click(within(agents).getByLabelText('Remove Ticket worker Fallback 1'));
    await fireEvent.click(within(agents).getByLabelText('Add Ticket worker entry'));
    await fireEvent.click(within(agents).getByLabelText('Ticket worker Fallback 1 harness'));
    await fireEvent.click(await screen.findByRole('option', { name: 'Pig' }));
    await fireEvent.click(within(agents).getByLabelText('Ticket worker Fallback 1 model'));
    await fireEvent.click(await screen.findByRole('option', { name: /auto/i }));
    await fireEvent.click(within(agents).getByLabelText('Ticket worker Fallback 1 effort'));
    expect(await screen.findByRole('option', { name: 'Medium' })).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'Low' })).toBeNull();
    expect(screen.queryByRole('option', { name: 'High' })).toBeNull();
    await fireEvent.click(screen.getByRole('option', { name: 'Medium' }));

    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.chains.worker).toEqual([
      { backend: 'claude', model: 'opus', effort: 'high' },
      { backend: 'pig', model: 'auto', effort: 'medium' },
    ]);
    expect(seen.body.roles.implementer).toEqual({ harness: 'claude', model: 'opus', effort: 'high' });
  });

  it('renders canonical chains returned by the server alongside legacy role keys', async () => {
    stubSettings({
      settings: {
        ...base,
        roles: {
          orchestrator: { harness: 'claude', model: 'opus', effort: 'high' },
          worker: { harness: 'pig', model: 'auto', effort: 'low' },
          scout: { harness: 'pig', model: 'auto', effort: 'medium' },
          implementer: { harness: 'claude', model: 'opus', effort: 'high' },
          'spec-review': { harness: 'claude', model: 'opus', effort: 'low' },
          'code-review': { harness: 'pig', model: 'auto', effort: 'medium' },
        },
        chains: {
          orchestrator: [{ backend: 'claude', model: 'opus', effort: 'high' }],
          subagent: [{ backend: 'pig', model: 'auto', effort: 'low' }],
          scout: [{ backend: 'pig', model: 'auto', effort: 'medium' }],
          worker: [{ backend: 'claude', model: 'opus', effort: 'high' }],
          'reviewer-spec': [{ backend: 'claude', model: 'opus', effort: 'low' }],
          'reviewer-standards': [{ backend: 'pig', model: 'auto', effort: 'medium' }],
        },
      },
    });
    const { container } = renderSettings();
    const agents = await within(container).findByTestId('agent-chains-settings');
    for (const [role, model] of [
      ['Orchestrator', 'opus'],
      ['Subagent', 'auto'],
      ['Scout', 'auto'],
      ['Ticket worker', 'opus'],
      ['Spec reviewer', 'opus'],
      ['Standards reviewer', 'auto'],
    ]) {
      expect(within(agents).getByLabelText(`${role} Primary model`).textContent).toMatch(new RegExp(model.replace('.', '\\.'), 'i'));
    }
  });

  it('limits effort choices to Default when the model catalogue cannot be loaded', async () => {
    stubSettings({
      failModels: true,
      settings: { ...base, chains: { worker: [{ backend: 'claude', model: 'opus', effort: 'high' }] } },
    });
    const { container } = renderSettings();
    const agents = await within(container).findByTestId('agent-chains-settings');
    await fireEvent.click(within(agents).getByLabelText('Ticket worker Primary effort'));
    expect(await screen.findByRole('option', { name: 'Default' })).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'High' })).toBeNull();
  });

  it('caps each role at a primary and two fallbacks', async () => {
    stubSettings({
      settings: {
        ...base,
        chains: {
          worker: [
            { backend: 'pig', model: 'auto', effort: 'low' },
            { backend: 'claude', model: 'opus', effort: 'high' },
            { backend: 'pig', model: 'auto', effort: 'medium' },
          ],
        },
      },
    });
    const { container } = renderSettings();
    const agents = await within(container).findByTestId('agent-chains-settings');
    expect(within(agents).getByText('Fallback 2')).toBeTruthy();
    expect(within(agents).getByLabelText('Add Ticket worker entry').disabled).toBe(true);
  });
});

describe('SettingsPage automatic Memory settings', () => {
  beforeEach(() => { section = 'memory'; });

  it('loads, edits and saves the host-global extraction, embedding and recall controls', async () => {
    const seen = {};
    const settings = { ...base, memory_extract_model: 'extract/model', memory_extract_effort: 'low', memory_embedding_model: 'embed/model', memory_auto_extract: false, memory_recall_weights: { vector: 0.4, fts: 0.4, importance: 0.2 } };
    stubSettings({ settings, seen });
    const { container } = renderSettings({ props: { admin: true } });
    await within(container).findByRole('switch', { name: 'Automatic memory extraction' });
    const extractor = container.querySelector('#memory-extract-model');
    expect(extractor).not.toBeNull();
    await fireEvent.input(extractor, { target: { value: 'new/extractor' } });
    await fireEvent.input(within(container).getByLabelText('Vector'), { target: { value: '0.2' } });
    await fireEvent.click(within(container).getByRole('switch', { name: 'Automatic memory extraction' }));
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.memory_extract_model).toBe('new/extractor');
    expect(seen.body.memory_embedding_model).toBe('embed/model');
    expect(seen.body.memory_auto_extract).toBe(true);
    expect(seen.body.memory_recall_weights).toEqual({ vector: 0.2, fts: 0.4, importance: 0.2 });
  });
});

describe('SettingsPage find decision model', () => {
  beforeEach(() => { section = 'memory'; });
  it('shows the saved model and sends the picked one', async () => {
    const seen = {};
    stubSettings({
      settings: {
        ...base,
        find: { decision_model: '', default: 'typesafe/jev-1.13', from_env: false, suggestions: ['typesafe/jev-1.13', 'liquid/d1'] },
      },
      seen,
    });
    const { container } = renderSettings({ props: { admin: true } });
    const panel = await within(container).findByTestId('find-settings');
    expect(within(panel).getByLabelText('Model').placeholder).toBe('typesafe/jev-1.13');
    await fireEvent.click(within(panel).getByRole('button', { name: 'liquid/d1' }));
    await tick();
    await fireEvent.click(await within(container).findByText('Save'));
    expect(await within(container).findByText('Saved')).toBeTruthy();
    expect(seen.body.find).toEqual({ decision_model: 'liquid/d1' });
  });

  it('locks the field and omits it from the save when the environment sets it', async () => {
    const seen = {};
    stubSettings({
      settings: { ...base, find: { decision_model: '', default: 'typesafe/jev-1.13', from_env: true, suggestions: ['liquid/d1'] }, decider: { decider_mode: 'heuristic' } },
      seen,
    });
    const { container } = renderSettings({ props: { admin: true } });
    const panel = await within(container).findByTestId('find-settings');
    expect(within(panel).getByLabelText('Model').disabled).toBe(true);
    expect(within(panel).getByText(/EL_DECISION_MODEL/)).toBeTruthy();
    expect(within(panel).queryByRole('button', { name: 'liquid/d1' })).toBeNull();
  });
});
