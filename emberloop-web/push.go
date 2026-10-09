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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// pushTurnDone sends a notification when a root chat turn settles.
func (d *Daemon) pushTurnDone(meta SessionMeta, status string) {
	if meta.Scout || meta.ParentID != "" {
		return
	}
	title := meta.Title
	if title == "" {
		title = meta.Harness + " · " + meta.ID
	}
	body := "Turn finished."
	if status == "failed" {
		body = "Turn failed."
	}
	payload, _ := json.Marshal(map[string]string{"title": title, "body": body, "tag": meta.ID, "sessionId": meta.ID})
	go d.push.notify(payload)
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
