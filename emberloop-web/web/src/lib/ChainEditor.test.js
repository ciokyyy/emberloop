import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import ChainEditor from './ChainEditor.svelte';

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const role = { id: 'worker', label: 'Ticket worker', hint: 'Implements a ticket.' };
const harnessItems = [{ value: 'claude', label: 'Claude' }, { value: 'pig', label: 'Pig' }];

function props(extra = {}) {
  return {
    role,
    entries: [{ backend: 'claude', model: 'opus', effort: 'high' }, { backend: 'pig', model: 'auto', effort: '' }],
    harnessItems,
    itemsFor: (_, model) => [{ value: model, label: model }],
    effortItemsFor: () => [{ value: '', label: 'Default' }, { value: 'high', label: 'High' }],
    onadd: vi.fn(),
    onremove: vi.fn(),
    onmove: vi.fn(),
    onbackend: vi.fn(),
    onmodel: vi.fn(),
    oneffort: vi.fn(),
    ...extra,
  };
}

function stubPhone(matches) {
  vi.stubGlobal('matchMedia', (query) => ({ matches, media: query, addEventListener() {}, removeEventListener() {} }));
}

describe('ChainEditor on desktop', () => {
  beforeEach(() => stubPhone(false));
  it('edits every entry inline', () => {
    render(ChainEditor, { props: props() });
    expect(screen.getByLabelText('Ticket worker Primary harness')).toBeTruthy();
    expect(screen.getByLabelText('Ticket worker Fallback 1 model')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Edit Ticket worker Primary' })).toBeNull();
  });
  it('adds, moves and removes through icon buttons', async () => {
    const p = props();
    render(ChainEditor, { props: p });
    await fireEvent.click(screen.getByRole('button', { name: 'Add Ticket worker entry' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Move Ticket worker Fallback 1 up' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Remove Ticket worker Fallback 1' }));
    expect(p.onadd).toHaveBeenCalled();
    expect(p.onmove).toHaveBeenCalledWith(1, -1);
    expect(p.onremove).toHaveBeenCalledWith(1);
  });
  it('disables Add at three entries', () => {
    const entries = [1, 2, 3].map(() => ({ backend: 'pig', model: 'auto', effort: '' }));
    render(ChainEditor, { props: props({ entries }) });
    expect(screen.getByRole('button', { name: 'Add Ticket worker entry' }).disabled).toBe(true);
  });
});

describe('ChainEditor on phone', () => {
  beforeEach(() => stubPhone(true));
  it('lists entries and opens a sheet per entry', async () => {
    const p = props();
    render(ChainEditor, { props: p });
    const primary = await screen.findByRole('button', { name: 'Edit Ticket worker Primary' });
    expect(primary.textContent).toContain('opus');
    expect(primary.textContent).toContain('High');
    expect(screen.queryByLabelText('Ticket worker Primary harness')).toBeNull();

    await fireEvent.click(primary);
    const sheet = await screen.findByRole('dialog', { name: 'Ticket worker · Primary' });
    expect(within(sheet).getByLabelText('Ticket worker Primary harness')).toBeTruthy();
    expect(within(sheet).getByLabelText('Ticket worker Primary model')).toBeTruthy();
    expect(within(sheet).getByLabelText('Ticket worker Primary effort')).toBeTruthy();

    await fireEvent.click(within(sheet).getByRole('button', { name: 'Remove Ticket worker Primary' }));
    expect(p.onremove).toHaveBeenCalledWith(0);
    expect(screen.queryByRole('dialog')).toBeNull();
  });
  it('keeps the sheet on the entry it moves', async () => {
    const p = props();
    p.onmove = vi.fn();
    render(ChainEditor, { props: p });
    await fireEvent.click(await screen.findByRole('button', { name: 'Edit Ticket worker Primary' }));
    const sheet = await screen.findByRole('dialog', { name: 'Ticket worker · Primary' });
    await fireEvent.click(within(sheet).getByRole('button', { name: 'Move Ticket worker Primary down' }));
    expect(p.onmove).toHaveBeenCalledWith(0, 1);
    expect(await screen.findByRole('dialog', { name: 'Ticket worker · Fallback 1' })).toBeTruthy();
  });
});
