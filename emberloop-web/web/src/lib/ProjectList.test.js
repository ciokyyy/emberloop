import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, within, fireEvent, cleanup } from '@testing-library/svelte';
import ProjectList from './ProjectList.svelte';

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const sections = [
  { key: 'p1', name: 'emberloop', path: '/src/emberloop', roots: [{ id: 'a1', harness: 'claude', status: 'idle', title: 'Fix sidebar' }] },
  { key: '', name: 'Other', path: '', roots: [{ id: 'b1', harness: 'pig', status: 'idle', title: 'Loose chat' }] },
];

function mount(props = {}) {
  return render(ProjectList, { props: { ownerId: 'u1', sections, ...props } });
}

// The collapsible header of the emberloop section: its name is a plain span.
const header = (view) => view.getByText('emberloop').closest('button');

describe('ProjectList sections', () => {
  it('shows a header per Project with its rows open by default', () => {
    const view = mount();
    expect(header(view).getAttribute('aria-expanded')).toBe('true');
    expect(view.getByText('Fix sidebar')).toBeTruthy();
    expect(view.getByText('Loose chat')).toBeTruthy();
  });

  it('collapses a Project on its header, hides its rows and keeps the state', async () => {
    const view = mount();
    await fireEvent.click(header(view));
    expect(view.queryByText('Fix sidebar')).toBeNull();
    expect(JSON.parse(localStorage.getItem('eb.projects-collapsed'))).toEqual({ 'u1/p1': true });
  });

  it('opens every section while a chat filter runs, even collapsed ones', async () => {
    const view = mount();
    await fireEvent.click(header(view));
    expect(view.queryByText('Fix sidebar')).toBeNull();
    await view.rerender({ ownerId: 'u1', sections, force: true });
    expect(view.queryByText('Fix sidebar')).not.toBeNull();
  });

  it('shows the row count on a collapsed Project header', async () => {
    const view = mount();
    await fireEvent.click(header(view));
    expect(within(header(view)).getByText('1')).toBeTruthy();
  });

  it('starts a draft chat in a Project with the plus button', async () => {
    const onNew = vi.fn();
    const view = mount({ onNew });
    await fireEvent.click(view.getByRole('button', { name: 'New chat in emberloop' }));
    expect(onNew).toHaveBeenCalledWith('p1');
  });

  it('renames and removes real Projects only, never Other', () => {
    const view = mount({ onRename: vi.fn(), onRemove: vi.fn() });
    expect(view.getByRole('button', { name: 'Rename emberloop' })).toBeTruthy();
    expect(view.getByRole('button', { name: 'Remove emberloop' })).toBeTruthy();
    expect(view.queryByRole('button', { name: 'Rename Other' })).toBeNull();
    expect(view.queryByRole('button', { name: 'New chat in Other' })).toBeNull();
  });

  it('calls onRename with the Project, not the section', async () => {
    const onRename = vi.fn();
    const view = mount({ onRename });
    await fireEvent.click(view.getByRole('button', { name: 'Rename emberloop' }));
    expect(onRename).toHaveBeenCalledWith({ id: 'p1', name: 'emberloop', path: '/src/emberloop' });
  });
});
