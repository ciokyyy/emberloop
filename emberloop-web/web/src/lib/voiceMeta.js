// Voice recordings share the Session image store. A recording id starts with
// "aud_"; its duration and waveform peaks come from the image list, fetched
// once per Session and again only when an id is missing.
import { listImages } from './api.js';

export const isAudioId = (id) => typeof id === 'string' && id.startsWith('aud_');

const lists = new Map(); // sessionId -> Promise<Map<id, meta>>

function load(sessionId) {
  const p = listImages(sessionId)
    .then((body) => new Map((body.images ?? []).map((m) => [m.id, m])))
    .catch(() => new Map());
  lists.set(sessionId, p);
  return p;
}

export async function audioMeta(sessionId, id) {
  if (!sessionId) return null;
  let map = await (lists.get(sessionId) ?? load(sessionId));
  if (!map.has(id)) map = await load(sessionId);
  return map.get(id) ?? null;
}

export function forgetAudioMeta(sessionId) {
  lists.delete(sessionId);
}

// Fallback bars when a recording has no stored peaks.
export function flatPeaks(n = 48) {
  return Array.from({ length: n }, (_, i) => 30 + ((i * 37) % 40));
}

export function fmtDuration(ms) {
  const s = Math.max(0, Math.round((ms || 0) / 1000));
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}
