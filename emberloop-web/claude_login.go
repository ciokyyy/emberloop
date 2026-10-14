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
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/creack/pty"
)

const (
	claudeLoginTimeout    = 10 * time.Minute
	claudeLoginOutputMax  = 64 << 10
	claudeLoginCodeMaxLen = 1024
)

var (
	errClaudeLoginAlreadyRunning = errors.New("a Claude login is already running")
	errClaudeLoginNotRunning     = errors.New("Claude login is not running")
	errClaudeCLINotFound         = errors.New("Claude Code CLI was not found in the Daemon PATH")
)

var terminalANSISequence = regexp.MustCompile(`\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)

func stripTerminalANSI(output []byte) []byte {
	return terminalANSISequence.ReplaceAll(output, nil)
}

type claudeLoginSession struct {
	mu        sync.Mutex
	id        string
	state     string
	output    []byte
	errorText string
	cmd       *exec.Cmd
	terminal  *os.File
	cancel    context.CancelFunc
}

type claudeLoginView struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

func (s *claudeLoginSession) snapshot() claudeLoginView {
	s.mu.Lock()
	defer s.mu.Unlock()
	return claudeLoginView{ID: s.id, State: s.state, Output: string(stripTerminalANSI(s.output)), Error: s.errorText}
}

func (s *claudeLoginSession) appendOutput(output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output = append(s.output, output...)
	if len(s.output) > claudeLoginOutputMax {
		s.output = append([]byte(nil), s.output[len(s.output)-claudeLoginOutputMax:]...)
	}
}

func (s *claudeLoginSession) readOutput(done chan<- struct{}) {
	defer close(done)
	buffer := make([]byte, 4096)
	for {
		n, err := s.terminal.Read(buffer)
		if n > 0 {
			s.appendOutput(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

func (s *claudeLoginSession) wait(readerDone <-chan struct{}) {
	waitErr := s.cmd.Wait()
	<-readerDone
	_ = s.terminal.Close()
	s.mu.Lock()
	if s.state == "running" {
		switch {
		case waitErr == nil:
			s.state = "succeeded"
		default:
			s.state = "failed"
			s.errorText = fmt.Sprintf("claude auth login exited unsuccessfully: %v", waitErr)
		}
	}
	s.mu.Unlock()
	s.cancel()
}

func (s *claudeLoginSession) sendCode(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != "running" {
		return errClaudeLoginNotRunning
	}
	_, err := io.WriteString(s.terminal, code+"\n")
	return err
}

func (s *claudeLoginSession) stop() {
	s.mu.Lock()
	if s.state == "running" {
		s.state = "canceled"
		s.errorText = "Claude login canceled."
		s.cancel()
		_ = s.terminal.Close()
	}
	s.mu.Unlock()
}

func newClaudeLoginID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func ensureClaudeLoginTerminalEnv(env []string) []string {
	if env == nil {
		env = os.Environ()
	}
	for i, entry := range env {
		if strings.HasPrefix(entry, "TERM=") {
			if entry == "TERM=" {
				env[i] = "TERM=xterm-256color"
			}
			return env
		}
	}
	return append(env, "TERM=xterm-256color")
}

func startClaudeLoginPTY(cmd *exec.Cmd) (*os.File, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, err
	}
	if err := pty.Setsize(master, &pty.Winsize{Rows: 40, Cols: 240}); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, fmt.Errorf("size Claude login PTY: %w", err)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = ensureClaudeLoginTerminalEnv(cmd.Env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, err
	}
	_ = slave.Close()
	return master, nil
}

func (d *Daemon) startClaudeLogin() (*claudeLoginSession, error) {
	d.claudeLoginMu.Lock()
	defer d.claudeLoginMu.Unlock()
	if d.claudeLogin != nil && d.claudeLogin.snapshot().State == "running" {
		return nil, errClaudeLoginAlreadyRunning
	}

	binary, err := exec.LookPath("claude")
	if err != nil {
		return nil, errClaudeCLINotFound
	}
	id, err := newClaudeLoginID()
	if err != nil {
		return nil, errors.New("could not create Claude login session")
	}
	ctx, cancel := context.WithTimeout(context.Background(), claudeLoginTimeout)
	cmd := exec.CommandContext(ctx, binary, "auth", "login")
	terminal, err := startClaudeLoginPTY(cmd)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start claude auth login: %w", err)
	}
	session := &claudeLoginSession{
		id:       id,
		state:    "running",
		cmd:      cmd,
		terminal: terminal,
		cancel:   cancel,
	}
	d.claudeLogin = session
	readerDone := make(chan struct{})
	go session.readOutput(readerDone)
	go session.wait(readerDone)
	return session, nil
}

func (d *Daemon) getClaudeLogin(id string) *claudeLoginSession {
	d.claudeLoginMu.Lock()
	defer d.claudeLoginMu.Unlock()
	if d.claudeLogin == nil || d.claudeLogin.id != id {
		return nil
	}
	return d.claudeLogin
}

func (d *Daemon) stopClaudeLogin() {
	d.claudeLoginMu.Lock()
	session := d.claudeLogin
	d.claudeLoginMu.Unlock()
	if session != nil {
		session.stop()
	}
}

func claudeLoginCodeRequest(w http.ResponseWriter, r *http.Request) (string, error) {
	var body struct {
		Code string `json:"code"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		return "", errors.New("a login code is required")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", errors.New("invalid request body")
	}
	body.Code = strings.TrimSpace(body.Code)
	if body.Code == "" || len(body.Code) > claudeLoginCodeMaxLen {
		return "", errors.New("a valid login code is required")
	}
	for _, r := range body.Code {
		if unicode.IsControl(r) {
			return "", errors.New("login code must be a single line")
		}
	}
	return body.Code, nil
}

func (d *Daemon) requireClaudeLoginAdmin(w http.ResponseWriter, r *http.Request) bool {
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

func (d *Daemon) handleClaudeLogin(w http.ResponseWriter, r *http.Request) {
	if !d.requireClaudeLoginAdmin(w, r) {
		return
	}
	if r.URL.Path != "/api/providers/claude/login" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	session, err := d.startClaudeLogin()
	if err != nil {
		if errors.Is(err, errClaudeLoginAlreadyRunning) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		if errors.Is(err, errClaudeCLINotFound) {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, session.snapshot())
}

func (d *Daemon) handleClaudeLoginSub(w http.ResponseWriter, r *http.Request) {
	if !d.requireClaudeLoginAdmin(w, r) {
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 5 || parts[0] != "api" || parts[1] != "providers" || parts[2] != "claude" || parts[3] != "login" || parts[4] == "" {
		http.NotFound(w, r)
		return
	}
	session := d.getClaudeLogin(parts[4])
	if session == nil {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 5 {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, session.snapshot())
		case http.MethodDelete:
			session.stop()
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) != 6 || parts[5] != "code" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code, err := claudeLoginCodeRequest(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := session.sendCode(code); err != nil {
		if errors.Is(err, errClaudeLoginNotRunning) {
			http.Error(w, err.Error(), http.StatusConflict)
		} else {
			http.Error(w, "could not send Claude login code", http.StatusGone)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
