import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import { tick } from 'svelte';
import Combobox from './combobox.svelte';

// jsdom has no layout: the TanStack virtualizer reads offsetWidth/offsetHeight
// (zero there, which legitimately means "no window"), so give every element a
// viewport and noop ResizeObserver. Rows then render off the estimated height.
beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
  vi.spyOn(HTMLElement.prototype, 'offsetHeight', 'get').mockReturnValue(256);
  vi.spyOn(HTMLElement.prototype, 'offsetWidth', 'get').mockReturnValue(320);
  // bits-ui scrolls the highlighted option into view; jsdom has no such method.
  if (!Element.prototype.scrollIntoView) Element.prototype.scrollIntoView = () => {};
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const MANY = [
  { value: 'alpha', label: 'Alpha' },
  { value: 'beta', label: 'Beta' },
  { value: 'gamma', label: 'Gamma' },
  { value: 'delta', label: 'Delta' },
  { value: 'epsilon', label: 'Epsilon' },
  { value: 'zeta', label: 'Zeta' },
  { value: 'eta', label: 'Eta' },
  { value: 'theta', label: 'Theta' },
  { value: 'iota', label: 'Iota' },
  { value: 'kappa', label: 'Kappa' },
  { value: 'lambda', label: 'Lambda' },
  { value: 'mu', label: 'Mu' },
];

const FEW = [
  { value: 'on', label: 'On' },
  { value: 'off', label: 'Off' },
];

async function openCombobox(props) {
  const onValueChange = vi.fn();
  render(Combobox, { props: { items: MANY, placeholder: 'Pick one', onValueChange, 'aria-label': 'Choice', ...props } });
  await fireEvent.click(await screen.findByLabelText('Choice'));
  return { onValueChange };
}

describe('combobox Command picker', () => {
  it('shows the placeholder, then the search and options on open', async () => {
    const { onValueChange } = await openCombobox({});
    expect(screen.getByLabelText('Choice').textContent).toMatch(/pick one/i);
    expect(await screen.findByLabelText('Search options')).toBeTruthy();
    const options = await screen.findAllByRole('option');
    expect(options.length).toBeGreaterThan(0);
    expect(options[0].textContent).toMatch(/alpha/i);
    expect(onValueChange).not.toHaveBeenCalled();
  });

  it('filters by label and picks through onValueChange', async () => {
    const { onValueChange } = await openCombobox({});
    await fireEvent.input(await screen.findByLabelText('Search options'), { target: { value: 'ta' } });
    await tick();
    const options = await screen.findAllByRole('option');
    const labels = options.map((o) => o.textContent);
    expect(labels.some((t) => /theta/i.test(t))).toBe(true);
    expect(labels.some((t) => /alpha/i.test(t))).toBe(false);
    await fireEvent.click(screen.getByRole('option', { name: /theta/i }));
    await tick();
    expect(onValueChange).toHaveBeenCalledWith('theta');
    expect(screen.getByLabelText('Choice').textContent).toMatch(/theta/i);
  });

  it('shows the empty state when nothing matches', async () => {
    await openCombobox({});
    await fireEvent.input(await screen.findByLabelText('Search options'), { target: { value: 'zzz' } });
    await tick();
    expect(await screen.findByText('No results found.')).toBeTruthy();
    expect(screen.queryAllByRole('option')).toHaveLength(0);
  });

  it('moves the highlight with arrow keys', async () => {
    await openCombobox({});
    const search = await screen.findByLabelText('Search options');
    const options = await screen.findAllByRole('option');
    expect(options[0].getAttribute('aria-selected')).toBe('true');
    await fireEvent.keyDown(search, { key: 'ArrowDown' });
    await tick();
    const after = await screen.findAllByRole('option');
    expect(after[1].getAttribute('aria-selected')).toBe('true');
  });

  it('hides the search when items are few and still picks', async () => {
    const onValueChange = vi.fn();
    render(Combobox, { props: { items: FEW, onValueChange, 'aria-label': 'Toggle' } });
    await fireEvent.click(await screen.findByLabelText('Toggle'));
    expect(screen.queryByLabelText('Search options')).toBeNull();
    await fireEvent.click(await screen.findByRole('option', { name: 'Off' }));
    await tick();
    expect(onValueChange).toHaveBeenCalledWith('off');
    expect(screen.getByLabelText('Toggle').textContent).toMatch(/off/i);
  });

  it('shows the bound value and stays shut when disabled', async () => {
    render(Combobox, { props: { items: FEW, value: 'on', disabled: true, 'aria-label': 'Locked' } });
    expect(screen.getByLabelText('Locked').textContent).toMatch(/on/i);
    await fireEvent.click(screen.getByLabelText('Locked'));
    await tick();
    expect(screen.queryByRole('listbox')).toBeNull();
  });
  it('virtualizes a 500-item list to a window of rows', async () => {
    const onValueChange = vi.fn();
    const big = Array.from({ length: 500 }, (_, i) => ({ value: `item-${i}`, label: `Item ${i}` }));
    render(Combobox, { props: { items: big, onValueChange, 'aria-label': 'Big' } });
    await fireEvent.click(await screen.findByLabelText('Big'));
    const options = await screen.findAllByRole('option');
    expect(options.length).toBeGreaterThan(0);
    expect(options.length).toBeLessThan(500);
  });

  const RICH = [
    { value: 'openrouter/aurora', label: 'aurora', detail: 'openrouter' },
    { value: 'anthropic/opus', label: 'opus', detail: 'anthropic' },
    { value: 'openai/sonnet', label: 'sonnet', detail: 'openai' },
    { value: 'openrouter/borealis', label: 'borealis', detail: 'openrouter' },
    { value: 'openrouter/cirrus', label: 'cirrus', detail: 'openrouter' },
    { value: 'anthropic/drizzle', label: 'drizzle', detail: 'anthropic' },
    { value: 'openai/ember', label: 'ember', detail: 'openai' },
    { value: 'openrouter/flurry', label: 'flurry', detail: 'openrouter' },
    { value: 'anthropic/gale', label: 'gale', detail: 'anthropic' },
    { value: 'openai/haze', label: 'haze', detail: 'openai' },
  ];

  it('renders the provider under the model name and matches it in search', async () => {
    const onValueChange = vi.fn();
    render(Combobox, { props: { items: RICH, onValueChange, 'aria-label': 'Model' } });
    await fireEvent.click(await screen.findByLabelText('Model'));
    const opus = await screen.findByRole('option', { name: /opus anthropic/i });
    expect(opus.textContent).toMatch(/opus/);
    expect(opus.textContent).toMatch(/anthropic/);
    // Provider text matches even though no label contains it.
    await fireEvent.input(await screen.findByLabelText('Search options'), { target: { value: 'openrouter' } });
    await tick();
    const options = await screen.findAllByRole('option');
    expect(options).toHaveLength(4);
    await fireEvent.click(screen.getByRole('option', { name: /aurora/i }));
    await tick();
    expect(onValueChange).toHaveBeenCalledWith('openrouter/aurora');
    // The trigger keeps the provider beside the picked model.
    expect(screen.getByLabelText('Model').textContent).toMatch(/aurora.*openrouter/);
  });

  it('highlights the selected item on open', async () => {
    render(Combobox, { props: { items: MANY, value: 'gamma', 'aria-label': 'Choice' } });
    await fireEvent.click(await screen.findByLabelText('Choice'));
    await tick();
    const options = await screen.findAllByRole('option');
    const selected = options.filter((o) => o.getAttribute('aria-selected') === 'true');
    expect(selected).toHaveLength(1);
    expect(selected[0].textContent).toMatch(/gamma/i);
  });

  it('windows a 500-item rich list to fewer rows', async () => {
    const big = Array.from({ length: 500 }, (_, i) => ({ value: `prov-${i % 7}/model-${i}`, label: `model-${i}`, detail: `prov-${i % 7}` }));
    render(Combobox, { props: { items: big, 'aria-label': 'Rich' } });
    await fireEvent.click(await screen.findByLabelText('Rich'));
    const options = await screen.findAllByRole('option');
    expect(options.length).toBeGreaterThan(0);
    expect(options.length).toBeLessThan(500);
  });
});
