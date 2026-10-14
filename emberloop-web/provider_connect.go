package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	antigravity "example.com/pig-antigravity-auth"
	"github.com/MichaelKinsy/PiG/ai"
)

const (
	antigravityConnectPath      = "/api/providers/google-antigravity/connect"
	antigravityConnectTimeout   = 10 * time.Minute
	antigravityConnectRetention = 10 * time.Minute
)

type antigravityConnectSnapshot struct {
	ID           string `json:"id"`
	State        string `json:"state"`
	URL          string `json:"url,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Prompt       string `json:"prompt,omitempty"`
	Progress     string `json:"progress,omitempty"`
	Error        string `json:"error,omitempty"`
}

type antigravityConnectSession struct {
	mu      sync.Mutex
	view    antigravityConnectSnapshot
	input   chan string
	changed chan struct{}
	cancel  context.CancelFunc
}

func newAntigravityConnectSession(id string, cancel context.CancelFunc) *antigravityConnectSession {
	return &antigravityConnectSession{
		view:    antigravityConnectSnapshot{ID: id, State: "starting"},
		changed: make(chan struct{}),
		cancel:  cancel,
	}
}

func (s *antigravityConnectSession) update(change func(*antigravityConnectSnapshot)) {
	s.mu.Lock()
	change(&s.view)
	changed := s.changed
	s.changed = make(chan struct{})
	close(changed)
	s.mu.Unlock()
}

func (s *antigravityConnectSession) snapshot() (antigravityConnectSnapshot, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view, s.changed
}

func connectReady(view antigravityConnectSnapshot) bool {
	return view.Prompt != "" || view.State == "complete" || view.State == "error" || view.State == "cancelled"
}

func antigravityConnectTerminal(state string) bool {
	return state == "complete" || state == "error" || state == "cancelled"
}

func (s *antigravityConnectSession) waitReady(ctx context.Context) (antigravityConnectSnapshot, error) {
	for {
		view, changed := s.snapshot()
		if connectReady(view) {
			return view, nil
		}
		select {
		case <-ctx.Done():
			return antigravityConnectSnapshot{}, ctx.Err()
		case <-changed:
		}
	}
}

func (s *antigravityConnectSession) prompt(ctx context.Context, message string) (string, error) {
	input := make(chan string, 1)
	s.mu.Lock()
	s.input = input
	s.view.State = "waiting"
	s.view.Prompt = message
	changed := s.changed
	s.changed = make(chan struct{})
	close(changed)
	s.mu.Unlock()

	select {
	case value := <-input:
		s.mu.Lock()
		if s.input == input {
			s.input = nil
			s.view.Prompt = ""
			s.view.State = "progress"
			changed := s.changed
			s.changed = make(chan struct{})
			close(changed)
		}
		s.mu.Unlock()
		return value, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *antigravityConnectSession) submit(value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if antigravityConnectTerminal(s.view.State) {
		return errors.New("this Antigravity login is no longer active")
	}
	if s.input == nil || s.view.State != "waiting" {
		return errors.New("Antigravity login is not waiting for input")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("input is required")
	}
	select {
	case s.input <- value:
		s.view.Error = ""
		changed := s.changed
		s.changed = make(chan struct{})
		close(changed)
		return nil
	default:
		return errors.New("Antigravity login input was already submitted")
	}
}

func (s *antigravityConnectSession) cancelFlow() {
	s.mu.Lock()
	if antigravityConnectTerminal(s.view.State) {
		s.mu.Unlock()
		return
	}
	s.view.State = "cancelled"
	s.view.Prompt = ""
	cancel := s.cancel
	changed := s.changed
	s.changed = make(chan struct{})
	close(changed)
	s.mu.Unlock()
	cancel()
}

func (s *antigravityConnectSession) finish(state, message string) {
	s.update(func(view *antigravityConnectSnapshot) {
		if antigravityConnectTerminal(view.State) {
			return
		}
		view.State = state
		view.Prompt = ""
		view.Error = message
	})
}

func (d *Daemon) startAntigravityConnect(ctx context.Context) (antigravityConnectSnapshot, error) {
	var randomID [16]byte
	if _, err := rand.Read(randomID[:]); err != nil {
		return antigravityConnectSnapshot{}, fmt.Errorf("create Antigravity login id: %w", err)
	}
	id := hex.EncodeToString(randomID[:])
	loginCtx, cancel := context.WithTimeout(context.Background(), antigravityConnectTimeout)
	session := newAntigravityConnectSession(id, cancel)
	d.providerConnectMu.Lock()
	if d.providerConnects == nil {
		d.providerConnects = make(map[string]*antigravityConnectSession)
	}
	d.providerConnects[id] = session
	d.providerConnectMu.Unlock()

	go d.runAntigravityConnect(loginCtx, id, session)
	view, err := session.waitReady(ctx)
	if err != nil {
		session.cancelFlow()
		d.forgetAntigravityConnect(id, session)
		return antigravityConnectSnapshot{}, err
	}
	return view, nil
}

func (d *Daemon) runAntigravityConnect(ctx context.Context, id string, session *antigravityConnectSession) {
	defer session.cancel()
	credentials, err := antigravity.LoginWithCallbacks(ctx, antigravity.LoginCallbacks{
		OnAuth: func(url, instructions string) {
			session.update(func(view *antigravityConnectSnapshot) {
				view.URL = url
				view.Instructions = instructions
			})
		},
		OnPrompt: func(message string) (string, error) {
			return session.prompt(ctx, message)
		},
		OnProgress: func(message string) {
			session.update(func(view *antigravityConnectSnapshot) {
				view.State = "progress"
				view.Progress = message
			})
		},
		OnInfo: func(message string) {
			session.update(func(view *antigravityConnectSnapshot) {
				view.Error = message
			})
		},
	})
	if err != nil {
		state := "error"
		if errors.Is(err, context.Canceled) {
			state = "cancelled"
		}
		session.finish(state, err.Error())
		d.expireAntigravityConnect(id, session)
		return
	}

	raw, err := json.Marshal(credentials)
	if err != nil {
		session.finish("error", "could not encode Antigravity credentials")
		d.expireAntigravityConnect(id, session)
		return
	}
	var credential ai.Credential
	if err := json.Unmarshal(raw, &credential); err != nil || credential.Access == "" || credential.Refresh == "" {
		session.finish("error", "Antigravity returned incomplete OAuth credentials")
		d.expireAntigravityConnect(id, session)
		return
	}
	credential.Type = ai.CredentialOAuth
	store, err := ai.NewAuthStorage(d.providerAuthPath())
	if err == nil {
		err = store.Set(antigravityProviderID, credential)
	}
	if err != nil {
		session.finish("error", "could not save Antigravity credentials")
		d.expireAntigravityConnect(id, session)
		return
	}
	session.finish("complete", "")
	d.expireAntigravityConnect(id, session)
}

func (d *Daemon) antigravityConnect(id string) *antigravityConnectSession {
	d.providerConnectMu.Lock()
	defer d.providerConnectMu.Unlock()
	return d.providerConnects[id]
}

func (d *Daemon) expireAntigravityConnect(id string, session *antigravityConnectSession) {
	time.AfterFunc(antigravityConnectRetention, func() {
		d.forgetAntigravityConnect(id, session)
	})
}

func (d *Daemon) forgetAntigravityConnect(id string, session *antigravityConnectSession) {
	d.providerConnectMu.Lock()
	if d.providerConnects[id] == session {
		delete(d.providerConnects, id)
	}
	d.providerConnectMu.Unlock()
}

func (d *Daemon) handleAntigravityConnect(w http.ResponseWriter, r *http.Request) {
	requester, ok := d.requesterOf(r)
	if !ok || requester.user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if !requester.user.Admin {
		http.Error(w, "admin only", http.StatusForbidden)
		return
	}

	if r.URL.Path == antigravityConnectPath {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		view, err := d.startAntigravityConnect(r.Context())
		if err != nil {
			http.Error(w, "could not start Antigravity login", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, http.StatusCreated, view)
		return
	}

	prefix := antigravityConnectPath + "/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, prefix)
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	session := d.antigravityConnect(id)
	if session == nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		view, _ := session.snapshot()
		writeJSON(w, http.StatusOK, view)
	case http.MethodPost:
		var body struct {
			Value string `json:"value"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			http.Error(w, "input is required", http.StatusBadRequest)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := session.submit(body.Value); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
	case http.MethodDelete:
		session.cancelFlow()
		d.expireAntigravityConnect(id, session)
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
