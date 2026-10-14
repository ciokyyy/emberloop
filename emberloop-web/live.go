package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// liveHub is the one realtime channel of the Daemon. Every state change
// publishes the name of the topic it touched; GET /api/live fans those names
// out over a single SSE stream. State topics normally carry no data and the
// client refetches their normal GET endpoint. Topics may opt into a payload
// when refetching would defeat the purpose of a live update (quota).
//
// Topics: "sessions" (the list), "session:<id>" (one Session's meta and
// queue), "runs" (the list), "run:<id>" (one Run), "settings", "users" (the
// User list and who is online), and "quota" (the provider list).
// Transcript Events keep their own append-only stream; they are a log, not
// state.
type liveMessage struct {
	topic   string
	payload []byte
}

type liveHub struct {
	mu   sync.Mutex
	subs map[chan liveMessage]struct{}
}

func newLiveHub() *liveHub { return &liveHub{subs: map[chan liveMessage]struct{}{}} }

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
		for _, topic := range topics {
			select {
			case ch <- liveMessage{topic: topic}:
			default:
			}
		}
	}
}

// publishPayload publishes a topic with a JSON provider-list payload. The
// bytes are copied so the caller can safely reuse its buffer.
func (h *liveHub) publishPayload(topic string, payload []byte) {
	if h == nil || !json.Valid(payload) {
		return
	}
	message := liveMessage{topic: topic, payload: append([]byte(nil), payload...)}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- message:
		default:
		}
	}
}

func (h *liveHub) subscribe() chan liveMessage {
	ch := make(chan liveMessage, 256)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *liveHub) unsubscribe(ch chan liveMessage) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func quotaTopicRequested(r *http.Request) bool {
	for _, topic := range r.URL.Query()["topic"] {
		if topic == "quota" {
			return true
		}
	}
	for _, topics := range r.URL.Query()["topics"] {
		for _, topic := range strings.Split(topics, ",") {
			if strings.TrimSpace(topic) == "quota" {
				return true
			}
		}
	}
	return false
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
// message, which tells the client to refetch everything it shows. Quota
// subscribers opt in with ?topic=quota; only then does its refresher run.
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
	if quotaTopicRequested(r) {
		stopQuota := d.subscribeQuotaRefresher()
		defer stopQuota()
	}
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
		case message := <-ch:
			if len(message.payload) != 0 {
				fmt.Fprintf(w, "data: {\"topic\":%q,\"providers\":%s}\n\n", message.topic, message.payload)
			} else {
				fmt.Fprintf(w, "data: %s\n\n", message.topic)
			}
			flush()
		}
	}
}
