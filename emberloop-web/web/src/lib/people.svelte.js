// The Daemon's account list (GET /api/users) with presence. App refreshes it
// on mount and on the `users` topic; components read it for sender badges,
// owner colors, owner groups and the Session creator. A failed refresh keeps
// the last good list.
import { listUsers } from './api.js';

export const people = $state({ list: [] });

export async function refreshPeople() {
  try {
    people.list = await listUsers();
  } catch {
    // Keep the last good list.
  }
}

// The account with this id, or null while unknown.
export function personFor(id) {
  if (!id) return null;
  return people.list.find((u) => u.id === id) ?? null;
}
