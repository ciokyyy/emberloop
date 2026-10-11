package web

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Web Push: the browser registers a subscription, the Daemon encrypts a
// small JSON payload per RFC 8291 (aes128gcm) and signs the request with a
// VAPID key (RFC 8292). Stdlib only. Browsers deliver pushes only to a
// secure context: https, or http on localhost. A plain http tailnet IP is
// not secure, so use `tailscale serve` (https on the ts.net name).

var b64 = base64.RawURLEncoding

type pushSub struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

type pushStore struct {
	mu   sync.Mutex
	dir  string
	key  *ecdsa.PrivateKey
	subs []pushSub
	// client sends the push requests; tests swap it.
	client *http.Client
}

func newPushStore(stateDir string) *pushStore {
	return &pushStore{dir: filepath.Join(stateDir, "enderbloom", "push"), client: &http.Client{Timeout: 15 * time.Second}}
}

func (p *pushStore) load() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.key != nil {
		return nil
	}
	if raw, err := os.ReadFile(filepath.Join(p.dir, "vapid.json")); err == nil {
		var v struct{ D string }
		if json.Unmarshal(raw, &v) == nil {
			if d, err := b64.DecodeString(v.D); err == nil {
				if k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), d); err == nil {
					p.key = k
				}
			}
		}
	}
	if p.key == nil {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(p.dir, 0o700); err != nil {
			return err
		}
		raw, _ := json.Marshal(map[string]string{"D": b64.EncodeToString(k.D.FillBytes(make([]byte, 32)))})
		if err := os.WriteFile(filepath.Join(p.dir, "vapid.json"), raw, 0o600); err != nil {
			return err
		}
		p.key = k
	}
	if raw, err := os.ReadFile(filepath.Join(p.dir, "subs.json")); err == nil {
		_ = json.Unmarshal(raw, &p.subs)
	}
	return nil
}

func (p *pushStore) saveSubsLocked() {
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return
	}
	raw, _ := json.Marshal(p.subs)
	_ = os.WriteFile(filepath.Join(p.dir, "subs.json"), raw, 0o600)
}

// publicKey is the uncompressed VAPID public key, base64url, for
// pushManager.subscribe({applicationServerKey}).
func (p *pushStore) publicKey() (string, error) {
	if err := p.load(); err != nil {
		return "", err
	}
	pub, err := p.key.PublicKey.ECDH()
	if err != nil {
		return "", err
	}
	return b64.EncodeToString(pub.Bytes()), nil
}

func (p *pushStore) add(s pushSub) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.subs {
		if p.subs[i].Endpoint == s.Endpoint {
			p.subs[i] = s
			p.saveSubsLocked()
			return
		}
	}
	p.subs = append(p.subs, s)
	p.saveSubsLocked()
}

func (p *pushStore) remove(endpoint string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.subs[:0]
	for _, s := range p.subs {
		if s.Endpoint != endpoint {
			out = append(out, s)
		}
	}
	p.subs = out
	p.saveSubsLocked()
}

// notify sends the payload to every subscription. Gone subscriptions
// (404, 410) are dropped.
func (p *pushStore) notify(payload []byte) {
	if p.load() != nil {
		return
	}
	p.mu.Lock()
	subs := append([]pushSub(nil), p.subs...)
	p.mu.Unlock()
	for _, s := range subs {
		code, err := p.send(s, payload)
		if err == nil && (code == http.StatusNotFound || code == http.StatusGone) {
			p.remove(s.Endpoint)
		}
	}
}

func (p *pushStore) send(s pushSub, payload []byte) (int, error) {
	body, err := encryptPush(s, payload)
	if err != nil {
		return 0, err
	}
	u, err := url.Parse(s.Endpoint)
	if err != nil {
		return 0, err
	}
	jwt, err := p.vapidJWT(u.Scheme + "://" + u.Host)
	if err != nil {
		return 0, err
	}
	pub, _ := p.publicKey()
	req, err := http.NewRequest(http.MethodPost, s.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", fmt.Sprintf("vapid t=%s, k=%s", jwt, pub))
	res, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	res.Body.Close()
	return res.StatusCode, nil
}

func (p *pushStore) vapidJWT(aud string) (string, error) {
	if err := p.load(); err != nil {
		return "", err
	}
	head := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, _ := json.Marshal(map[string]any{"aud": aud, "exp": time.Now().Add(12 * time.Hour).Unix(), "sub": "mailto:el@localhost"})
	signing := head + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, sg, err := ecdsa.Sign(rand.Reader, p.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := append(r.FillBytes(make([]byte, 32)), sg.FillBytes(make([]byte, 32))...)
	return signing + "." + b64.EncodeToString(sig), nil
}

// encryptPush builds the RFC 8291 aes128gcm body for one subscription.
func encryptPush(s pushSub, payload []byte) ([]byte, error) {
	uaPub, err := b64.DecodeString(s.P256dh)
	if err != nil {
		return nil, errors.New("bad p256dh")
	}
	authSecret, err := b64.DecodeString(s.Auth)
	if err != nil {
		return nil, errors.New("bad auth")
	}
	curve := ecdh.P256()
	uaKey, err := curve.NewPublicKey(uaPub)
	if err != nil {
		return nil, err
	}
	asKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := asKey.ECDH(uaKey)
	if err != nil {
		return nil, err
	}
	asPub := asKey.PublicKey().Bytes()
	keyInfo := append(append([]byte("WebPush: info\x00"), uaPub...), asPub...)
	ikm, err := hkdf.Key(sha256.New, shared, authSecret, string(keyInfo), 32)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	// One record: payload, then the 0x02 last-record delimiter.
	sealed := gcm.Seal(nil, nonce, append(append([]byte(nil), payload...), 0x02), nil)
	hdr := make([]byte, 0, 21+len(asPub))
	hdr = append(hdr, salt...)
	hdr = binary.BigEndian.AppendUint32(hdr, 4096)
	hdr = append(hdr, byte(len(asPub)))
	hdr = append(hdr, asPub...)
	return append(hdr, sealed...), nil
}

// pushMessage is the JSON body every push carries. Cue names an interface
// sound for open tabs (the service worker messages them to play it; a
// custom sound inside the OS notification itself is impossible). Key names
// the event, so a tab that already sounded it over its live stream stays
// silent. Path is the exact client route the notification opens.
type pushMessage struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	Tag       string `json:"tag,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	Cue       string `json:"cue,omitempty"`
	Key       string `json:"key,omitempty"`
	Path      string `json:"path,omitempty"`
}

func pushPayload(m pushMessage) []byte {
	payload, _ := json.Marshal(m)
	return payload
}

func sessionTitle(meta SessionMeta) string {
	if meta.Title != "" {
		return meta.Title
	}
	return meta.Harness + " · " + meta.ID
}

// pushTurnDone sends a notification when a root chat turn settles.
func (d *Daemon) pushTurnDone(meta SessionMeta, status string, seq int64) {
	if meta.Scout || meta.ParentID != "" || meta.Owner == "" {
		return
	}
	body := "Turn finished."
	cue := "notification"
	if status == "failed" {
		body = "Turn failed."
		cue = "error"
	}
	payload := pushPayload(pushMessage{
		Title: sessionTitle(meta), Body: body, Tag: meta.ID, SessionID: meta.ID,
		Cue: cue, Key: "turn:" + meta.ID + ":" + itoa(seq), Path: "/s/" + meta.ID,
	})
	go d.push.notify(payload)
}

// pushSudo sends a notification when an agent asks to run sudo: the turn
// cannot continue without the user, on any session depth.
func (d *Daemon) pushSudo(meta SessionMeta, reqID, command string) {
	if meta.Owner == "" {
		return
	}
	payload := pushPayload(pushMessage{
		Title: sessionTitle(meta), Body: "Sudo approval needed: " + truncatePush(command, 120),
		Tag: reqID, SessionID: meta.ID,
		Cue: "mention", Key: "sudo:" + reqID, Path: "/s/" + meta.ID,
	})
	go d.push.notify(payload)
}

// pushRunNeedsYou sends a notification when a Run ticket stalls and needs
// the user. Callers hold no lock: the payload is built before the call.
func (d *Daemon) pushRunNeedsYou(runID, ticketID string, num int, title, reason string) {
	name := title
	if name == "" {
		name = ticketID
	}
	payload := pushPayload(pushMessage{
		Title: "Ticket " + itoa(int64(num)) + " needs you",
		Body:  name + ": " + truncatePush(reason, 180),
		Tag:   runID + "/" + ticketID,
		Cue:   "blocked", Key: needsYouKey(runID, ticketID, reason),
		Path: "/runs/" + runID + "/" + ticketID,
	})
	go d.push.notify(payload)
}

// needsYouKey names a needs-you event for push dedup: run, ticket and the
// reason's hash, so a new reason sounds again in a tab that already heard
// the old one. The browser builds the same key in sound.js (FNV-1a over the
// UTF-8 reason), so a push arriving over both the Run SSE and the service
// worker message sounds once.
func needsYouKey(runID, ticketID, reason string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(reason))
	return "needsyou:" + runID + ":" + ticketID + ":" + strconv.FormatUint(uint64(h.Sum32()), 16)
}

func truncatePush(s string, max int) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	if len([]rune(s)) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func (d *Daemon) handlePush(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/api/push/key" && r.Method == http.MethodGet:
		k, err := d.push.publicKey()
		if err != nil {
			http.Error(w, "push key unavailable", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"key": k})
	case r.URL.Path == "/api/push/subscribe" && r.Method == http.MethodPost:
		var in struct {
			Endpoint string `json:"endpoint"`
			Keys     struct {
				P256dh string `json:"p256dh"`
				Auth   string `json:"auth"`
			} `json:"keys"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil || in.Endpoint == "" || in.Keys.P256dh == "" || in.Keys.Auth == "" {
			http.Error(w, "endpoint and keys are required", http.StatusBadRequest)
			return
		}
		if u, err := url.Parse(in.Endpoint); err != nil || u.Scheme != "https" {
			http.Error(w, "endpoint must be https", http.StatusBadRequest)
			return
		}
		if d.push.load() != nil {
			http.Error(w, "push unavailable", http.StatusInternalServerError)
			return
		}
		d.push.add(pushSub{Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth})
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case r.URL.Path == "/api/push/subscribe" && r.Method == http.MethodDelete:
		var in struct {
			Endpoint string `json:"endpoint"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil || in.Endpoint == "" {
			http.Error(w, "endpoint is required", http.StatusBadRequest)
			return
		}
		if d.push.load() == nil {
			d.push.remove(in.Endpoint)
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.NotFound(w, r)
	}
}
