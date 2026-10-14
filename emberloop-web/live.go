package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	el "example.com/emberloop-web/el"
)

// liveHub is the one realtime channel of the Daemon. State changes publish
// topic names over an SSE stream; the client refetches their normal endpoints.
// Quota subscribers use a dedicated payload event because the page should not
// poll to pick up changed provider values.
type quotaLiveEvent struct {
	Topic     string             `json:"topic"`
	Providers []el.QuotaProvider `json:"providers"`
}

type liveMessage struct {
	topic string
	quota *quotaLiveEvent
}

type liveHub struct {
	mu   sync.Mutex
	subs map[chan liveMessage]bool // true only for streams subscribed to quota
}

func newLiveHub() *liveHub { return &liveHub{subs: map[chan liveMessage]bool{}} }

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

// enqueueQuota replaces any queued quota event with the newest provider list.
// If the buffer is full, it preserves ordinary invalidations and drops only
// the oldest one needed to make room.
func enqueueQuota(ch chan liveMessage, message liveMessage) {
	select {
	case ch <- message:
		return
	default:
	}

	queued := make([]liveMessage, 0, cap(ch))
	for {
		select {
		case old := <-ch:
			if old.quota == nil {
				queued = append(queued, old)
			}
		default:
			if len(queued) == cap(ch) {
				queued = queued[1:]
			}
			for _, old := range queued {
				ch <- old
			}
			ch <- message
			return
		}
	}
}

// publishQuota sends the full latest provider list to quota subscribers only.
func (h *liveHub) publishQuota(providers []el.QuotaProvider) {
	if h == nil {
		return
	}
	message := liveMessage{
		topic: "quota",
		quota: &quotaLiveEvent{Topic: "quota", Providers: append([]el.QuotaProvider(nil), providers...)},
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, quotaSubscriber := range h.subs {
		if quotaSubscriber {
			enqueueQuota(ch, message)
		}
	}
}

func (h *liveHub) subscribe(quota bool) chan liveMessage {
	ch := make(chan liveMessage, 256)
	h.mu.Lock()
	h.subs[ch] = quota
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
// consumers open a separate connection with ?topic=quota; only those streams
// start the refresher and receive its provider-list payloads.
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
	quotaSubscriber := quotaTopicRequested(r)
	ch := d.live.subscribe(quotaSubscriber)
	defer d.live.unsubscribe(ch)
	if quotaSubscriber {
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
			if message.quota != nil {
				if event, err := json.Marshal(message.quota); err == nil {
					fmt.Fprintf(w, "data: %s\n\n", event)
				}
			} else {
				fmt.Fprintf(w, "data: %s\n\n", message.topic)
			}
			flush()
		}
	}
}
