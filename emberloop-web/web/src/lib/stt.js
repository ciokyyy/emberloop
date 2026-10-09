// Speech-to-text client: records the mic in short segments and posts each
// one to the Daemon, which transcribes it on OpenRouter (openai/gpt-transcribe
// by default, EL_STT_MODEL overrides) and optionally runs the cleanup model
// (?format=1). The OpenRouter key stays on the Daemon.
//
// Livestream: OpenRouter STT is not streaming, so while the user speaks each
// finished segment is transcribed as it closes and the caller appends the
// text to the draft live. Every segment is recorded with its own fresh
// MediaRecorder, so each blob is independently decodable (never slices of
// one webm). Segments close on a silence pause (AnalyserNode) or on a timer
// when no audio analysis is available.
import { apiBase } from './api.js';

// OpenRouter's 25 MB upload cap, enforced per segment on both ends.
export const STT_MAX_BYTES = 25 * 1024 * 1024;

const SEGMENT_MS = 9000; // timed fallback cadence without audio analysis
const SILENCE_MS = 1600; // quiet gap that closes a segment mid-speech
const MIN_SEGMENT_MS = 1200; // never close a segment before this
const MAX_SEGMENT_MS = 20000; // hard close for one unbroken utterance
const LOUD_RMS = 0.02; // time-domain RMS above this counts as speech

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
export async function transcribe(blob, { format = true } = {}) {
  if (blob.size > STT_MAX_BYTES) throw new Error('Voice part is over the 25 MB cap.');
  const res = await fetch(apiBase() + '/api/stt' + (format ? '?format=1' : '?format=0'), {
    method: 'POST',
    headers: { 'Content-Type': blob.type || 'audio/webm' },
    body: blob,
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(body.error || `request failed: ${res.status}`);
  return String(body.text ?? '').trim();
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

// Start live recording; onSegment(blob, seq) fires in seq order as each
// segment closes, plus a final one on stop(). Returns { stop(), cancel() }:
// stop flushes the tail segment, cancel drops it. Both release the mic.
export async function startLiveRecording({ onSegment, segmentMs = SEGMENT_MS, silenceMs = SILENCE_MS } = {}) {
  if (typeof onSegment !== 'function') throw new Error('onSegment(blob, seq) is required');
  const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
  const mimeType = pickMime();
  let stopped = false;
  let closing = false;
  let seq = 0;
  let rec = null;
  let chunks = [];
  let segStart = 0;
  let lastLoud = 0;
  let heard = false;
  let analyser = null;
  let audioCtx = null;
  let freq = null;

  // Silence detection is best-effort: without an AudioContext the timed
  // fallback below still closes segments, just without pause detection.
  try {
    const AC = window.AudioContext || window.webkitAudioContext;
    if (AC) {
      audioCtx = new AC();
      const src = audioCtx.createMediaStreamSource(stream);
      analyser = audioCtx.createAnalyser();
      analyser.fftSize = 2048;
      freq = new Uint8Array(analyser.fftSize);
      src.connect(analyser);
    }
  } catch {
    analyser = null;
  }

  const loudNow = () => {
    if (!analyser) return false;
    analyser.getByteTimeDomainData(freq);
    let sum = 0;
    for (let i = 0; i < freq.length; i++) {
      const v = (freq[i] - 128) / 128;
      sum += v * v;
    }
    return Math.sqrt(sum / freq.length) > LOUD_RMS;
  };

  const newRecorder = () => {
    try {
      return mimeType ? new MediaRecorder(stream, { mimeType }) : new MediaRecorder(stream);
    } catch {
      return new MediaRecorder(stream);
    }
  };

  const openSegment = () => {
    if (stopped) return;
    chunks = [];
    rec = newRecorder();
    rec.ondataavailable = (e) => {
      if (e.data?.size) chunks.push(e.data);
    };
    segStart = Date.now();
    lastLoud = segStart;
    heard = false;
    try {
      rec.start(250);
    } catch {
      rec.start();
    }
  };

  // The final dataavailable for a segment fires as stop() runs, before
  // onstop, so the chunk array is only swapped inside onstop: swapping
  // earlier would drop the tail audio into the next segment's array.
  const closeSegment = () =>
    new Promise((resolve) => {
      const cur = rec;
      if (!cur || cur.state === 'inactive') return resolve();
      cur.onstop = () => {
        const parts = chunks;
        chunks = [];
        rec = null;
        const blob = new Blob(parts, { type: cur.mimeType || mimeType || 'audio/webm' });
        // Empty segments (instant stop) carry nothing; the caller treats a
        // recording with no text at all as "no speech heard".
        if (blob.size > 0) {
          try {
            onSegment(blob, seq++);
          } catch {
            // A throwing callback must not kill the recording loop.
          }
        }
        resolve();
      };
      try {
        cur.stop();
      } catch {
        rec = null;
        chunks = [];
        resolve();
      }
    });

  const tickFn = async () => {
    if (stopped || closing || !rec) return;
    const now = Date.now();
    const elapsed = now - segStart;
    if (analyser) {
      if (loudNow()) {
        heard = true;
        lastLoud = now;
      }
      const paused = heard && elapsed >= MIN_SEGMENT_MS && now - lastLoud >= silenceMs;
      if (paused || elapsed >= MAX_SEGMENT_MS) {
        closing = true;
        try {
          await closeSegment();
        } finally {
          closing = false;
          openSegment();
        }
      }
    } else if (elapsed >= segmentMs) {
      closing = true;
      try {
        await closeSegment();
      } finally {
        closing = false;
        openSegment();
      }
    }
  };

  const release = () => {
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

  openSegment();
  const timer = setInterval(() => {
    tickFn().catch(() => {});
  }, 200);

  return {
    stop: async () => {
      if (stopped) return;
      stopped = true;
      clearInterval(timer);
      await closeSegment();
      release();
    },
    cancel: () => {
      if (stopped) return;
      stopped = true;
      clearInterval(timer);
      const cur = rec;
      rec = null;
      chunks = [];
      if (cur) {
        cur.ondataavailable = null;
        cur.onstop = () => {};
        try {
          if (cur.state !== 'inactive') cur.stop();
        } catch {
          // Already stopped.
        }
      }
      release();
    },
  };
}
