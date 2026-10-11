import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import ProfileSettings from './ProfileSettings.svelte';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubProfile({ profile, versions = [], queue = [], seen = {}, failPut = false }) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (u, opts) => {
      const url = String(u);
      const method = opts?.method ?? 'GET';
      if (url.includes('/api/profile/queue/') && url.endsWith('/decide') && method === 'POST') {
        seen.decide = JSON.parse(opts.body);
        if (failPut) return { ok: false, status: 400 };
        return { ok: true, json: async () => ({ version: 2, lines: [], rendered: '' }) };
      }
      if (url.includes('/api/profile/queue') && method === 'GET') {
        return { ok: true, json: async () => queue };
      }
      if (url.includes('/api/profile/versions') && method === 'GET') {
        return { ok: true, json: async () => versions };
      }
      if (url.includes('/api/profile/restore') && method === 'POST') {
        seen.restore = JSON.parse(opts.body);
        return { ok: true, json: async () => ({ version: 3, lines: [], rendered: '' }) };
      }
      if (url.includes('/api/profile') && method === 'PUT') {
        seen.put = JSON.parse(opts.body);
        if (failPut) return { ok: false, status: 400 };
        return { ok: true, json: async () => ({ version: 1, lines: [], rendered: '' }) };
      }
      if (url.includes('/api/profile') && method === 'GET') {
        return { ok: true, json: async () => profile };
      }
      return { ok: true, json: async () => ({}) };
    }),
  );
}

const baseProfile = { version: 1, lines: [{ id: 'l1', text: 'use tabs', tag: 'workflow' }], rendered: 'use tabs' };

describe('ProfileSettings', () => {
  it('lists the saved lines', async () => {
    stubProfile({ profile: baseProfile });
    const { container } = render(ProfileSettings);
    expect(await within(container).findByText('use tabs')).toBeTruthy();
    expect(within(container).getByText(/workflow/)).toBeTruthy();
  });

  it('shows the empty state with no lines', async () => {
    stubProfile({ profile: { version: 0, lines: [], rendered: '' } });
    const { container } = render(ProfileSettings);
    expect(await within(container).findByText(/No profile yet/)).toBeTruthy();
  });

  it('adds a line through the settings view', async () => {
    const seen = {};
    stubProfile({ profile: baseProfile, seen });
    const { container } = render(ProfileSettings);
    await within(container).findByText('use tabs');
    const input = await within(container).findByLabelText('New line');
    await fireEvent.input(input, { target: { value: 'short replies' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Add line'));
    await within(container).findByText('Profile saved.');
    expect(seen.put.ops).toEqual([{ op: 'add', text: 'short replies', tag: 'workflow' }]);
  });

  it('retires a line', async () => {
    const seen = {};
    stubProfile({ profile: baseProfile, seen });
    const { container } = render(ProfileSettings);
    await fireEvent.click(await within(container).findByRole('button', { name: 'Retire l1' }));
    await within(container).findByText(/Line retired/);
    expect(seen.put.ops).toEqual([{ op: 'retire', id: 'l1' }]);
  });

  it('approves and rejects queue items', async () => {
    const seen = {};
    const queue = [{ id: 7, kind: 'profile_patch', payload: '{"ops":[{"op":"add"}]}', status: 'pending' }];
    stubProfile({ profile: baseProfile, queue, seen });
    const { container } = render(ProfileSettings);
    await within(container).findByText(/#7/);
    await fireEvent.click(await within(container).findByText('Approve'));
    await within(container).findByText('Patch approved.');
    expect(seen.decide.status).toBe('approved');
  });

  it('restores a version as a new version', async () => {
    const seen = {};
    stubProfile({
      profile: baseProfile,
      versions: [{ version: 1, lines: baseProfile.lines }],
      seen,
    });
    const { container } = render(ProfileSettings);
    await fireEvent.click(await within(container).findByRole('button', { name: 'Restore version 1' }));
    await within(container).findByText(/Restored version 1/);
    expect(seen.restore).toEqual({ version: 1 });
  });

  it('surfaces a rejected save as an error', async () => {
    stubProfile({ profile: baseProfile, failPut: true });
    const { container } = render(ProfileSettings);
    await within(container).findByText('use tabs');
    const input = await within(container).findByLabelText('New line');
    await fireEvent.input(input, { target: { value: 'another' } });
    await tick();
    await fireEvent.click(await within(container).findByText('Add line'));
    expect(await within(container).findByRole('alert')).toBeTruthy();
  });
});
