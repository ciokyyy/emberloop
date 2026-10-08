import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Composer from './Composer.svelte';

describe('Composer mode badge', () => {
  it('sends as prompt when idle', () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    expect(within(container).getByTestId('composer-mode').textContent).toContain('Prompt');
  });
  it('offers steer and follow-up while working, defaulting to steer', () => {
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    const badge = within(container).getByTestId('composer-mode');
    expect(badge.textContent).toContain('Steer');
    expect(within(container).getByText('Follow-up')).toBeTruthy();
  });
  it('switches the promised mode when follow-up is picked', async () => {
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend: () => {} } });
    await fireEvent.click(within(container).getByText('Follow-up'));
    await tick();
    expect(within(container).getByTestId('composer-mode').textContent).toContain('Follow-up');
  });
  it('keeps the text and shows an error when send fails', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => { throw new Error('down'); } } });
    const box = within(container).getByLabelText('message');
    await fireEvent.input(box, { target: { value: 'hello' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(box.value).toBe('hello');
    expect(within(container).getByRole('alert').textContent).toMatch(/failed/i);
  });
  it('sends with the working mode on submit', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'working', busy: false, onSend } });
    await fireEvent.input(within(container).getByLabelText('message'), { target: { value: 'wait' } });
    await tick();
    await fireEvent.click(within(container).getByText('Send'));
    await tick();
    expect(onSend).toHaveBeenCalledWith('wait', 'steer');
  });
});
