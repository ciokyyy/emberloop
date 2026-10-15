package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"example.com/emberloop-web/el/tools/systemone"
	"example.com/emberloop-web/memory"
	"github.com/MichaelKinsy/PiG/ai"
)

// Backend drives one harness behind the Daemon. The fake Backend replays
// scripted Events; real Backends (pig in ticket 03, claude in ticket 04)
// drive the harness SDK. Backends are registered per harness; the default
// Backend serves any harness without one.
type Backend interface {
	// OnPrompt runs after the prompt Event is stored. It appends follow-up
	// Events through appendEvent.
	OnPrompt(sessionID, text string, appendEvent func(kind, text, status string))
	// CreateSession allocates the harness Session; emit streams its Events.
	// An error rejects the create with a 400.
	CreateSession(meta SessionMeta, emit func(kind, text, status string)) error
	// CloseSession releases the harness Session without deleting history.
	CloseSession(sessionID string)
	// Models lists the harness's runnable models.
	Models() []string
	// ReleaseSession releases the agent process without deleting history
	// and returns the token ResumeSession needs ("" when none).
	ReleaseSession(sessionID string) string
	// ResumeSession reallocates the agent process for a released Session
	// using meta.ResumeToken.
	ResumeSession(meta SessionMeta, emit func(kind, text, status string)) error
	// ResumeToken reports the current live handle for persistence.
	ResumeToken(sessionID string) string
	// ResumeOK reports whether a released Session can take a new prompt,
	// with the why-not when it cannot.
	ResumeOK(meta SessionMeta) (bool, string)
}

// Daemon owns every Session, writes Event logs and serves the API.
type Daemon struct {
	// allowHosts are extra Host names the guard accepts (see SetAllowedHosts).
	allowHosts []string
	// allowTailnet accepts Host names that are Tailscale addresses (100.64.0.0/10).
	allowTailnet bool
	// sttURL overrides the OpenRouter transcription endpoint (tests).
	sttURL string
	// sttChatURL overrides the OpenRouter chat endpoint for the STT format pass (tests).
	sttChatURL string
	// sttKeyFn overrides the OpenRouter key lookup (tests).
	sttKeyFn func(context.Context) string
	stateDir string
	// quotaMu guards refresh errors, the manual cooldown and the last quota
	// snapshot published to live subscribers.
	quotaMu              sync.RWMutex
	quotaErrors          map[string]error
	quotaManualRefreshAt map[string]time.Time
	quotaPublishedJSON   []byte
	quotaPublishedReady  bool
	// quotaLiveMu starts one background refresher for the first quota-topic
	// subscriber and cancels it when the last subscriber leaves.
	quotaLiveMu          sync.Mutex
	quotaLiveSubscribers int
	quotaLiveCancel      context.CancelFunc
	quotaLiveWG          sync.WaitGroup
	// hostClaudeDir and hostPigDir are the host's own harness dirs. Owner
	// dirs are seeded from them (harnessdirs.go); no harness starts on them.
	hostClaudeDir string
	hostPigDir    string
	// providerLoginMu protects ChatGPT's short-lived device-code login sessions.
	providerLoginMu sync.Mutex
	chatGPTLogins   map[string]*chatGPTDeviceLogin
	// claudeLoginMu guards the one short-lived host Claude OAuth PTY session.
	claudeLoginMu sync.Mutex
	claudeLogin   *claudeLoginSession
	// providerConnectMu guards Antigravity's short-lived interactive login flow.
	providerConnectMu sync.Mutex
	providerConnects  map[string]*antigravityConnectSession
	// dirsMu serializes owner dir seeding across concurrent starts.
	dirsMu sync.Mutex
	// users are the accounts under stateDir. machineToken is the bearer that
	// local machine clients send (see auth.go).
	users        *userStore
	machineToken string
	// summarizer writes Handoffs for Compress (summarize.go); tests replace it.
	summarizer handoffSummarizer
	// decider confirms mined candidates (decider.go); nil builds it from the
	// decider settings on every mine. Tests set a scripted Decider.
	decider memory.Decider
	// deciderOneShot scripts the Decider's one-off completion in tests; nil
	// runs the bare pig one-shot.
	deciderOneShot func(model, system, user string) (string, error)
	// compressSleep waits between compress retries (compress_retry.go). Nil
	// sleeps for real; tests replace it.
	compressSleep func(time.Duration)
	// projects are the recorded working directories; projMu guards them.
	projMu   sync.Mutex
	projects []Project
	// projectPickKeyFn and projectPickProvider override Jev for the draft
	// project picker; nil uses systemone.APIKey and OpenRouter.
	projectPickKeyFn    func(context.Context) string
	projectPickProvider systemone.Provider
	backend             Backend
	live                *liveHub
	presence            presence
	push                *pushStore
	claude              *ClaudeBackend
	// maxTickets overrides the max_tickets setting when positive.
	maxTickets int
	// starting counts Sessions reserving a running slot while their
	// backend handle builds before publish.
	starting int
	mu       sync.Mutex
	sessions map[string]*session
	backends map[string]Backend
	// groups tracks one spawn call each: its parent and members, so the
	// last member to finish delivers one grouped result as a Steer.
	groups map[string]*spawnGroup
	// runs tracks one deterministic Run each: its Tickets and their Worker
	// Sessions. submits holds every Session's Worker submits by Session id,
	// which the driver loop reads for merges and checkbox ticks. stopSubmit
	// arms one Session each whose accepted terminal submit still waits for the
	// submit tool's terminal update before its turn is stopped
	// (runs_stopsubmit.go). Memory only: a restart clears it and the turn
	// simply runs on.
	runs map[string]*runState
	// runSessionIndex maps Run Tickets to their direct Sessions and descendants;
	// runSessionOwners lets a newly attached child inherit its Ticket.
	runSessionIndex  map[runTicketRef]map[string]struct{}
	runSessionOwners map[string]runTicketRef
	// runGitCache holds short-lived git results per Run and Ticket; Run events
	// invalidate that Run's entries.
	runGitCache runGitResultCache
	submits     map[string]*submitRec
	stopSubmit  map[string]bool
	// sudo holds the pending and settled sudo approval requests (sudo.go).
	sudoOnce sync.Once
	sudo     *sudoStore
	// memEng is the per-User memory engine (profile.go). memMu guards lazy
	// open; the engine guards its own banks. Separate from mu so resume and
	// compress paths that hold mu can still read the Profile.
	memMu  sync.Mutex
	memEng *memory.Engine
	// rememberMu serializes an agent remember's limit check with its enqueue,
	// so concurrent calls in one Session cannot pass the limit together and
	// exceed it. It is Daemon-wide (not per Session): the critical section is
	// two quick bank calls, and no path takes it while holding mu.
	rememberMu sync.Mutex
	// restartCh fires once after RequestRestart; the binary waits on it,
	// drains and re-executes. restartReq guards repeats, restartAt dates the
	// request so a repeat can re-arm a stuck one, and restartDrain is the
	// drain the binary waits (restart.go).
	restartCh    chan struct{}
	restartReq   bool
	restartAt    time.Time
	restartDrain time.Duration
	// shuttingDown is set once Shutdown starts closing Sessions: a turn error
	// from a closing process is a restart, not a failure (restart.go).
	shuttingDown bool
	// stopRestart marks the Run Worker and Reviewer Sessions a restart
	// request stops at their next tool-call boundary (runs_restart_resume.go).
	stopRestart map[string]bool
	// clock is the Daemon clock seam: idle quiet time and the runs driver read
	// it; nil reads time.Now (idle.go).
	clock func() time.Time
	// settingsPath points this Daemon at a settings file for tests; empty
	// reads the host settings file. Set it before serving: tests give each
	// parallel Daemon its own file instead of sharing EL_SETTINGS.
	settingsPath string
	// pollInterval caps this Daemon's Run driver sleep; zero keeps the
	// production default. Tests set it before serving so multi-pass Run
	// tests poll fast. Zero means runPollInterval.
	pollInterval time.Duration
	// Run final-merge hooks replace the generated-file build and repo check in seam tests.
	runFinalBuild func(string) (string, error)
	runRepoCheck  func(string) (string, error)
	// Nightly job (nightly.go): distillOneShot and profileOneShot script the
	// cheap-LLM calls in tests; nil runs the bare pig one-shot. nightlyFailStage
	// forces one stage to fail (the test seam for abort and continue paths).
	// nightlyMu serializes runs; nightlyLastDay is the last auto-run local day.
	nightlyMu          sync.Mutex
	distillOneShot     func(model, system, user string) (string, error)
	profileOneShot     func(model, system, user string) (string, error)
	consolidateOneShot func(model, system, user string) (string, error)
	nightlyFailStage   string
	// baseCheck scripts the spec-branch base-red gate in tests; nil runs
	// .emberloop/check at the requested tip in a detached worktree.
	baseCheck func(repo, specBranch, tip string) (bool, []string)
	// tipCheck scripts the per-tip repo check before Reviewers in tests; nil
	// runs .emberloop/check at the requested tip in a detached worktree.
	tipCheck       func(repo, ticketBranch, tip string) (bool, []string)
	nightlyLastDay string
	// Turn-end ingestion (mine_live.go): liveMine turns it on, mineState
	// holds one in-flight/again flag per Session, mineWG lets tests and
	// Shutdown wait for running scans, mineActive counts them for the
	// restart state, and mineWait bounds Shutdown's wait (0 takes 30s).
	liveMine   bool
	mineMu     sync.Mutex
	mineState  map[string]*mineFlight
	mineActive int
	mineWG     sync.WaitGroup
	mineWait   time.Duration
	// Disk hygiene (disk.go): last janitor result and last low-disk warn,
	// guarded by diskMu so the janitor goroutine and /api/disk race no one.
	diskMu       sync.Mutex
	diskLast     janitorResult
	diskWarnedAt time.Time
}

type session struct {
	meta SessionMeta
	// titleTried is set once a title was attempted, so a failure is not retried.
	titleTried bool
	events     []Event
	notify     chan struct{}
	last       time.Time
	// live tells whether the backend holds an agent process. Released
	// and never-started Sessions keep history only.
	live bool
	// active counts launched turns still running. A release waits for
	// zero, so it never drops a process a new prompt just took.
	active int
	// busy marks a turn in flight. Messages arriving while busy queue
	// in pending (fake and claude) or in the harness native queue (pig)
	// and deliver after the turn. Only the turn runner mutates these
	// under d.mu.
	busy    bool
	pending []queuedMsg
	// openTools holds the tool calls in flight by ToolCall id, opened on
	// tool_call_started and closed on a completed or failed
	// tool_call_updated, stamped on the Daemon clock. The Run driver reads
	// it beside the working status, so a long foreground tool call (a
	// minutes-long go test) dates worker activity. Memory only: a restart
	// ends every turn, so nothing in flight survives one.
	openTools map[string]time.Time
	// gateSkip marks a turn the user stopped (interrupt, cancel): the submit
	// gate takes it as a deliberate stop, not a missing submit, so no
	// reminder follows it. The gate consumes it.
	gateSkip bool
	// restartStopAttempt and restartStopRetry coordinate a restart interrupt
	// with a Backend turn that may not have started when the Session became busy.
	restartStopAttempt bool
	restartStopRetry   bool
	// resuming marks a resume in flight. A second send on a released
	// Session gets 409 instead of double-resuming past the cap.
	// releasing marks a release in flight. Sends answer 409 while it
	// runs, so a stale release never kills a freshly resumed process.
	resuming  bool
	releasing bool
	// compressWait holds a compress until no Group of the Session is
	// outstanding; compressInstr keeps its instructions for the wait.
	// compressAuto marks a wait the automatic check armed: runDeferredCompress
	// re-verifies the meter before running one, while a manual wait always
	// runs. Request paths clear all three together.
	compressWait  bool
	compressAuto  bool
	compressInstr string
	// swapping marks a Handoff from its claim until its turn ends.
	swapping bool
	// inject holds the memory bundle the next turn carries ahead of its
	// prompt (handle.go). A fresh handle sets it; the first turn takes it.
	inject injection
	// injectOp is the handle op that set inject: create builds fresh recall
	// from the prompt, resume carries the Profile only, compress already
	// carries fresh recall from the Handoff.
	injectOp handleOp
}

// queuedMsg is one not-yet-delivered message. Mode is prompt, steer or
// followup; degraded marks a claude Steer stored as a Follow-up.
type queuedMsg struct {
	mode     string
	text     string
	images   []Image
	imageIDs []string
	degraded bool
	// held marks a claude Steer waiting for the next tool call to finish.
	held bool
	// from types the message: the User who sent it, or the system.
	from promptSender
}

// steerBackend is the harness native queue seam. Pig implements it; the
// Daemon owns the queue for every other harness. Steer and FollowUp report
// whether pig queued the message; false means the shim was unreachable and
// the Daemon must hold the message itself.
type steerBackend interface {
	Steer(sessionID, text string) bool
	FollowUp(sessionID, text string) bool
	QueuedMessages(sessionID string) []QueuedMessage
	ClearQueued(sessionID string) int
}

// imageSteerBackend is the optional native-queue seam for steers and
// follow-ups that carry images. See steerBackend for the return.
type imageSteerBackend interface {
	SteerImages(sessionID, text string, images []Image) bool
	FollowUpImages(sessionID, text string, images []Image) bool
}

// interruptBackend stops the turn a Session is running. It reports whether
// a live turn was asked to stop.
type interruptBackend interface {
	Interrupt(sessionID string) bool
}

// NewDaemon loads persisted Sessions from stateDir/enderbloom/sessions.
// Loaded Sessions hold history only; a new prompt resumes them.
func NewDaemon(stateDir string, backend Backend) *Daemon {
	d := &Daemon{
		stateDir:         stateDir,
		hostClaudeDir:    claudeHostDir(),
		hostPigDir:       userPigAgentDir(),
		backend:          backend,
		live:             newLiveHub(),
		push:             newPushStore(stateDir),
		sessions:         map[string]*session{},
		backends:         map[string]Backend{},
		groups:           map[string]*spawnGroup{},
		runs:             map[string]*runState{},
		runSessionIndex:  map[runTicketRef]map[string]struct{}{},
		runSessionOwners: map[string]runTicketRef{},
		submits:          map[string]*submitRec{},
		stopSubmit:       map[string]bool{},
		restartCh:        make(chan struct{}, 1),
	}
	d.users = newUserStore(stateDir)
	tok, err := newMachineToken(stateDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "el-web: machine token:", err)
	}
	d.machineToken = tok
	d.summarizer = pigSummarizer{}
	d.load()
	d.loadGroups()
	d.loadRuns()
	d.mu.Lock()
	d.rebuildRunSessionIndexLocked()
	d.mu.Unlock()
	d.loadProjects()
	return d
}

// RegisterBackend routes one harness to its Backend. The binary registers
// the pig Backend; tests register their own.
func (d *Daemon) RegisterBackend(harness string, be Backend) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.backends[harness] = be
}

// backendForLocked picks the harness Backend; callers hold d.mu.
func (d *Daemon) backendForLocked(harness string) Backend {
	if be, ok := d.backends[harness]; ok {
		return be
	}
	return d.backend
}

// hasBackend reports whether a Backend is registered for exactly this
// harness, with no fallback to the default.
func (d *Daemon) hasBackend(harness string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.backends[harness]
	return ok
}

// Handler returns the full HTTP handler: the loopback guard, then the cookie
// or machine-token gate on every /api route except the auth routes.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.serveUI)
	mux.HandleFunc("/api/live", d.handleLive)
	mux.HandleFunc("/api/push/", d.handlePush)
	mux.HandleFunc("/api/models", d.handleModels)
	mux.HandleFunc("/api/settings", d.handleSettings)
	mux.HandleFunc("/api/stt", d.handleSTT)
	mux.HandleFunc("/api/profile", d.handleProfile)
	mux.HandleFunc("/api/profile/versions", d.handleProfileVersions)
	mux.HandleFunc("/api/profile/restore", d.handleProfileRestore)
	mux.HandleFunc("/api/profile/queue", d.handleProfileQueue)
	mux.HandleFunc("/api/profile/queue/", d.handleProfileQueueSub)
	mux.HandleFunc("/api/rules/queue", d.handleRulesQueue)
	mux.HandleFunc("/api/rules/queue/", d.handleRulesQueueSub)
	mux.HandleFunc("/api/rules", d.handleRulesList)
	mux.HandleFunc("/api/observations", d.handleObservationsList)
	mux.HandleFunc("/api/memory/ledger", d.handleMemoryLedger)
	mux.HandleFunc("/api/memory/status", d.handleMemoryStatus)
	mux.HandleFunc("/api/memory/purge", d.handleMemoryPurge)
	mux.HandleFunc("/api/memory/nightly", d.handleNightly)
	mux.HandleFunc("/api/promotions", d.handlePromotions)
	mux.HandleFunc("/api/promotions/queue", d.handlePromotionsQueue)
	mux.HandleFunc("/api/promotions/queue/", d.handlePromotionsQueueSub)
	mux.HandleFunc("/api/sessions", d.handleSessions)
	mux.HandleFunc("/api/sessions/", d.handleSessionSub)
	mux.HandleFunc("/api/specs", d.handleSpecs)
	mux.HandleFunc("/api/specs/preview", d.handleSpecPreview)
	mux.HandleFunc("/api/dashboard", d.handleDashboard)
	mux.HandleFunc("/api/runs", d.handleRuns)
	mux.HandleFunc("/api/runs/", d.handleRunSub)
	mux.HandleFunc("/api/projects/suggest", d.handleProjectPick)
	mux.HandleFunc("/api/projects", d.handleProjects)
	mux.HandleFunc("/api/projects/", d.handleProjectSub)
	mux.HandleFunc("/api/quota", d.handleQuota)
	mux.HandleFunc("/api/quota/refresh", d.handleQuotaRefresh)
	mux.HandleFunc("/api/providers", d.handleProviders)
	mux.HandleFunc(chatGPTDeviceCodePath, d.handleChatGPTDeviceCodeStart)
	mux.HandleFunc(chatGPTDeviceCodePath+"/", d.handleChatGPTDeviceCodeSession)
	mux.HandleFunc("/api/providers/claude/login", d.handleClaudeLogin)
	mux.HandleFunc("/api/providers/claude/login/", d.handleClaudeLoginSub)
	mux.HandleFunc("/api/providers/google-antigravity/connect", d.handleAntigravityConnect)
	mux.HandleFunc("/api/providers/google-antigravity/connect/", d.handleAntigravityConnect)
	mux.HandleFunc("/api/providers/", d.handleProviderKey)
	mux.HandleFunc("/api/version", d.handleVersion)
	mux.HandleFunc("/api/health", d.handleHealth)
	mux.HandleFunc("/api/disk", d.handleDisk)
	mux.HandleFunc("/api/restart", d.handleRestart)
	mux.HandleFunc("/api/auth/signup", d.handleSignup)
	mux.HandleFunc("/api/auth/login", d.handleLogin)
	mux.HandleFunc("/api/auth/logout", d.handleLogout)
	mux.HandleFunc("/api/auth/me", d.handleMe)
	mux.HandleFunc("/api/auth/state", d.handleAuthState)
	mux.HandleFunc("/api/users", d.handleUsers)
	return guardHosts(d.requireRequester(mux), d.allowHosts, d.allowTailnet)
}

// MachineToken is the bearer that local machine clients send to this Daemon:
// agent callbacks and the el CLI. main wires it into the agent backends.
func (d *Daemon) MachineToken() string { return d.machineToken }

// WithClaudeBackend routes harness "claude" Sessions to the ACP backend.
// It also registers the backend so the generic harness lookup finds it.
// Without it claude Sessions have nowhere to run.
func (d *Daemon) WithClaudeBackend(b *ClaudeBackend) *Daemon {
	d.mu.Lock()
	defer d.mu.Unlock()
	b.tokenFile = claudeTokenPath(d.stateDir)
	d.claude = b
	d.backends["claude"] = b
	return d
}

// modelEffortOptions is the effort list for one offered model. Pig capabilities
// come from its generated model metadata; the other harnesses expose the levels
// their adapters validate for each currently offered model.
func modelEffortOptions(harness, model string) []string {
	switch harness {
	case "pig":
		for _, candidate := range ai.ListModels("") {
			if candidate.Provider+"/"+candidate.ID != model {
				continue
			}
			var efforts []string
			for _, level := range []ai.ModelThinkingLevel{
				ai.ThinkingOff, ai.ThinkingMinimal, ai.ThinkingLow,
				ai.ThinkingMedium, ai.ThinkingHigh, ai.ThinkingXHigh, ai.ThinkingMax,
			} {
				if _, ok := candidate.ThinkingLevelMap[level]; ok {
					efforts = append(efforts, string(level))
				}
			}
			return efforts
		}
	case "claude":
		if model == "opus" || model == "sonnet" {
			return append([]string(nil), claudeEfforts...)
		}
	case "grok":
		return append([]string(nil), grokEfforts...)
	}
	return []string{}
}

func modelEffortsFor(harness string, models []string) map[string][]string {
	out := make(map[string][]string, len(models))
	for _, model := range models {
		out[model] = modelEffortOptions(harness, model)
	}
	return out
}

// handleModels lists the harness's runnable models for the new-Session form.
// Pig lists only providers with usable auth; ?current= keeps the selected or
// stored model visible even when its provider has none.
func (d *Daemon) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	harness := r.URL.Query().Get("harness")
	if harness == "" {
		http.Error(w, "harness is required", http.StatusBadRequest)
		return
	}
	if harness == "claude" {
		models := ClaudeModels()
		writeJSON(w, http.StatusOK, map[string]any{"models": models, "model_efforts": modelEffortsFor(harness, models)})
		return
	}
	if harness != "pig" && harness != "grok" && harness != "fake" {
		http.Error(w, "unknown harness (want claude, pig, grok or fake)", http.StatusBadRequest)
		return
	}
	if harness == "grok" && !d.hasBackend("grok") {
		http.Error(w, "grok harness not available (grok or el-mcp missing)", http.StatusServiceUnavailable)
		return
	}
	d.mu.Lock()
	be := d.backendForLocked(harness)
	d.mu.Unlock()
	models := be.Models()
	if harness == "pig" {
		if cur := strings.TrimSpace(r.URL.Query().Get("current")); cur != "" {
			keep := true
			for _, m := range models {
				if m == cur {
					keep = false
					break
				}
			}
			if keep {
				models = append(models, cur)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "model_efforts": modelEffortsFor(harness, models)})
}

func validClaudeModel(model string) bool {
	for _, m := range ClaudeModels() {
		if m == model {
			return true
		}
	}
	return false
}

// removeSession drops a Session that failed to start, with its log and any
// backend state, so a failed claude start leaks no adapter process.
func (d *Daemon) removeSession(id string) {
	d.mu.Lock()
	delete(d.sessions, id)
	d.removeRunSessionIndexLocked(id)
	d.mu.Unlock()
	if d.claude != nil {
		d.claude.DropSession(id)
	}
	_ = os.RemoveAll(d.sessionDir(id))
}

// startError is a Session start refusal carrying its HTTP status. asJSON
// marks the {"error"} envelope; otherwise the message goes out as plain text.
type startError struct {
	code   int
	msg    string
	asJSON bool
}

func (e *startError) Error() string { return e.msg }

// startSession allocates the harness Session and publishes it live,
// enforcing the concurrency cap. It returns the owning Backend for the
// caller to run turns on. Both the create route and spawn share it, so a
// Subagent starts exactly like a Session.
func (d *Daemon) startSession(meta SessionMeta) (Backend, *startError) {
	if meta.ProjectID == "" {
		meta.ProjectID = d.projectIDForPath(meta.WorkDir)
	}
	h := startHandle(meta, handleCreate)
	// The Profile leads the injection block: owner Sessions, Subagents and
	// scouts all carry it, Reviewers the filtered one. A missing engine or
	// an empty Profile injects nothing and never blocks the start.
	if pb, pv := d.profileBlockFor(meta); len(pb) > 0 {
		h.inject.prepend(pb, pv)
	}
	dirs, err := d.ownerDirsFor(meta.Owner)
	if err != nil {
		return nil, &startError{code: http.StatusInternalServerError, msg: err.Error(), asJSON: true}
	}
	meta.Dirs = dirs
	// Sessions are uncapped: only concurrent Run tickets are limited, by the
	// driver. starting still counts the reservation for the idle check.
	d.mu.Lock()
	if meta.Harness != "claude" {
		d.starting++
	}
	d.mu.Unlock()
	// Allocate the harness Session before publishing: a concurrent GET
	// or prompt must never see a Session with no harness handle.
	// Claude skips the generic path here: its blocking StartSession runs
	// below after publishing, so MCP tools are connected before the
	// first prompt can go out.
	if meta.Harness != "claude" {
		d.mu.Lock()
		be := d.backendForLocked(meta.Harness)
		d.mu.Unlock()
		if be == nil {
			d.mu.Lock()
			d.starting--
			d.mu.Unlock()
			return nil, &startError{code: http.StatusInternalServerError, msg: "backend not configured", asJSON: true}
		}
		id := meta.ID
		if err := be.CreateSession(meta, func(kind, text, status string) {
			d.appendEvent(id, kind, text, status)
		}); err != nil {
			d.mu.Lock()
			d.starting--
			d.mu.Unlock()
			return nil, &startError{code: http.StatusBadRequest, msg: err.Error()}
		}
		d.mu.Lock()
		d.sessions[meta.ID] = &session{meta: meta, live: true, notify: make(chan struct{}), last: d.now(), inject: h.inject, injectOp: handleCreate}

		d.persistMeta(meta)
		d.indexRunChildSessionLocked(meta)
		d.starting--
		d.mu.Unlock()
		// Seed the context meter now that the handle exists: the first turn's
		// end may be far away (or stalled), and the meter reads empty until a
		// usage Event lands.
		d.seedUsage(meta.ID)
		return be, nil
	}
	// Claude creation blocks until session/new answers, so MCP tools
	// are connected before the first prompt can go out. The Session
	// publishes live to hold its cap slot while it starts.
	d.mu.Lock()
	d.sessions[meta.ID] = &session{meta: meta, live: true, notify: make(chan struct{}), last: d.now(), inject: h.inject, injectOp: handleCreate}

	d.persistMeta(meta)
	d.indexRunChildSessionLocked(meta)
	cb := d.claude
	d.mu.Unlock()
	if cb == nil {
		d.removeSession(meta.ID)
		return nil, &startError{code: http.StatusInternalServerError, msg: "claude backend not configured"}
	}
	id := meta.ID
	if err := cb.CreateSession(meta, func(kind, text, status string) {
		d.appendEvent(id, kind, text, status)
	}); err != nil {
		d.removeSession(meta.ID)
		return nil, &startError{code: http.StatusInternalServerError, msg: err.Error()}
	}
	// Seed the meter from the known claude window: the adapter's first
	// usage_update may never come, and the meter reads empty until one does.
	d.seedUsage(meta.ID)
	return cb, nil
}

// persistResumeToken stores the backend's live handle so a restart or a
// later prompt can resume from it.
func (d *Daemon) persistResumeToken(be Backend, id string) {
	if be == nil {
		return
	}
	tok := be.ResumeToken(id)
	if tok == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if s, ok := d.sessions[id]; ok {
		s.meta.ResumeToken = tok
		d.persistMeta(s.meta)
	}
}

// resumeBackendLocked picks the Backend a prompt resumes through. Claude
// Sessions need the wired claude Backend; without it only history reads.
// Callers hold d.mu.
func (d *Daemon) resumeBackendLocked(meta SessionMeta) (Backend, string) {
	if meta.Harness == "claude" {
		if d.claude == nil {
			return nil, "claude backend not configured; history is read-only"
		}
		return d.claude, ""
	}
	return d.backendForLocked(meta.Harness), ""
}

// resumeViewLocked reports whether a new prompt would be accepted now: live
// Sessions take one, ended Sessions never do, and released Sessions defer
// to the Backend. Callers hold d.mu.
func (d *Daemon) resumeViewLocked(s *session) (bool, string) {
	if s.meta.Status == "ended" {
		return false, "session ended; history is read-only"
	}
	if s.live {
		return true, ""
	}
	be, unwired := d.resumeBackendLocked(s.meta)
	if unwired != "" {
		return false, unwired
	}
	return be.ResumeOK(s.meta)
}

// finishTurn ends the current turn, drains the Daemon pending list in turn
// order, then releases the agent process once the Session settles at idle or
// failed with no turn left running, so settled Sessions hold history only
// and a retry resumes from the log. Send paths count each launched turn in
// active under the lock; a release that finds a turn still running leaves
// the process alone, which closes the race where a new message lands while
// the old turn's status still reads idle. An ended Session drops its pending
// list: ending is terminal too.
func (d *Daemon) finishTurn(id string) {
	for {
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			return
		}
		if s.active > 0 {
			s.active--
		}
		// The turn that set swapping has ended.
		s.swapping = false
		if s.meta.Status == "ended" {
			s.pending = nil
			s.busy = false
			d.mu.Unlock()
			return
		}
		// A blocked Session holds its queue and its live handle until Retry or
		// Dismiss: nothing drains and nothing releases.
		if s.meta.CompressBlock != "" {
			s.busy = false
			d.mu.Unlock()
			return
		}
		if len(s.pending) > 0 && d.stopRestart[id] && s.meta.RestartStopped {
			// A restart stopped this turn: nothing drains into a new turn.
			// The queue rides in the Session record to the next Daemon.
			for _, m := range s.pending {
				s.meta.Carry += "- " + m.text + "\n"
			}
			s.pending = nil
			d.persistMeta(s.meta)
		}
		if len(s.pending) > 0 {
			// A Steer still held when the turn ends had no tool call to
			// follow: it arrives as a Follow-up.
			for i := range s.pending {
				if s.pending[i].held {
					s.pending[i].held = false
					s.pending[i].mode = "followup"
					s.pending[i].degraded = true
				}
			}
			n := leadRun(s.pending)
			next := mergePending(s.pending[:n])
			s.pending = s.pending[n:]
			s.active++
			be := d.backendForLocked(s.meta.Harness)
			d.mu.Unlock()
			d.appendPromptEvent(id, next.text, modeLabel(next.mode), len(next.images), next.imageIDs, next.from)
			if be == nil {
				continue
			}
			btext, bimages := d.backendMessage(id, next.text, next.images, next.imageIDs)
			promptBackend(be, id, btext, bimages, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			})
			// Drained turns run no gate and no turn-end auto check, so a
			// Session fed by a constant queue could cook its context
			// unchecked: arm the automatic wait here and let
			// runDeferredCompress run it once the queue drains.
			d.armAutoCompress(id)
			continue
		}
		s.busy = false
		if !s.live || s.active > 0 || (s.meta.Status != "idle" && s.meta.Status != "failed") {
			d.mu.Unlock()
			return
		}
		// Flip live off and mark the release under one lock hold: a send
		// in the release window answers 409 instead of resuming a process
		// the stale ReleaseSession would then kill by id lookup.
		s.live = false
		s.releasing = true
		be := d.backendForLocked(s.meta.Harness)
		if s.meta.Harness == "claude" && d.claude != nil {
			be = d.claude
		}
		d.mu.Unlock()
		if be == nil {
			d.mu.Lock()
			if rs, ok := d.sessions[id]; ok {
				rs.releasing = false
			}
			d.mu.Unlock()
			return
		}
		tok := be.ReleaseSession(id)
		d.mu.Lock()
		s, ok = d.sessions[id]
		if !ok {
			d.mu.Unlock()
			return
		}
		s.releasing = false
		// A message that resumed meanwhile already flipped live back on; its
		// ResumeSession owns the token now, so only store when still released.
		if !s.live && tok != "" {
			s.meta.ResumeToken = tok
			d.persistMeta(s.meta)
		}
		d.mu.Unlock()
		return
	}
}

func (d *Daemon) appendEvent(id, kind, text, status string) Event {
	e := d.appendModeEvent(id, kind, text, status, "")
	// Stop-after-submit reacts to the stored Event without the lock: every
	// Backend turn, tool call and submit route funnels through here.
	d.maybeStopAfterSubmit(id, e)
	d.maybeStopForRestart(id, e)
	return e
}

func (d *Daemon) appendModeEvent(id, kind, text, status, mode string) Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendModeEventLocked(id, kind, text, status, mode)
}

// appendPromptEvent stores one prompt Event typed by from. images counts the
// inline attachments and imageIDs names the Session-stored ones (the bytes go
// to the Backend, never into the log).
func (d *Daemon) appendPromptEvent(id, text, mode string, images int, imageIDs []string, from promptSender) Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.appendPromptEventLocked(id, text, mode, images, imageIDs, from)
}

// appendPromptEventLocked is appendPromptEvent; callers hold d.mu.
func (d *Daemon) appendPromptEventLocked(id, text, mode string, images int, imageIDs []string, from promptSender) Event {
	return d.appendEventExtraLocked(id, "prompt", text, "", mode, images, imageIDs, from.stamp)
}

// appendModeEventLocked stores one Event; callers hold d.mu.
func (d *Daemon) appendModeEventLocked(id, kind, text, status, mode string) Event {
	return d.appendEventLocked(id, kind, text, status, mode, 0, nil)
}

func (d *Daemon) appendEventLocked(id, kind, text, status, mode string, images int, imageIDs []string) Event {
	return d.appendEventExtraLocked(id, kind, text, status, mode, images, imageIDs, nil)
}

// appendEventExtraLocked stores one Event; extra sets optional fields (Handoff,
// Sender) before the Event is logged. Callers hold d.mu.
func (d *Daemon) appendEventExtraLocked(id, kind, text, status, mode string, images int, imageIDs []string, extra func(*Event)) Event {
	s, ok := d.sessions[id]
	if !ok {
		return Event{}
	}
	// Ended is terminal: a late turn edge (failed after a DELETE closed the
	// SDK session) must never reopen the Session.
	if kind == "status" && s.meta.Status == "ended" {
		status = "ended"
	}
	// A turn that errors because Shutdown closed its process did not fail:
	// the next Daemon continues it (runs_restart_resume.go).
	if kind == "status" && status == "failed" && d.shuttingDown {
		status, text = "idle", restartInterruptedText
	}
	e := Event{
		Version:   1,
		Seq:       int64(len(s.events) + 1),
		SessionID: id,
		Kind:      kind,
		Text:      text,
		Status:    status,
		Mode:      mode,
		Images:    images,
		ImageIDs:  imageIDs,
		CreatedAt: time.Now().UTC(),
	}
	if extra != nil {
		extra(&e)
	}
	// Every prompt Event carries a sender; an untyped one is the system's.
	if kind == "prompt" && e.Sender == "" {
		systemSender.stamp(&e)
	}
	s.events = append(s.events, e)
	s.last = d.now()
	trackSessionToolEvent(s, kind, text, s.last)
	if kind == "status" && status != "" {
		prev := s.meta.Status
		s.meta.Status = status
		s.meta.Capped = status == "failed" && sessionLimitFailureInTurn(s.events)
		if prev == "working" && (status == "idle" || status == "failed") {
			d.pushTurnDone(s.meta, status, e.Seq)
		}
	}
	notify := s.notify
	s.notify = make(chan struct{})
	meta := s.meta
	// Persist under the lock so concurrent appends land in the log in
	// sequence order and the metadata never goes stale.
	d.persistEvent(meta, e)
	d.persistMeta(meta)
	close(notify)
	if kind == "tool_call_updated" && meta.Harness == "claude" && toolCallDone(text) {
		var held []queuedMsg
		keep := s.pending[:0:0]
		for _, m := range s.pending {
			if m.held {
				held = append(held, m)
			} else {
				keep = append(keep, m)
			}
		}
		if len(held) > 0 {
			s.pending = keep
			go d.injectHeldSteers(id, held)
		}
	}
	return e
}

// toolCallDone reports whether a ToolCall Event text carries a finished tool.
func toolCallDone(text string) bool {
	var tc struct {
		Status string `json:"status"`
	}
	if json.Unmarshal([]byte(text), &tc) != nil {
		return false
	}
	return tc.Status == "completed" || tc.Status == "failed"
}

// injectHeldSteers sends Steers held for a tool call into the running claude
// turn. A Steer the adapter cannot take goes back as a Follow-up.
func (d *Daemon) injectHeldSteers(id string, held []queuedMsg) {
	d.mu.Lock()
	be := d.claude
	d.mu.Unlock()
	for _, m := range held {
		btext, bimages := d.backendMessage(id, m.text, m.images, m.imageIDs)
		if be != nil && be.SteerLive(id, btext, bimages) {
			d.appendPromptEvent(id, m.text, "steer", 0, nil, m.from)
			continue
		}
		d.routeLiveOpt(id, "followup", m.text, m.images, m.imageIDs, false, m.from)
	}
}

// sendResult is the outcome of routing one message.
type sendResult struct {
	seq      int64
	queued   bool
	position int
}

// resumeReleased resumes a released Session's agent process before a send.
// It returns 0 when the Session is live and ready; otherwise the HTTP code
// and error text the send route answers with. The flow mirrors the create
// cap: a resume over the cap is 429, a harness that cannot resume is 409
// with the why, and history stays readable either way.
func (d *Daemon) resumeReleased(id string) (int, string) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, "not found"
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return http.StatusGone, "session ended"
	}
	if s.live {
		d.mu.Unlock()
		return 0, ""
	}
	// Single-flight: a second send on a released Session answers 409
	// instead of double-resuming past the cap or stacking loads. A send
	// in a release window answers 409 too, so the stale release never
	// kills the process the retry resumes.
	if s.resuming || s.releasing {
		d.mu.Unlock()
		return http.StatusConflict, "session is resuming; send the message again"
	}
	s.resuming = true
	meta := s.meta
	be, unwired := d.resumeBackendLocked(meta)
	d.mu.Unlock()
	// The flag clears on every exit below; the Session pointer stays
	// valid even if the entry is deleted meanwhile.
	clear := func() {
		d.mu.Lock()
		s.resuming = false
		d.mu.Unlock()
	}
	// A released Session resumes from the log before the message runs.
	// Where the harness cannot resume, history stays readable and the
	// message is refused with the why.
	if unwired != "" {
		clear()
		return http.StatusConflict, unwired
	}
	if be == nil {
		clear()
		return http.StatusInternalServerError, "backend not configured"
	}
	if okResume, reason := be.ResumeOK(meta); !okResume {
		clear()
		return http.StatusConflict, reason
	}
	d.mu.Lock()
	cur, ok := d.sessions[id]
	resume := ok && !cur.live && cur.meta.Status != "ended"
	if resume {
		d.starting++
		meta = cur.meta
	}
	d.mu.Unlock()
	if resume {
		dirs, err := d.ownerDirsFor(meta.Owner)
		if err == nil {
			meta.Dirs = dirs
			err = be.ResumeSession(meta, func(kind, text, status string) {
				d.appendEvent(id, kind, text, status)
			})
		}
		if err != nil {
			d.mu.Lock()
			d.starting--
			d.mu.Unlock()
			clear()
			return http.StatusInternalServerError, err.Error()
		}
		d.mu.Lock()
		d.starting--
		if cur, ok := d.sessions[id]; ok && !cur.live {
			cur.live = true
			// Resume injects the Profile only (recall is create-time).
			rh := startHandle(cur.meta, handleResume)
			if pb, pv := d.profileBlockFor(cur.meta); len(pb) > 0 {
				rh.inject.prepend(pb, pv)
			}
			cur.inject = rh.inject
			cur.injectOp = handleResume
			if tok := be.ResumeToken(id); tok != "" {
				cur.meta.ResumeToken = tok
			}
			d.persistMeta(cur.meta)
		}
		d.mu.Unlock()
	}
	clear()
	// Final gate: a concurrent end must not send on a dead process.
	d.mu.Lock()
	s, ok = d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return http.StatusNotFound, "not found"
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return http.StatusGone, "session ended"
	}
	if !s.live {
		d.mu.Unlock()
		return http.StatusConflict, "session is resuming; send the message again"
	}
	d.mu.Unlock()
	return 0, ""
}

// sendMessage resumes a released Session if needed, then routes one prompt,
// steer or follow-up. It writes the HTTP response itself. Refusals carry
// the why as {"error"} so the UI can show it.
func (d *Daemon) sendMessage(w http.ResponseWriter, r *http.Request, id, mode, text string, images []Image, imageIDs []string) {
	// Scout Subagents are read-only: their spawn prompt is their only turn.
	d.mu.Lock()
	if s, ok := d.sessions[id]; ok && s.meta.Scout {
		d.mu.Unlock()
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "scout sessions are read-only"})
		return
	}
	d.mu.Unlock()
	if code, msg := d.resumeReleased(id); code != 0 {
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		if code == http.StatusGone {
			http.Error(w, msg, code)
			return
		}
		writeJSON(w, code, map[string]any{"error": msg})
		return
	}
	// Inline attachments are stored under the Session too, so the transcript
	// and its gallery keep them and the agent gets their file paths; from here
	// they ride as ids like any other stored image.
	if len(images) > 0 {
		imageIDs = append(imageIDs, d.storeInlineImages(id, images)...)
		images = nil
	}
	from := d.senderOf(r)
	// A structural /remember action on the first line of a typed sender=user
	// message creates a Rule directly after grounding. The slash line names the
	// Rule; leftover lines are still the user's task, so they route on as the
	// turn without the slash line. Pasted blocks (the command on a later line),
	// colon forms and system messages route as normal prompts and create
	// nothing. A routed remainder that fails to start still reports its ruleId,
	// so a resend knows the Rule already exists (each send is a new grounding
	// Event, so resending the same text records a second Rule).
	if from.kind == senderUser {
		if _, _, isCommand := parseRememberCommand(text); isCommand {
			res, code, msg := d.handleRememberCommand(id, mode, text, images, imageIDs, from)
			if code != 0 {
				writeJSON(w, code, map[string]any{"error": msg})
				return
			}
			if res.rest == "" && len(images) == 0 && len(imageIDs) == 0 {
				writeJSON(w, http.StatusCreated, map[string]any{"seq": res.event.Seq, "ruleId": res.ruleID})
				return
			}
			routed, rcode := d.routeLiveImages(id, mode, res.rest, images, imageIDs, from)
			if rcode != http.StatusAccepted {
				writeJSON(w, rcode, map[string]any{"error": "session is resuming; send the message again", "ruleId": res.ruleID})
				return
			}
			out := writeSendResult(routed)
			out["ruleId"] = res.ruleID
			writeJSON(w, rcode, out)
			return
		}
	}
	res, code := d.routeLiveImages(id, mode, text, images, imageIDs, from)
	if code != http.StatusAccepted {
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		if code == http.StatusGone {
			http.Error(w, "session ended", code)
			return
		}
		writeJSON(w, code, map[string]any{"error": "session is resuming; send the message again"})
		return
	}
	writeJSON(w, code, writeSendResult(res))
}

// routeLive stores or queues one prompt, steer or follow-up on a live
// Session. Idle Sessions start a turn at once; messages arriving mid-turn
// queue: in the pig native queue for pig steers and follow-ups, in the
// Daemon pending list otherwise. A claude Steer arriving mid-turn degrades
// to a Follow-up: the adapter queues mid-turn prompts until the turn ends,
// so injection at a tool boundary is impossible. The fake Backend keeps both
// mode labels so the seam tests cover them, though it delivers both after
// the turn. Launched turns count in active so finishTurn never releases
// their process early.
func (d *Daemon) routeLive(id, mode, text string) (sendResult, int) {
	return d.routeLiveOpt(id, mode, text, nil, nil, true, systemSender)
}

// routeLiveImages is routeLive with image attachments and the sender of the
// message: inline bytes plus the Session-stored ids resolved to bytes and file
// paths for the Backend.
func (d *Daemon) routeLiveImages(id, mode, text string, images []Image, imageIDs []string, from promptSender) (sendResult, int) {
	return d.routeLiveOpt(id, mode, text, images, imageIDs, true, from)
}

// routeLiveOpt is routeLive; inject lets a mid-turn claude Steer try the
// adapter's _session/steering before it degrades to a Follow-up. from types
// the prompt Event.
func (d *Daemon) routeLiveOpt(id, mode, text string, images []Image, imageIDs []string, inject bool, from promptSender) (sendResult, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return sendResult{}, http.StatusNotFound
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return sendResult{}, http.StatusGone
	}
	if !s.live {
		d.mu.Unlock()
		return sendResult{}, http.StatusConflict
	}
	harness := s.meta.Harness
	be := d.backendForLocked(harness)
	degraded := false
	hasImages := len(images) > 0 || len(imageIDs) > 0
	btext, bimages := d.backendMessage(id, text, images, imageIDs)
	_, imgSteer := be.(imageSteerBackend)
	if hasImages && s.busy && mode == "steer" && harness != "claude" && !(harness == "pig" && imgSteer) {
		mode = "followup"
		degraded = true
	}
	if harness == "grok" && mode == "steer" && s.busy {
		// Grok has no mid-turn injection on the primary Session: a Steer
		// waits as a Follow-up.
		mode = "followup"
		degraded = true
	}
	if harness == "claude" && mode == "steer" && s.busy {
		// A Steer always lands after the next tool call: the adapter injects
		// at priority now, which pre-empts a running tool, so the Daemon holds
		// the message in pending and injects it when a tool call completes.
		// If the turn ends first, finishTurn delivers it as a Follow-up.
		// During a Handoff a claude Steer degrades too: the Handoff turn must lead.
		if _, ok := be.(*ClaudeBackend); ok && inject && !s.swapping {
			s.pending = append(s.pending, queuedMsg{mode: "steer", text: text, images: images, imageIDs: imageIDs, held: true, from: from})
			pos := len(s.pending)
			d.mu.Unlock()
			return sendResult{queued: true, position: pos}, http.StatusAccepted
		}
		mode = "followup"
		degraded = true
	}
	if harness == "pig" && s.swapping && (mode == "steer" || mode == "followup") {
		// During a Handoff swap the old handle is dying: a native queue would
		// land on the handle release closes. Hold in the Daemon pending list
		// instead, like claude, so the message survives to the new handle.
		// A held steer degrades: the Handoff turn must lead.
		if mode == "steer" {
			mode = "followup"
			degraded = true
		}
		s.pending = append(s.pending, queuedMsg{mode: mode, text: text, images: images, imageIDs: imageIDs, degraded: degraded, from: from})
		pos := len(s.pending)
		d.mu.Unlock()
		return sendResult{queued: true, position: pos}, http.StatusAccepted
	}
	// A working pig Session steers text-only messages through the native
	// queue, which delivers at the next tool boundary. Image messages stay in
	// the Daemon pending list so a Handoff swap never drops their attachments
	// (the SDK and shim queues carry text only). The native call and the prompt
	// Event land under d.mu together, so the turn cannot end between
	// them and strand the message until the turn after next. Lock order
	// is d.mu before the backend lock; no Backend path takes them in
	// the reverse order. The Event lands at once so the transcript shows
	// what was queued, even if it is cleared before delivery.
	if harness == "pig" && s.busy && !hasImages && (mode == "steer" || mode == "followup") {
		if qb, ok := be.(steerBackend); ok {
			queued := false
			if mode == "steer" {
				queued = qb.Steer(id, text)
			} else {
				queued = qb.FollowUp(id, text)
			}
			if queued {
				e := d.appendPromptEventLocked(id, text, mode, len(images), imageIDs, from)
				d.mu.Unlock()
				return sendResult{seq: e.Seq, queued: true}, http.StatusAccepted
			}
			// The native queue missed (shim unreachable): fall through to the
			// Daemon pending list so the message waits instead of dropping.
		}
	}
	// A blocked Session queues every message until Retry or Dismiss.
	if s.busy || s.meta.CompressBlock != "" {
		s.pending = append(s.pending, queuedMsg{mode: mode, text: text, images: images, imageIDs: imageIDs, degraded: degraded, from: from})
		pos := len(s.pending)
		d.mu.Unlock()
		return sendResult{queued: true, position: pos}, http.StatusAccepted
	}
	// Immediate first turns (create) build recall synchronously from the task
	// prompt before the turn launches: the request returns after recall, so a
	// status poller never sees idle during recall. Resume turns carry the
	// Profile only; compress turns already carry recall from the Handoff.
	// Queued messages (above) never trigger recall.
	needRecall := s.injectOp == handleCreate
	recallMeta := s.meta
	s.busy = true
	s.active++
	if needRecall {
		// Mark the op consumed so the turn skips its fallback recall; the
		// block is merged below after the bank work.
		s.injectOp = handleResume
	}
	d.mu.Unlock()
	if needRecall {
		rb, refs := d.recallOrNotice(context.Background(), id, recallMeta, text)
		d.mu.Lock()
		cur, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			return sendResult{}, http.StatusNotFound
		}
		if cur.meta.Status == "ended" {
			d.mu.Unlock()
			return sendResult{}, http.StatusGone
		}
		if len(rb) > 0 {
			cur.inject.appendRecall(rb, refs)
		}
		d.mu.Unlock()
	}
	e := d.appendPromptEvent(id, text, modeLabel(mode), len(images), imageIDs, from)
	// Seed the meter at dispatch: a resumed or older Session may hold no
	// usage Event yet, and the estimate now includes this prompt. A Session
	// with a reading keeps it; the seed never overwrites.
	d.seedUsage(id)
	d.maybeTitle(id, text)
	if be != nil {
		go d.runTurn(id, be, btext, bimages)
	} else {
		d.finishTurn(id)
	}
	return sendResult{seq: e.Seq}, http.StatusAccepted
}

// backendMessage resolves a message for the Backend turn: Session-stored
// image ids become inline bytes and their file paths join the text, so the
// agent sees the pictures and can read the files; the transcript keeps the
// clean text and the ids. No ids leaves the message untouched.
func (d *Daemon) backendMessage(id, text string, images []Image, imageIDs []string) (string, []Image) {
	if len(imageIDs) == 0 {
		return text, images
	}
	return d.imageRefsText(id, text, imageIDs), d.withStoredImages(id, images, imageIDs)
}

// mergePending folds queued messages into one, in order, joined by a blank
// line. One message keeps its mode; mixed modes merge as a follow-up.
func mergePending(q []queuedMsg) queuedMsg {
	if len(q) == 1 {
		return q[0]
	}
	out := queuedMsg{mode: q[0].mode, from: q[0].from}
	parts := make([]string, 0, len(q))
	for _, m := range q {
		if m.text != "" {
			parts = append(parts, m.text)
		}
		out.images = append(out.images, m.images...)
		out.imageIDs = append(out.imageIDs, m.imageIDs...)
		if m.mode != out.mode {
			out.mode = "followup"
		}
	}
	out.text = strings.Join(parts, "\n\n")
	return out
}

// leadRun counts the queued messages at the head of q that share one sender.
// A delivered prompt Event never mixes Users, nor a User with the system.
func leadRun(q []queuedMsg) int {
	n := 0
	for n < len(q) && q[n].from == q[0].from {
		n++
	}
	return n
}

// modeLabel maps a route mode to the prompt Event mode. Plain prompts keep
// the field empty for log back-compatibility.
func modeLabel(mode string) string {
	if mode == "steer" || mode == "followup" {
		return mode
	}
	return ""
}

// runTurn drives one backend turn, then drains the Daemon pending list in
// turn order. Pig native queues drain inside the SDK Send, never here.
func (d *Daemon) runTurn(id string, be Backend, text string, images []Image) {
	d.runTurnWith(id, be, text, images, false, nil)
}

// runTurnWith is runTurn with an optional tap that sees every Backend event
// before it is appended. A handoff turn is Daemon-made: it skips the submit
// gate, auto-compress and the Reviewer verdict hook, since its reply is no
// submit and no verdict.
func (d *Daemon) runTurnWith(id string, be Backend, text string, images []Image, handoff bool, tap func(kind, text, status string)) {
	mark, harness, gated := d.gateBaseline(id)
	gated = gated && !handoff
	in, op := d.takeInjection(id)
	// Create turns build fresh recall from the task prompt: the Profile came
	// with the handle, the Rules come now that the prompt is known. An empty
	// prompt injects the Profile only. Resume turns carry the Profile only;
	// compress turns already carry fresh recall from the Handoff. This is the
	// fallback owner: routeLiveOpt and spawnFrom build synchronously, so only
	// direct runTurn callers (Run Workers) reach it.
	if !handoff && op == handleCreate {
		d.mu.Lock()
		meta := SessionMeta{}
		if s, ok := d.sessions[id]; ok {
			meta = s.meta
		}
		d.mu.Unlock()
		if rb, refs := d.recallOrNotice(context.Background(), id, meta, text); len(rb) > 0 {
			in.appendRecall(rb, refs)
		}
	}
	if !in.empty() {
		d.appendInjectionEvent(id, in.profile, in.refs)
		if block := in.block(); block != "" {
			text = block + "\n\n" + text
		}
	}
	promptBackend(be, id, text, images, func(kind, text, status string) {
		if tap != nil {
			tap(kind, text, status)
		}
		d.appendEvent(id, kind, text, status)
	})
	if gated {
		// A turn that settled with a compress due compresses before the
		// gate reminds: reminders into a nearly full context burn turns
		// the Handoff would have saved, so the gate stands down and the
		// deferred compress runs once the turn settles below. A user stop
		// is deliberate: it takes no reminder and records no failure. A
		// dead turn (fast, no assistant text, tool call or submit) gets no
		// reminder either: it records a turn failure for the Run Chain
		// classifier instead. Anything else reminds as today.
		switch {
		case d.compressBeforeGate(id):
		case d.takeGateSkip(id):
		case d.failDeadTurn(id, mark):
		default:
			d.enforceSubmitGate(id, be, mark, harness)
		}
	}
	d.finishTurn(id)
	chainHandled := d.handleRunChainFailure(id)
	// A Run Reviewer's settled turn carries its verdict unless the Chain
	// already replaced a failed Session with its next entry.
	if !handoff && !chainHandled {
		d.handleReviewerTurnEnd(id)
	}
	// A settled Subagent may complete its Group: the last member to finish
	// delivers one grouped result to the parent as a Steer.
	d.maybeDeliverGroup(id)
	// A compress held by a Group of this Session may run now. An explicit
	// wait wins over an automatic one: the auto check runs last, so it sees
	// the busy claim a deferred compress just took and skips to the next
	// turn end.
	d.runDeferredCompress(id)
	// Auto-compress runs after the Session settles, so the usage Event of
	// this turn is in the log and requestCompress sees an idle Session. A
	// handoff turn never auto-compresses: its reply would loop on the same
	// threshold, and the cooldown already guards the next one.
	if !handoff {
		d.maybeAutoCompress(id)
		d.maybeMineLive(id)
	}
}

// listQueue reports not-yet-delivered messages.
// It merges the Daemon pending list with pig native queues.
func (d *Daemon) listQueue(id string) ([]QueuedMessage, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return nil, http.StatusNotFound
	}
	out := make([]QueuedMessage, 0, len(s.pending))
	for _, p := range s.pending {
		out = append(out, QueuedMessage{Mode: p.mode, Text: p.text, Degraded: p.degraded})
	}
	var qb steerBackend
	if s.meta.Harness == "pig" {
		qb, _ = d.backendForLocked(s.meta.Harness).(steerBackend)
	}
	d.mu.Unlock()
	if qb != nil {
		out = append(qb.QueuedMessages(id), out...)
	}
	return out, http.StatusOK
}

// dropQueue clears not-yet-delivered messages before delivery.
// It clears the Daemon pending list and pig native queues.
func (d *Daemon) dropQueue(id string) (int, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return 0, http.StatusNotFound
	}
	n := len(s.pending)
	s.pending = nil
	var qb steerBackend
	if s.meta.Harness == "pig" {
		qb, _ = d.backendForLocked(s.meta.Harness).(steerBackend)
	}
	d.mu.Unlock()
	if qb != nil {
		n += qb.ClearQueued(id)
	}
	return n, http.StatusOK
}

// interrupt stops the running turn of a Session and drops every message
// queued behind it, so the stopped agent does not pick up where it left off.
// A Session with no running turn reports interrupted false.
func (d *Daemon) interrupt(id string) (bool, int) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if !ok {
		d.mu.Unlock()
		return false, http.StatusNotFound
	}
	if s.meta.Status == "ended" {
		d.mu.Unlock()
		return false, http.StatusGone
	}
	busy := s.busy
	ib, _ := d.backendForLocked(s.meta.Harness).(interruptBackend)
	d.mu.Unlock()
	if !busy || ib == nil {
		return false, http.StatusOK
	}
	d.dropQueue(id)
	stopped := ib.Interrupt(id)
	if stopped {
		// A user stop is deliberate: the submit gate must not remind a turn
		// the user cancelled.
		d.mu.Lock()
		if s, ok := d.sessions[id]; ok {
			s.gateSkip = true
		}
		d.mu.Unlock()
	}
	return stopped, http.StatusOK
}

func (d *Daemon) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		d.mu.Lock()
		metas := make([]SessionMeta, 0, len(d.sessions))
		lasts := map[string]time.Time{}
		for _, s := range d.sessions {
			if s.meta.Owner == "" {
				continue
			}
			metas = append(metas, s.meta)
			lasts[s.meta.ID] = s.last
		}
		d.mu.Unlock()
		sort.Slice(metas, func(i, j int) bool {
			if lasts[metas[i].ID].Equal(lasts[metas[j].ID]) {
				return metas[i].ID > metas[j].ID
			}
			return lasts[metas[i].ID].After(lasts[metas[j].ID])
		})
		writeJSON(w, http.StatusOK, metas)
	case "POST":
		creator, ok := d.creatorOf(w, r)
		if !ok {
			return
		}
		var req struct {
			Harness   string `json:"harness"`
			Model     string `json:"model"`
			Effort    string `json:"effort"`
			WorkDir   string `json:"workDir"`
			ProjectID string `json:"projectId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.ProjectID != "" {
			p, ok := d.projectByID(req.ProjectID)
			if !ok {
				http.Error(w, "unknown project", http.StatusBadRequest)
				return
			}
			req.WorkDir = p.Path
		}
		if req.Harness == "" || req.Model == "" {
			http.Error(w, "harness and model are required", http.StatusBadRequest)
			return
		}
		if req.Harness == "grok" && !d.hasBackend("grok") {
			http.Error(w, "grok harness not available (grok or el-mcp missing)", http.StatusServiceUnavailable)
			return
		}
		if req.Harness == "claude" && !validClaudeModel(req.Model) {
			http.Error(w, "unknown claude model (want opus, sonnet or haiku)", http.StatusBadRequest)
			return
		}
		if req.WorkDir == "" && req.ProjectID == "" {
			// No Project and no folder: the Session opens in the User's home.
			req.WorkDir = d.users.homeDir(creator.Username)
			if err := os.MkdirAll(req.WorkDir, 0o755); err != nil {
				http.Error(w, "could not create the home directory: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if req.WorkDir == "" {
			http.Error(w, "workDir or projectId with a path is required", http.StatusBadRequest)
			return
		}
		req.WorkDir = absWorkDir(req.WorkDir)
		meta := SessionMeta{
			ID:        newID(),
			Harness:   req.Harness,
			Model:     req.Model,
			Effort:    req.Effort,
			WorkDir:   req.WorkDir,
			ProjectID: req.ProjectID,
			Status:    "idle",
			CreatedAt: time.Now().UTC(),
			Owner:     creator.ID,
		}
		be, serr := d.startSession(meta)
		if serr != nil {
			if serr.asJSON {
				writeJSON(w, serr.code, map[string]any{"error": serr.msg})
			} else {
				http.Error(w, serr.msg, serr.code)
			}
			return
		}
		d.persistResumeToken(be, meta.ID)
		d.appendEvent(meta.ID, "status", "", "idle")
		d.mu.Lock()
		m := d.sessions[meta.ID].meta
		d.mu.Unlock()
		writeJSON(w, http.StatusCreated, m)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// writeSendResult renders one routing outcome. Immediate turns carry the
// prompt Event seq; Daemon-queued messages carry their queue position.
func writeSendResult(res sendResult) map[string]any {
	out := map[string]any{"seq": res.seq, "queued": res.queued}
	if res.queued && res.seq == 0 {
		out["position"] = res.position
	}
	return out
}
func (d *Daemon) handleSessionSub(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	parts := strings.SplitN(rest, "/", 2)
	id := parts[0]
	sub := ""
	if len(parts) == 2 {
		sub = parts[1]
	}
	// A Session with no Owner is hidden: every route answers 404 for it.
	if d.hiddenSession(id) {
		http.NotFound(w, r)
		return
	}
	// A Run Reviewer answers only the Daemon's gate prompts: user messages
	// would corrupt the verdict reply it records.
	if r.Method == "POST" && (sub == "prompt" || sub == "steer" || sub == "followup") {
		if _, _, _, isReviewer := d.reviewerOf(id); isReviewer {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "reviewer sessions take no messages; steer the worker instead"})
			return
		}
	}
	switch {
	case sub == "" && r.Method == "GET":
		d.mu.Lock()
		s, ok := d.sessions[id]
		if !ok {
			d.mu.Unlock()
			http.NotFound(w, r)
			return
		}
		meta := s.meta
		events := append([]Event(nil), s.events...)
		live := s.live
		resumable, note := d.resumeViewLocked(s)
		be := d.backendForLocked(meta.Harness)
		_, isHandoff := be.(handoffBackend)
		compressible := isHandoff
		d.mu.Unlock()
		var compressBlocked string
		if isHandoff {
			compressBlocked = compressNote(d.compressSettingsOf().Model)
		}
		// Full log for old clients; ?tail= and ?before= page by turn.
		tail, hasBefore, before, turns, at := parseSnapshotQuery(r.URL.Query())
		start, end := 0, len(events)
		if hasBefore {
			start, end = beforeWindow(events, before, turns)
		} else if tail >= 0 {
			start = atStart(events, tailStart(events, tail), at)
		}
		window, firstSeq, hasMore := snapshotWindow(events, start, end)
		writeJSON(w, http.StatusOK, map[string]any{
			"session": meta, "events": window,
			"hasMore": hasMore, "firstSeq": firstSeq,
			"live": live, "resumable": resumable, "resumeNote": note,
			"compressible": compressible,
			"compressNote": compressBlocked,
		})
	case sub == "prompt" && r.Method == "POST":
		var req struct {
			Text     string   `json:"text"`
			Mode     string   `json:"mode"`
			Images   []Image  `json:"images"`
			ImageIDs []string `json:"imageIds"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxPromptBytes)
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.Text == "" && len(req.Images) == 0 && len(req.ImageIDs) == 0) {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		if msg := validateImages(req.Images); msg != "" {
			http.Error(w, msg, http.StatusBadRequest)
			return
		}
		if bad := d.imageIDsValid(id, req.ImageIDs); bad != "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown image: " + bad})
			return
		}
		mode := req.Mode
		if mode == "" {
			mode = "prompt"
		}
		if mode != "prompt" && mode != "steer" && mode != "followup" {
			http.Error(w, "unknown mode (want prompt, steer or followup)", http.StatusBadRequest)
			return
		}
		d.sendMessage(w, r, id, mode, req.Text, req.Images, req.ImageIDs)
	case (sub == "steer" || sub == "followup") && r.Method == "POST":
		var req struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
			http.Error(w, "text is required", http.StatusBadRequest)
			return
		}
		d.sendMessage(w, r, id, sub, req.Text, nil, nil)
	case sub == "interrupt" && r.Method == "POST":
		stopped, code := d.interrupt(id)
		if code != http.StatusOK {
			http.Error(w, http.StatusText(code), code)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"interrupted": stopped})
	case sub == "compress" && r.Method == "POST":
		d.handleCompress(w, r, id)
	case sub == "compress/dismiss" && r.Method == "POST":
		d.handleCompressDismiss(w, r, id)
	case sub == "queue" && r.Method == "GET":
		q, code := d.listQueue(id)
		if code != http.StatusOK {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"queue": q})
	case sub == "queue" && r.Method == "DELETE":
		n, code := d.dropQueue(id)
		if code != http.StatusOK {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cleared": n})
	case sub == "submit" && r.Method == "POST":
		d.handleSessionSubmit(w, r, id)
	case sub == "spawn" && r.Method == "POST":
		d.handleSpawn(w, r, id)
	case sub == "sudo" && r.Method == "POST":
		d.handleSudoCreate(w, r, id)
	case strings.HasPrefix(sub, "sudo/") && r.Method == "POST":
		d.handleSudoDecide(w, r, id, strings.TrimPrefix(sub, "sudo/"))
	case sub == "" && r.Method == "DELETE":
		m, code := d.endSession(id)
		if code == http.StatusNotFound {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, m)
	case sub == "images":
		d.handleSessionImages(w, r, id)
	case strings.HasPrefix(sub, "images/"):
		d.handleSessionImageFile(w, r, id, strings.TrimPrefix(sub, "images/"))
	case sub == "events" && r.Method == "GET":
		d.serveEvents(w, r, id)
	case sub == "profile" && r.Method == "GET":
		d.handleSessionProfile(w, r, id)
	case sub == "profile/propose" && r.Method == "POST":
		d.handleSessionProfilePropose(w, r, id)
	case sub == "remember" && r.Method == "POST":
		d.handleSessionRemember(w, r, id)
	case sub == "recall" && r.Method == "GET":
		d.handleSessionRecall(w, r, id)
	case sub == "promote" && r.Method == "POST":
		d.handleSessionPromote(w, r, id)
	default:
		http.NotFound(w, r)
	}
}

func (d *Daemon) serveEvents(w http.ResponseWriter, r *http.Request, id string) {
	since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			since = n
		}
	}
	d.mu.Lock()
	s, ok := d.sessions[id]
	d.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, _ := w.(http.Flusher)

	pos := since
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		d.mu.Lock()
		s, ok = d.sessions[id]
		var pending []Event
		var ch chan struct{}
		if ok {
			for _, e := range s.events {
				if e.Seq > pos {
					pending = append(pending, e)
				}
			}
			ch = s.notify
		}
		d.mu.Unlock()
		if !ok {
			return
		}
		for _, e := range pending {
			pos = e.Seq
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.Seq, mustJSON(e))
			if flusher != nil {
				flusher.Flush()
			}
		}
		// If the client asked for a finite replay via ?wait=0, stop here.
		if r.URL.Query().Get("wait") == "0" {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			if flusher != nil {
				flusher.Flush()
			}
		case <-ch:
		}
	}
}

// guard rejects requests whose Host or Origin is not loopback.
func guard(next http.Handler) http.Handler { return guardHosts(next, nil, false) }

// AllowTailnet accepts requests addressed to a Tailscale IP (100.64.0.0/10).
func (d *Daemon) AllowTailnet(on bool) { d.allowTailnet = on }

// IsTailnetIP reports an address in the Tailscale CGNAT range 100.64.0.0/10.
func IsTailnetIP(ip net.IP) bool {
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return ip != nil && ip.To4() != nil && cgnat.Contains(ip)
}

// SetAllowedHosts lets requests through whose Host (and Origin) name one of
// these hostnames, besides loopback. A reverse proxy on the same machine,
// such as `tailscale serve`, forwards the public name in Host. The Daemon
// still binds loopback only.
func (d *Daemon) SetAllowedHosts(hosts []string) {
	d.allowHosts = nil
	for _, h := range hosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			d.allowHosts = append(d.allowHosts, strings.TrimSuffix(h, "."))
		}
	}
}

// hostAllowed reports a loopback host or one on the allow list. A port on
// the host is ignored.
func hostAllowed(hostport string, allow []string, tailnet bool) bool {
	if isLoopbackHost(hostport) {
		return true
	}
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(strings.Trim(host, "[]")), ".")
	if tailnet && IsTailnetIP(net.ParseIP(host)) {
		return true
	}
	for _, a := range allow {
		if host == a {
			return true
		}
	}
	return false
}

func guardHosts(next http.Handler, allow []string, tailnet bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostAllowed(r.Host, allow, tailnet) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !hostAllowed(u.Host, allow, tailnet) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// redactServedDiagnostics redacts secret-shaped strings from diagnostics
// served over HTTP. The on-demand nightly run aggregates error text from git,
// bank and model subsystems into its response; redacting at serve time keeps
// stored ledger rows verbatim (purge can still scrub them) while nothing
// secret-shaped leaves over the wire. Details without secrets pass through
// unchanged.
func redactServedDiagnostics(res NightlyResult) NightlyResult {
	stages := make([]NightlyStageResult, len(res.Stages))
	copy(stages, res.Stages)
	res.Stages = stages
	for i := range res.Stages {
		res.Stages[i].Detail = memory.Redact(res.Stages[i].Detail)
	}
	res.Error = memory.Redact(res.Error)
	return res
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// --- persistence: stateDir/enderbloom/sessions/{id}/meta.json + events.jsonl ---

func (d *Daemon) sessionDir(id string) string {
	return filepath.Join(d.stateDir, "enderbloom", "sessions", id)
}

func (d *Daemon) persistMeta(meta SessionMeta) {
	defer d.live.publish("sessions", "session:"+meta.ID)
	dir := d.sessionDir(meta.ID)
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(meta)
	// Atomic write: a crash mid-write never leaves a corrupt meta.json
	// that load would silently drop.
	tmp, err := os.CreateTemp(dir, "meta-*.json")
	if err != nil {
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return
	}
	_ = os.Rename(tmpName, filepath.Join(dir, "meta.json"))
}

func (d *Daemon) persistEvent(meta SessionMeta, e Event) {
	dir := d.sessionDir(meta.ID)
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(e)
	_, _ = f.Write(append(b, '\n'))
}

func (d *Daemon) load() {
	root := filepath.Join(d.stateDir, "enderbloom", "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, en := range entries {
		if !en.IsDir() {
			continue
		}
		id := en.Name()
		metaBytes, err := os.ReadFile(filepath.Join(root, id, "meta.json"))
		if err != nil {
			continue
		}
		var meta SessionMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}
		var events []Event
		if raw, err := os.ReadFile(filepath.Join(root, id, "events.jsonl")); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var e Event
				if err := json.Unmarshal([]byte(line), &e); err == nil {
					events = append(events, e)
				}
			}
		}
		// Order by sequence number so a log written under concurrency
		// still replays in order; malformed lines are skipped above.
		sort.Slice(events, func(i, j int) bool { return events[i].Seq < events[j].Seq })
		// Key by the metadata ID so the map key always matches meta.ID.
		key := meta.ID
		if key == "" {
			key = id
		}
		d.sessions[key] = &session{meta: meta, events: events, notify: make(chan struct{}), last: meta.CreatedAt, injectOp: handleResume}
		if len(events) > 0 {
			d.sessions[key].last = events[len(events)-1].CreatedAt
		}
	}
}

// absWorkDir expands a leading ~ and makes the path absolute: the claude
// adapter rejects relative working directories.
func absWorkDir(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}
