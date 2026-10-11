package web

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// decryptPush is the browser side of RFC 8291, for the round-trip check.
func decryptPush(t *testing.T, ua *ecdh.PrivateKey, auth, body []byte) []byte {
	t.Helper()
	salt := body[:16]
	if binary.BigEndian.Uint32(body[16:20]) != 4096 {
		t.Fatal("record size not 4096")
	}
	idLen := int(body[20])
	asPub := body[21 : 21+idLen]
	cipherText := body[21+idLen:]
	asKey, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(asKey)
	if err != nil {
		t.Fatal(err)
	}
	info := append(append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...), asPub...)
	ikm, _ := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plain[len(plain)-1] != 0x02 {
		t.Fatal("missing last-record delimiter")
	}
	return plain[:len(plain)-1]
}

func newTestSub(t *testing.T, endpoint string) (pushSub, *ecdh.PrivateKey, []byte) {
	t.Helper()
	ua, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return pushSub{Endpoint: endpoint, P256dh: b64.EncodeToString(ua.PublicKey().Bytes()), Auth: b64.EncodeToString(auth)}, ua, auth
}

func TestPushEncryptRoundTrip(t *testing.T) {
	sub, ua, auth := newTestSub(t, "https://push.example/x")
	body, err := encryptPush(sub, []byte(`{"title":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(decryptPush(t, ua, auth, body)); got != `{"title":"hi"}` {
		t.Fatalf("payload = %q", got)
	}
}

func TestPushNotifySendsVapidAndDropsGone(t *testing.T) {
	var gotAuth, gotEnc string
	var gotBody []byte
	gone := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotEnc = r.Header.Get("Authorization"), r.Header.Get("Content-Encoding")
		gotBody, _ = io.ReadAll(r.Body)
		if gone {
			w.WriteHeader(http.StatusGone)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	p := newPushStore(tempDir(t))
	p.client = srv.Client()
	sub, ua, auth := newTestSub(t, srv.URL+"/sub")
	p.add(sub)
	p.notify([]byte("ping"))
	if gotEnc != "aes128gcm" || !strings.HasPrefix(gotAuth, "vapid t=") {
		t.Fatalf("headers enc=%q auth=%q", gotEnc, gotAuth)
	}
	pub, _ := p.publicKey()
	if !strings.HasSuffix(gotAuth, ", k="+pub) {
		t.Fatalf("auth lacks public key: %q", gotAuth)
	}
	if string(decryptPush(t, ua, auth, gotBody)) != "ping" {
		t.Fatal("bad payload")
	}
	// The key and subscription survive a reload.
	p2 := newPushStore(p.dir[:len(p.dir)-len("/enderbloom/push")])
	if k, _ := p2.publicKey(); k != pub || len(p2.subs) != 1 {
		t.Fatalf("reload key=%v subs=%d", k == pub, len(p2.subs))
	}
	gone = true
	p.notify([]byte("ping"))
	if len(p.subs) != 0 {
		t.Fatal("410 subscription not dropped")
	}
}

func TestPushAPI(t *testing.T) {
	d := NewDaemon(tempDir(t), NewFakeBackend())
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	res, err := http.Get(srv.URL + "/api/push/key")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("key: %v %v", err, res)
	}
	res.Body.Close()
	post := func(method, body string) int {
		req, _ := http.NewRequest(method, srv.URL+"/api/push/subscribe", strings.NewReader(body))
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		return r.StatusCode
	}
	if c := post("POST", `{"endpoint":"http://x","keys":{"p256dh":"a","auth":"b"}}`); c != 400 {
		t.Fatalf("http endpoint = %d", c)
	}
	if c := post("POST", `{"endpoint":"https://x/1","keys":{"p256dh":"a","auth":"b"}}`); c != 200 {
		t.Fatalf("subscribe = %d", c)
	}
	if len(d.push.subs) != 1 {
		t.Fatal("not stored")
	}
	if c := post("DELETE", `{"endpoint":"https://x/1"}`); c != 200 || len(d.push.subs) != 0 {
		t.Fatalf("unsubscribe = %d subs=%d", c, len(d.push.subs))
	}
}
