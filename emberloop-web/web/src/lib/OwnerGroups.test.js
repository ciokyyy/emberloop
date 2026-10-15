import { describe, it, expect, afterEach } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
import OwnerGroups from './OwnerGroups.svelte';

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const users = [
  { id: 'u-cioky', username: 'cioky', admin: true, online: true },
  { id: 'u-mira', username: 'mira', admin: false, online: true },
];
const me = { id: 'u-cioky', username: 'cioky', admin: true };
const projects = [
  { id: 'p1', name: 'emberloop', path: '/src/emberloop' },
  { id: 'p2', name: 'dotfiles', path: '/src/dotfiles' },
];
const sessions = [
  { id: 'a1', owner: 'u-cioky', harness: 'claude', status: 'idle', title: 'Own chat', projectId: 'p1' },
  { id: 'm1', owner: 'u-mira', harness: 'pig', status: 'working', title: 'Mira chat', projectId: 'p2' },
];
const props = { users, me, sessions, runs: [], projects, currentId: null, activeProjectId: null, query: '' };

const group = (container, owner) => container.querySelector(`[data-owner="${owner}"]`);
const headerOf = (container, owner) => group(container, owner).querySelector('button');
const setStored = (key, value) => localStorage.setItem(key, JSON.stringify(value));

describe('OwnerGroups grouping', () => {
  it('opens the own group first and collapses the others with a count and an online dot', () => {
    setStored('eb.owner-filter', 'all');
    const { container } = render(OwnerGroups, { props });
    expect([...container.querySelectorAll('[data-owner]')].map((s) => s.dataset.owner)).toEqual(['u-cioky', 'u-mira']);
    expect(headerOf(container, 'u-cioky').getAttribute('aria-expanded')).toBe('true');
    expect(headerOf(container, 'u-mira').getAttribute('aria-expanded')).toBe('false');
    expect(within(group(container, 'u-mira')).getByTestId('owner-count').textContent).toBe('1');
    expect(within(group(container, 'u-mira')).getByTestId('online-dot')).toBeTruthy();
    expect(within(group(container, 'u-mira')).queryByText('Mira chat')).toBeNull();
  });

  it('gives each group its User color on the left bar and the initial on the avatar', () => {
    setStored('eb.owner-filter', 'all');
    setStored('eb.owners-collapsed', { 'u-mira': false });
    const { container } = render(OwnerGroups, { props });
    expect(group(container, 'u-mira').querySelector('.border-l-2').getAttribute('style')).toContain('var(--owner-2)');
    expect(within(headerOf(container, 'u-cioky')).getByTestId('avatar').textContent?.trim()).toBe('C');
  });

  it('lists every Project in the own group and only the Projects with rows in the others', () => {
    setStored('eb.owner-filter', 'all');
    setStored('eb.owners-collapsed', { 'u-mira': false });
    const { container } = render(OwnerGroups, { props });
    const sectionNames = (owner) =>
      [...group(container, owner).querySelectorAll('section[aria-label]')].map((s) => s.getAttribute('aria-label'));
    expect(sectionNames('u-cioky')).toEqual(['emberloop', 'dotfiles']);
    expect(sectionNames('u-mira')).toEqual(['dotfiles']);
  });

  it('toggles a group on its header and keeps the state', async () => {
    setStored('eb.owner-filter', 'all');
    const { container } = render(OwnerGroups, { props });
    await fireEvent.click(headerOf(container, 'u-mira'));
    expect(headerOf(container, 'u-mira').getAttribute('aria-expanded')).toBe('true');
    expect(JSON.parse(localStorage.getItem('eb.owners-collapsed'))).toEqual({ 'u-mira': false });
  });
});

describe('OwnerGroups filters', () => {
  it('shows only the own group on Mine, and every group on Everyone, remembering the choice', async () => {
    const { container, getByRole } = render(OwnerGroups, { props });
    expect(getByRole('button', { name: 'Mine' }).getAttribute('aria-pressed')).toBe('true');
    expect(container.querySelectorAll('[data-owner]')).toHaveLength(1);
    await fireEvent.click(getByRole('button', { name: 'Everyone' }));
    expect(container.querySelectorAll('[data-owner]')).toHaveLength(2);
    expect(getByRole('button', { name: 'Everyone' }).getAttribute('aria-pressed')).toBe('true');
    expect(JSON.parse(localStorage.getItem('eb.owner-filter'))).toBe('all');
  });

  it('keeps only the chosen Project on the Project filter', () => {
    setStored('eb.owner-filter', 'project');
    setStored('eb.owner-project', 'p2');
    setStored('eb.owners-collapsed', { 'u-mira': false });
    const { container, queryByText, getByText, getByLabelText } = render(OwnerGroups, { props });
    expect(queryByText('Own chat')).toBeNull();
    expect(getByText('Mira chat')).toBeTruthy();
    expect(getByLabelText('Project filter').textContent).toContain('dotfiles');
    expect(container.querySelectorAll('[data-owner]')).toHaveLength(2);
  });


  it('scopes Sessions to one Project without rendering per-Project sections or its filter', () => {
    setStored('eb.owner-filter', 'all');
    const { container, getByText, queryByText, queryByLabelText } = render(OwnerGroups, {
      props: { ...props, scoped: true, scopeProjectId: 'p1' },
    });
    expect(getByText('Own chat')).toBeTruthy();
    expect(queryByText('Mira chat')).toBeNull();
    expect(queryByLabelText('Project filter')).toBeNull();
    expect(container.querySelector('section[aria-label="emberloop"]')).toBeNull();
  });
  it('matches the chat query, opens the matching groups and shows an empty line when nothing matches', async () => {
    setStored('eb.owner-filter', 'all');
    const view = render(OwnerGroups, { props });
    await view.rerender({ ...props, query: 'mira chat' });
    expect(view.container.querySelectorAll('[data-owner]')).toHaveLength(1);
    expect(view.getByText('Mira chat')).toBeTruthy();
    await view.rerender({ ...props, query: 'zzz' });
    expect(view.getByText('No chats match “zzz”.')).toBeTruthy();
  });
});
