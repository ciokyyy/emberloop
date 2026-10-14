package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/MichaelKinsy/PiG/ai"
)

const (
	chatGPTDeviceCodePath     = "/api/providers/openai-codex/device-code"
	chatGPTDeviceCodeLifetime = 15 * time.Minute
	chatGPTLoginRetention     = 5 * time.Minute
)

type chatGPTDeviceLogin struct {
	id              string
	status          string
	userCode        string
	verificationURI string
	expiresAt       time.Time
	error           string
	finishedAt      time.Time
	cancel          context.CancelFunc
	done            chan struct{}
}

type chatGPTDeviceLoginView struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	UserCode        string `json:"userCode,omitempty"`
	VerificationURI string `json:"verificationUrl,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	Error           string `json:"error,omitempty"`
}

func (login *chatGPTDeviceLogin) view() chatGPTDeviceLoginView {
	view := chatGPTDeviceLoginView{ID: login.id, Status: login.status, UserCode: login.userCode, VerificationURI: login.verificationURI, Error: login.error}
	if !login.expiresAt.IsZero() {
		view.ExpiresAt = login.expiresAt.UnixMilli()
	}
	return view
}

func newChatGPTDeviceLoginID() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func chatGPTLoginActive(status string) bool {
	return status == "starting" || status == "waiting"
}

// expireChatGPTLoginsLocked expires active logins against the Daemon clock and
// removes terminal snapshots after a short grace period for the browser.
func (d *Daemon) expireChatGPTLoginsLocked(now time.Time) []context.CancelFunc {
	var cancels []context.CancelFunc
	for id, login := range d.chatGPTLogins {
		if chatGPTLoginActive(login.status) && !now.Before(login.expiresAt) {
			login.status = "expired"
			login.userCode = ""
			login.verificationURI = ""
			login.finishedAt = now
			d.scheduleChatGPTLoginCleanupLocked(login)
			cancels = append(cancels, login.cancel)
		}
		if !chatGPTLoginActive(login.status) && !login.finishedAt.IsZero() && !now.Before(login.finishedAt.Add(chatGPTLoginRetention)) {
			delete(d.chatGPTLogins, id)
		}
	}
	return cancels
}

func cancelChatGPTLogins(cancels []context.CancelFunc) {
	for _, cancel := range cancels {
		if cancel != nil {
			cancel()
		}
	}
}

func (d *Daemon) scheduleChatGPTLoginCleanupLocked(login *chatGPTDeviceLogin) {
	time.AfterFunc(chatGPTLoginRetention, func() {
		d.providerLoginMu.Lock()
		if d.chatGPTLogins[login.id] == login && !chatGPTLoginActive(login.status) {
			delete(d.chatGPTLogins, login.id)
		}
		d.providerLoginMu.Unlock()
	})
}
func (d *Daemon) cancelChatGPTDeviceLogins() {
	now := d.now()
	d.providerLoginMu.Lock()
	var cancels []context.CancelFunc
	for _, login := range d.chatGPTLogins {
		if chatGPTLoginActive(login.status) {
			login.status = "canceled"
			login.userCode = ""
			login.verificationURI = ""
			login.finishedAt = now
			d.scheduleChatGPTLoginCleanupLocked(login)
			cancels = append(cancels, login.cancel)
		}
	}
	d.providerLoginMu.Unlock()
	cancelChatGPTLogins(cancels)
}

func (d *Daemon) handleChatGPTDeviceCodeStart(w http.ResponseWriter, r *http.Request) {
	if !d.requireProviderAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := newChatGPTDeviceLoginID()
	if err != nil {
		http.Error(w, "could not start ChatGPT login", http.StatusInternalServerError)
		return
	}
	now := d.now()
	ctx, cancel := context.WithTimeout(context.Background(), chatGPTDeviceCodeLifetime)
	login := &chatGPTDeviceLogin{
		id:        id,
		status:    "starting",
		expiresAt: now.Add(chatGPTDeviceCodeLifetime),
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	d.providerLoginMu.Lock()
	if d.chatGPTLogins == nil {
		d.chatGPTLogins = make(map[string]*chatGPTDeviceLogin)
	}
	cancels := d.expireChatGPTLoginsLocked(now)
	active := false
	for _, existing := range d.chatGPTLogins {
		if chatGPTLoginActive(existing.status) {
			active = true
			break
		}
	}
	if !active {
		d.chatGPTLogins[id] = login
	}
	d.providerLoginMu.Unlock()
	cancelChatGPTLogins(cancels)
	if active {
		cancel()
		http.Error(w, "a ChatGPT login is already in progress", http.StatusConflict)
		return
	}

	view := login.view()
	go d.runChatGPTDeviceLogin(ctx, login)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusAccepted, map[string]any{"login": view})
}

func (d *Daemon) handleChatGPTDeviceCodeSession(w http.ResponseWriter, r *http.Request) {
	if !d.requireProviderAdmin(w, r) {
		return
	}
	id := strings.TrimPrefix(r.URL.Path, chatGPTDeviceCodePath+"/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}

	d.providerLoginMu.Lock()
	cancels := d.expireChatGPTLoginsLocked(d.now())
	login := d.chatGPTLogins[id]
	if login == nil {
		d.providerLoginMu.Unlock()
		cancelChatGPTLogins(cancels)
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		if chatGPTLoginActive(login.status) {
			login.status = "canceled"
			login.userCode = ""
			login.verificationURI = ""
			login.finishedAt = d.now()
			d.scheduleChatGPTLoginCleanupLocked(login)
			cancels = append(cancels, login.cancel)
		}
	} else if r.Method != http.MethodGet {
		d.providerLoginMu.Unlock()
		cancelChatGPTLogins(cancels)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := login.view()
	d.providerLoginMu.Unlock()
	cancelChatGPTLogins(cancels)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"login": view})
}

func (d *Daemon) runChatGPTDeviceLogin(ctx context.Context, login *chatGPTDeviceLogin) {
	defer close(login.done)
	defer login.cancel()

	creds, err := (ai.CodexOAuthProvider{}).LoginContext(ctx, ai.OAuthLoginCallbacks{
		OnSelect: func(ai.OAuthSelectPrompt) (string, error) {
			return ai.OpenAICodexDeviceCodeLoginMethod, nil
		},
		OnDeviceCode: func(info ai.OAuthDeviceCodeInfo) {
			d.providerLoginMu.Lock()
			defer d.providerLoginMu.Unlock()
			if d.chatGPTLogins[login.id] != login || !chatGPTLoginActive(login.status) {
				return
			}
			expiresIn := chatGPTDeviceCodeLifetime
			if !math.IsNaN(info.ExpiresInSeconds) && info.ExpiresInSeconds > 0 && info.ExpiresInSeconds <= chatGPTDeviceCodeLifetime.Seconds() {
				expiresIn = time.Duration(info.ExpiresInSeconds * float64(time.Second))
			}
			login.status = "waiting"
			login.userCode = info.UserCode
			login.verificationURI = info.VerificationURI
			login.expiresAt = d.now().Add(expiresIn)
		},
	})
	if err != nil {
		status, publicError := "failed", "ChatGPT device-code login failed. Try again."
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status, publicError = "expired", ""
		} else if errors.Is(ctx.Err(), context.Canceled) {
			status, publicError = "canceled", ""
		}
		d.finishChatGPTDeviceLogin(login, status, publicError)
		return
	}
	if d.storeAndCompleteChatGPTDeviceLogin(login, creds) {
		go func() {
			quotaCtx, cancel := context.WithTimeout(context.Background(), quotaTimeout)
			defer cancel()
			d.refreshQuota(quotaCtx, "openai-codex", d.now(), true)
		}()
	}
}

func (d *Daemon) storeChatGPTCredentials(creds ai.OAuthCredentials) error {
	store, err := ai.NewAuthStorage(d.providerAuthPath())
	if err != nil {
		return err
	}
	credential := ai.Credential{
		Type:      ai.CredentialOAuth,
		Access:    creds.Access,
		Refresh:   creds.Refresh,
		AccountID: creds.AccountID,
	}
	credential.SetExpiresMillis(creds.ExpiresMillis())
	return store.Set("openai-codex", credential)
}

func (d *Daemon) finishChatGPTDeviceLogin(login *chatGPTDeviceLogin, status, publicError string) bool {
	d.providerLoginMu.Lock()
	defer d.providerLoginMu.Unlock()
	if d.chatGPTLogins[login.id] != login || !chatGPTLoginActive(login.status) {
		return false
	}
	login.status = status
	login.error = publicError
	login.userCode = ""
	login.verificationURI = ""
	login.finishedAt = d.now()
	d.scheduleChatGPTLoginCleanupLocked(login)
	return true
}

func (d *Daemon) storeAndCompleteChatGPTDeviceLogin(login *chatGPTDeviceLogin, creds ai.OAuthCredentials) bool {
	d.providerLoginMu.Lock()
	defer d.providerLoginMu.Unlock()
	if d.chatGPTLogins[login.id] != login || !chatGPTLoginActive(login.status) {
		return false
	}
	if err := d.storeChatGPTCredentials(creds); err != nil {
		login.status = "failed"
		login.error = "Could not save ChatGPT login to pig auth.json."
		login.userCode = ""
		login.verificationURI = ""
		login.finishedAt = d.now()
		d.scheduleChatGPTLoginCleanupLocked(login)
		return false
	}
	login.status = "complete"
	login.userCode = ""
	login.verificationURI = ""
	login.finishedAt = d.now()
	d.scheduleChatGPTLoginCleanupLocked(login)
	return true
}

func (d *Daemon) requireProviderAdmin(w http.ResponseWriter, r *http.Request) bool {
	requester, ok := d.requesterOf(r)
	if !ok || requester.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if !requester.user.Admin {
		http.Error(w, "admin only", http.StatusForbidden)
		return false
	}
	return true
}
