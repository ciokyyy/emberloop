package web

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// liveHub is the one realtime channel of the Daemon. Every state change
// publishes the name of the topic it touched; GET /api/live fans those names
// out over a single SSE stream. A message carries no data: the client refetches
// the topic's normal GET endpoint, so each payload keeps one definition.
//
// Topics: "sessions" (the list), "session:<id>" (one Session's meta and
// queue), "runs" (the list), "run:<id>" (one Run), "settings".
// Transcript Events keep their own append-only stream; they are a log, not
// state.
type liveHub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

func newLiveHub() *liveHub { return &liveHub{subs: map[chan string]struct{}{}} }

// publish tells every subscriber the topics changed. It never blocks: a
// subscriber with a full buffer already has a refetch pending for the same
// topic, or reconnects and resyncs everything.
func (h *liveHub) publish(topics ...string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		for _, t := range topics {
			select {
			case ch <- t:
			default:
			}
		}
	}
}

func (h *liveHub) subscribe() chan string {
	ch := make(chan string, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *liveHub) unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// handleLive serves GET /api/live. A new connection first gets a "hello"
// message, which tells the client to refetch everything it shows.
func (d *Daemon) handleLive(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	ch := d.live.subscribe()
	defer d.live.unsubscribe(ch)
	fmt.Fprint(w, "data: hello\n\n")
	flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flush()
		case topic := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", topic)
			flush()
		}
	}
}
