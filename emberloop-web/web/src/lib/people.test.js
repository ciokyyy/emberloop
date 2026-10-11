import { describe, it, expect, vi, afterEach } from 'vitest';
import { people, refreshPeople, personFor } from './people.svelte.js';

afterEach(() => {
  vi.unstubAllGlobals();
  people.list = [];
});

describe('people store', () => {
  it('loads the account list from GET /api/users and finds a User by id', async () => {
    const fetchMock = vi.fn(async () => ({ ok: true, json: async () => [{ id: 'u1', username: 'cioky', online: true }] }));
    vi.stubGlobal('fetch', fetchMock);
    await refreshPeople();
    expect(String(fetchMock.mock.calls[0][0])).toContain('/api/users');
    expect(personFor('u1')).toMatchObject({ username: 'cioky', online: true });
    expect(personFor('nobody')).toBeNull();
    expect(personFor('')).toBeNull();
  });

  it('keeps the last good list when a refresh fails', async () => {
    people.list = [{ id: 'u1', username: 'cioky' }];
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: false, status: 500, json: async () => ({}) })));
    await refreshPeople();
    expect(personFor('u1')?.username).toBe('cioky');
  });
});
