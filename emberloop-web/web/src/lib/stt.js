// Speech-to-text client: records one voice message from the mic and posts it
// whole to the Daemon, which transcribes it on OpenRouter (openai/gpt-transcribe
// by default, EL_STT_MODEL overrides). The OpenRouter key stays on the Daemon.
//
// The recording is one MediaRecorder take, flushed in 1 s chunks so an
// interrupted take (screen lock, tab kill, track end) still yields the audio
// captured so far. A Screen Wake Lock keeps the screen on while recording and
// is re-requested when the tab becomes visible again. While recording, a
// level meter feeds the live waveform and the stored peaks.
import { apiBase } from './api.js';

// OpenRouter's 25 MB upload cap, enforced on both ends.
export const STT_MAX_BYTES = 25 * 1024 * 1024;

export const WAVE_BARS = 48; // stored waveform bars per recording
const LEVEL_MS = 50; // level meter cadence
const CHUNK_MS = 1000; // MediaRecorder timeslice

// The mic needs a secure context (https or localhost), e.g. `tailscale serve`.
export function sttSupport() {
  if (typeof window === 'undefined') return { ok: false, reason: 'No window.' };
  if (!window.isSecureContext) return { ok: false, reason: 'Mic needs https or localhost.' };
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
    return { ok: false, reason: 'This browser cannot record audio.' };
  }
  return { ok: true, reason: '' };
}

// transcribe(blob, { format }) -> text. format=true appends ?format=1 so the
// Daemon runs the cleanup pass; false skips it. Throws on failure.
export async function transcribe(blob, { format = false } = {}) {
  if (blob.size > STT_MAX_BYTES) throw new Error('Recording is over the 25 MB cap.');
  const res = await fetch(apiBase() + '/api/stt' + (format ? '?format=1' : '?format=0'), {
    method: 'POST',
    headers: { 'Content-Type': blob.type || 'audio/webm' },
    body: blob,
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `request failed: ${res.status}`);
  return String(body.text ?? '').trim();
}

// makePeaks(levels, bars) -> bars integers in 6..100: the loudest level per
// bucket, scaled so the loudest bucket reaches 100. Silence stays at the floor.
export function makePeaks(levels, bars = WAVE_BARS) {
  const out = new Array(bars).fill(6);
  if (!levels.length) return out;
  const peak = new Array(bars).fill(0);
  levels.forEach((v, i) => {
    const b = Math.min(bars - 1, Math.floor((i / levels.length) * bars));
    if (v > peak[b]) peak[b] = v;
  });
  const max = Math.max(...peak);
  if (max <= 0) return out;
  return peak.map((v) => Math.max(6, Math.round((v / max) * 100)));
}

function pickMime() {
  if (typeof MediaRecorder !== 'undefined' && typeof MediaRecorder.isTypeSupported === 'function') {
    for (const t of ['audio/webm;codecs=opus', 'audio/webm', 'audio/mp4']) {
      try {
        if (MediaRecorder.isTypeSupported(t)) return t;
      } catch {
        // Try the next candidate.
      }
    }
  }
  return '';
}

// Wake lock: best-effort. It keeps the screen from sleeping while recording;
// the browser drops it when the tab hides, so it is requested again on return.
function holdWakeLock() {
  let lock = null;
  let held = true;
  const ask = async () => {
    if (!held || !navigator.wakeLock?.request) return;
    try {
      lock = await navigator.wakeLock.request('screen');
    } catch {
      // Denied or unsupported: recording still works.
    }
  };
  const onVisible = () => {
    if (document.visibilityState === 'visible') void ask();
  };
  document.addEventListener('visibilitychange', onVisible);
  void ask();
  return () => {
    held = false;
    document.removeEventListener('visibilitychange', onVisible);
    try {
      void lock?.release();
    } catch {
      // Already released.
    }
  };
}

// startRecording({ onLevel, onEnded }) records one take. onLevel(0..1) fires
// every 50 ms for the live waveform. onEnded() fires when the browser stops
// the take on its own (track ended, recorder error); call stop() then to keep
// what was captured. stop() resolves { blob, durationMs, peaks }; cancel()
// drops the take. Both release the mic and the wake lock.
export async function startRecording({ onLevel = () => {}, onEnded = () => {} } = {}) {
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const mimeType = pickMime();
  const releaseLock = holdWakeLock();
  let rec;
  try {
    rec = mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
  } catch {
    rec = new MediaRecorder(stream);
  }
  const chunks = [];
  const levels = [];
  let finished = false;
  let analyser = null;
  let audioCtx = null;
  let samples = null;

  try {
    const AC = window.AudioContext || window.webkitAudioContext;
    if (AC) {
      audioCtx = new AC();
      try {
        await audioCtx.resume();
      } catch {
        // The context still runs where autoplay needs no gesture.
      }
      analyser = audioCtx.createAnalyser();
      analyser.fftSize = 1024;
      samples = new Uint8Array(analyser.fftSize);
      audioCtx.createMediaStreamSource(stream).connect(analyser);
    }
  } catch {
    analyser = null;
  }

  const level = () => {
    if (!analyser) return 0;
    analyser.getByteTimeDomainData(samples);
    let sum = 0;
    for (let i = 0; i < samples.length; i++) {
      const v = (samples[i] - 128) / 128;
      sum += v * v;
    }
    return Math.min(1, Math.sqrt(sum / samples.length) * 4);
  };

  const t0 = Date.now();
  rec.ondataavailable = (e) => {
    if (e.data?.size) chunks.push(e.data);
  };
  rec.onerror = () => {
    if (!finished) onEnded();
  };
  stream.getTracks().forEach((t) => {
    t.onended = () => {
      if (!finished) onEnded();
    };
  });
  try {
    rec.start(CHUNK_MS);
  } catch {
    rec.start();
  }
  const meter = setInterval(() => {
    const v = level();
    levels.push(v);
    try {
      onLevel(v);
    } catch {
      // A throwing listener must not stop the meter.
    }
  }, LEVEL_MS);

  const release = () => {
    finished = true;
    clearInterval(meter);
    releaseLock();
    try {
      stream.getTracks().forEach((t) => t.stop());
    } catch {
      // The mic is already gone.
    }
    try {
      audioCtx?.close();
    } catch {
      // No audio context to close.
    }
  };

  return {
    stop: () =>
      new Promise((resolve) => {
        const done = () => {
          const durationMs = Date.now() - t0;
          release();
          const blob = new Blob(chunks, { type: rec.mimeType || mimeType || 'audio/webm' });
          resolve({ blob, durationMs, peaks: makePeaks(levels) });
        };
        if (finished) return resolve({ blob: new Blob([]), durationMs: 0, peaks: makePeaks([]) });
        if (rec.state === 'inactive') return done();
        rec.onstop = done;
        try {
          rec.stop();
        } catch {
          done();
        }
      }),
    cancel: () => {
      if (finished) return;
      rec.ondataavailable = null;
      rec.onstop = null;
      try {
        if (rec.state !== 'inactive') rec.stop();
      } catch {
        // Already stopped.
      }
      release();
    },
  };
}
