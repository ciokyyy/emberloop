import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import ChatMenu from './ChatMenu.svelte';
import { resetBackStack } from './overlayBack.js';

afterEach(() => {
  cleanup();
  resetBackStack();
  vi.restoreAllMocks();
});

const git = { isRepo: true, branch: 'main', files: ['a.txt', 'b.txt'] };

function mount(overrides = {}) {
  const props = {
    model: 'Opus 4.5',
    context: '15% context used',
    projectName: 'Demo',
    projects: [{ id: 'project-one', name: 'Demo' }],
    selectedId: 'project-one',
    git,
    onNewSession: vi.fn(),
    onOpenGit: vi.fn(),
    onOpenContext: vi.fn(),
    onSelectScope: vi.fn(),
    ...overrides,
  };
  return { props, ...render(ChatMenu, { props }) };
}

describe('ChatMenu', () => {
  it('stays one icon button until it opens the Session sheet', async () => {
    mount();
    const trigger = screen.getByRole('button', { name: 'Session menu' });
    expect(trigger.textContent.trim()).toBe('');
    expect(trigger.className).toContain('md:hidden');

    await fireEvent.click(trigger);
    const sheet = await screen.findByRole('dialog', { name: 'Session' });
    for (const text of ['New Session', 'main · 2 changed', 'Opus 4.5', '15% context used', 'Demo', 'Switch Project']) {
      expect(sheet.textContent).toContain(text);
    }
  });

  it('runs the Session actions and hands the back entry to the next sheet', async () => {
    const { props } = mount();
    await fireEvent.click(screen.getByRole('button', { name: 'Session menu' }));
    // A navigation closes the sheet through the router, not here.
    await fireEvent.click(await screen.findByRole('button', { name: /New Session/ }));
    expect(props.onNewSession).toHaveBeenCalledTimes(1);

    await fireEvent.click(screen.getByRole('button', { name: 'Session menu' }));
    await fireEvent.click(await screen.findByRole('button', { name: /15% context used/ }));
    expect(props.onOpenContext).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('dialog', { name: 'Session' })).toBeNull();
  });

  it('opens Git and Project from the same menu', async () => {
    const { props } = mount();
    await fireEvent.click(screen.getByRole('button', { name: 'Session menu' }));
    await fireEvent.click(await screen.findByRole('button', { name: /Git/ }));
    expect(props.onOpenGit).toHaveBeenCalledTimes(1);

    await fireEvent.click(screen.getByRole('button', { name: 'Session menu' }));
    await fireEvent.click(await screen.findByRole('button', { name: /Switch Project/ }));
    const project = await screen.findByRole('dialog', { name: 'Project' });
    expect(within(project).getByRole('option', { name: 'No project' })).toBeTruthy();
    expect(within(project).getByRole('button', { name: 'Back to Session' })).toBeTruthy();
    await fireEvent.click(within(project).getByRole('option', { name: 'All projects' }));
    expect(props.onSelectScope).toHaveBeenCalledWith('__all_projects__');
  });

  it('closes on system Back', async () => {
    mount();
    await fireEvent.click(screen.getByRole('button', { name: 'Session menu' }));
    expect(await screen.findByRole('dialog', { name: 'Session' })).toBeTruthy();
    window.dispatchEvent(new PopStateEvent('popstate'));
    await vi.waitFor(() => expect(screen.queryByRole('dialog', { name: 'Session' })).toBeNull());
  });
});
