import { describe, it, expect, vi } from 'vitest';
import { render, within, fireEvent } from '@testing-library/svelte';
import { tick } from 'svelte';
import VoiceBubble from './VoiceBubble.svelte';

describe('VoiceBubble', () => {
  it('renders one bar per peak and the duration', () => {
    const { container } = render(VoiceBubble, { props: { src: '/a', peaks: [10, 50, 90], durationMs: 65000 } });
    const q = within(container);
    expect(q.getByLabelText('seek voice message').children).toHaveLength(3);
    expect(q.getByText('1:05')).toBeTruthy();
  });

  it('falls back to a flat waveform without peaks', () => {
    const { container } = render(VoiceBubble, { props: { src: '/a' } });
    expect(within(container).getByLabelText('seek voice message').children.length).toBeGreaterThan(0);
  });

  it('plays and pauses through the audio element', async () => {
    const play = vi.fn(async () => {});
    HTMLMediaElement.prototype.play = play;
    const { container } = render(VoiceBubble, { props: { src: '/a', peaks: [10], durationMs: 1000 } });
    await fireEvent.click(within(container).getByLabelText('play voice message'));
    expect(play).toHaveBeenCalled();
  });

  it('cycles playback speed', async () => {
    const { container } = render(VoiceBubble, { props: { src: '/a', peaks: [10] } });
    const q = within(container);
    await fireEvent.click(q.getByLabelText('playback speed'));
    expect(q.getByLabelText('playback speed').textContent).toBe('1.5x');
    await fireEvent.click(q.getByLabelText('playback speed'));
    await fireEvent.click(q.getByLabelText('playback speed'));
    expect(q.getByLabelText('playback speed').textContent).toBe('1x');
  });

  it('shows the transcript on demand and on hover title', async () => {
    const { container } = render(VoiceBubble, { props: { src: '/a', peaks: [10], transcript: 'hi there' } });
    const q = within(container);
    expect(q.queryByTestId('voice-transcript')).toBeNull();
    expect(q.getByTestId('voice-bubble').getAttribute('title')).toBe('hi there');
    await fireEvent.click(q.getByLabelText('transcript'));
    await tick();
    expect(q.getByTestId('voice-transcript').textContent).toContain('hi there');
  });

  it('offers discard only when given a handler', async () => {
    const onRemove = vi.fn();
    const { container, rerender } = render(VoiceBubble, { props: { src: '/a', peaks: [10] } });
    expect(within(container).queryByLabelText('discard recording')).toBeNull();
    await rerender({ src: '/a', peaks: [10], onRemove });
    await fireEvent.click(within(container).getByLabelText('discard recording'));
    expect(onRemove).toHaveBeenCalled();
  });
});
