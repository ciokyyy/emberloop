import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import Composer from './Composer.svelte';
import { readImages, toWire, MAX_IMAGES } from './images.js';
import { cardForEvent, withImageNote } from './events.js';

const png = (name = 'a.png') => new File([new Uint8Array([137, 80, 78, 71])], name, { type: 'image/png' });

describe('readImages', () => {
  it('reads a png into base64 and strips the data: prefix', async () => {
    const { images, error } = await readImages([png()]);
    expect(error).toBe('');
    expect(images[0].mime).toBe('image/png');
    expect(images[0].data).toBe('iVBORw==');
  });
  it('skips non-images and reports why', async () => {
    const { images, error } = await readImages([new File(['x'], 'a.txt', { type: 'text/plain' })]);
    expect(images).toEqual([]);
    expect(error).toContain('a.txt');
  });
  it('stops at the per-message limit', async () => {
    const files = Array.from({ length: MAX_IMAGES + 1 }, () => png());
    const { images, error } = await readImages(files);
    expect(images).toHaveLength(MAX_IMAGES);
    expect(error).toContain('At most');
  });
  it('toWire drops the preview name', () => {
    expect(toWire([{ mime: 'image/png', data: 'x', name: 'n' }])).toEqual([{ mime: 'image/png', data: 'x' }]);
  });
});

describe('Composer images', () => {
  it('sends an image-only message and clears the attachments', async () => {
    const onSend = vi.fn();
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend } });
    const q = within(container);
    expect(q.getByRole('button', { name: /^Send/ }).disabled).toBe(true);
    await fireEvent.change(q.getByLabelText('image files'), { target: { files: [png()] } });
    await vi.waitFor(() => expect(q.getByLabelText('attached images')).toBeTruthy());
    await fireEvent.click(q.getByRole('button', { name: /^Send/ }));
    await tick();
    expect(onSend).toHaveBeenCalledWith('', 'prompt', [{ mime: 'image/png', data: 'iVBORw==' }], []);
    expect(q.queryByLabelText('attached images')).toBeNull();
  });
  it('removes one attachment', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => {} } });
    const q = within(container);
    await fireEvent.change(q.getByLabelText('image files'), { target: { files: [png('one.png')] } });
    await vi.waitFor(() => q.getByLabelText('remove one.png'));
    await fireEvent.click(q.getByLabelText('remove one.png'));
    expect(q.queryByLabelText('attached images')).toBeNull();
  });
  it('puts the images back when the send fails', async () => {
    const { container } = render(Composer, { props: { status: 'idle', busy: false, onSend: () => { throw new Error('down'); } } });
    const q = within(container);
    await fireEvent.change(q.getByLabelText('image files'), { target: { files: [png()] } });
    await vi.waitFor(() => q.getByLabelText('attached images'));
    await fireEvent.click(q.getByRole('button', { name: /^Send/ }));
    await vi.waitFor(() => expect(q.getByRole('alert')).toBeTruthy());
    expect(q.getByLabelText('attached images')).toBeTruthy();
  });
});

describe('image note', () => {
  it('shows the count on a user card', () => {
    expect(cardForEvent({ kind: 'prompt', text: 'look', images: 2 }).body).toBe('look\n\n_2 images attached_');
    expect(withImageNote('', 1)).toBe('_1 image attached_');
    expect(withImageNote('hi', 0)).toBe('hi');
  });
});
