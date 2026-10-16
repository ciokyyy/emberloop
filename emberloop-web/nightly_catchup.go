package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// nightlyCatchupIngest runs the turn-end ingester for settled Sessions whose
// debounce worker did not finish before shutdown. It shares the ingestion
// cursor and scan lock with the live path.

// The event-log scan delegates to ingestSession, which filters each candidate
// with MinerReadable before Episode storage or extraction.
func (d *Daemon) nightlyCatchupIngest(ctx context.Context) (int, error) {
	settled := map[string]bool{}
	d.mu.Lock()
	for id, s := range d.sessions {
		if s.meta.Status == "ended" || s.Idle(d.now(), MinerSettleWindow, d.groupOutstandingLocked(id)) {
			settled[id] = true
		}
	}
	d.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(d.stateDir, "enderbloom", "sessions"))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			b, e := os.ReadFile(filepath.Join(d.sessionDir(entry.Name()), "meta.json"))
			if e != nil {
				continue
			}
			var meta SessionMeta
			if json.Unmarshal(b, &meta) != nil {
				continue
			}
			if meta.Status == "ended" {
				settled[entry.Name()] = true
				continue
			}
			info, e := os.Stat(filepath.Join(d.sessionDir(entry.Name()), "events.jsonl"))
			if e == nil && d.now().Sub(info.ModTime()) >= MinerSettleWindow {
				settled[entry.Name()] = true
			}
		}
	}
	ids := make([]string, 0, len(settled))
	for id := range settled {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	failed := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return len(ids) - failed, err
		}
		flight := d.mineFlightFor(id)
		flight.scan.Lock()
		err := d.ingestSession(id)
		flight.scan.Unlock()
		if err != nil {
			failed++
		}
	}
	if failed > 0 {
		return len(ids) - failed, fmt.Errorf("nightly catch-up ingest failed for %d of %d Sessions", failed, len(ids))
	}
	return len(ids), nil
}
