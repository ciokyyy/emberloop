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
// queue), "runs" (the list), "run:<id>" (one Run), "settings", "users" (the
// User list and who is online).
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

// presence counts the live channels each User holds open. A User is online
// while one channel is open. The first join and the last leave publish
// "users", so the sidebar refetches GET /api/users.
type presence struct {
	mu   sync.Mutex
	open map[string]int
}

// join records one open channel of userID. It reports true when userID just
// turned online.
func (p *presence) join(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open == nil {
		p.open = map[string]int{}
	}
	p.open[userID]++
	return p.open[userID] == 1
}

// leave closes one channel of userID. It reports true when userID just turned
// offline.
func (p *presence) leave(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := p.open[userID]
	if n == 0 {
		return false
	}
	if n == 1 {
		delete(p.open, userID)
		return true
	}
	p.open[userID] = n - 1
	return false
}

// online reports whether userID holds at least one open channel.
func (p *presence) online(userID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.open[userID] > 0
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
	// A signed-in User with an open channel is online. The machine token is no User.
	if who, ok := d.requesterOf(r); ok && who.user != nil {
		uid := who.user.ID
		if d.presence.join(uid) {
			d.live.publish("users")
		}
		defer func() {
			if d.presence.leave(uid) {
				d.live.publish("users")
			}
		}()
	}
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
