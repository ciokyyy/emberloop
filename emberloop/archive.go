package emberloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"example.com/emberloop/tools"
)

// Archive layout (ticket 05; the stats indexer reads this).
//
// All paths are siblings of the worker store, under emberloop's state directory
// (the directory holding workers.json):
//
//	blobs/<hex>                          one file per distinct content, named
//	                                     for its SHA-256; the system prompt,
//	                                     the tool definitions served for the
//	                                     session's role, and the harness
//	                                     flags each land here once no matter
//	                                     how many sessions share them.
//	archive/<run>/<agent>/session.json   the session's archive record: run
//	                                     slug, agent id, harness, role, start
//	                                     time, the three blob hashes
//	                                     (sha256:<hex>), the transcript copy
//	                                     name and the source transcript path
//	                                     (filled in by the first copy that
//	                                     finds it; the snapshot runs before
//	                                     the first turn exists).
//	archive/<run>/<agent>/transcript.jsonl
//	                                     verbatim copy of the harness
//	                                     transcript, appended by byte offset
//	                                     so it survives harness cleanup.
//	archive/<run>/<agent>/transcript.offset
//	                                     "<source offset> <copy size>": bytes
//	                                     of the source already copied, and the
//	                                     copy's own size. A restart resumes
//	                                     from the offset and cuts a torn tail
//	                                     back instead of re-copying it.
//	archive/<run>/<agent>/usage.jsonl    NOT HERE: the per-agent usage log is
//	                                     a sibling of the archive, placed by
//	                                     ticket 03 (`UsageLogPath`:
//	                                     <state>/usage/<agent-id>.jsonl).
//	archive/<run>/<agent>/memory.jsonl   written by ticket 06: memory samples.
//
// <run> is the spec slug (specSlug of EMBERLOOP_SPEC), or "adhoc" for a session
// with no spec. <agent> is EMBERLOOP_AGENT_ID, sanitised for the filesystem the
// way launch files are. Nothing is ever deleted automatically; `emberloop
// stats prune` (ticket 07) removes runs and the blobs nothing references.

// blobDir holds one file per distinct content, beside the worker store.
func blobDir() string { return filepath.Join(filepath.Dir(statePath), "blobs") }

// archiveRoot holds one directory per run, beside the worker store.
func archiveRoot() string { return filepath.Join(filepath.Dir(statePath), "archive") }

// ArchiveSize is the bytes the archive and blob store hold on disk, for the
// dashboard's Settings tab. Missing directories read as zero; sizes are best
// effort the way prune's are.
func ArchiveSize() int64 { return dirSize(archiveRoot()) + dirSize(blobDir()) }

// ArchiveAgentDir is the archive directory of one agent's session, for the
// stats indexer and pruner to read as plain files.
func ArchiveAgentDir(run, agent string) string {
	return filepath.Join(archiveRoot(), sanitizeArchiveName(run), sanitizeArchiveName(agent))
}

func sanitizeArchiveName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	if s == "" {
		return "session"
	}
	return s
}

// BlobHash names content in the blob store: sha256:<hex of SHA-256>.
func BlobHash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// BlobPath is where the blob for hash lives. The hash names the file, so the
// same content always lands in the same place.
func BlobPath(hash string) string {
	return filepath.Join(blobDir(), strings.TrimPrefix(hash, "sha256:"))
}

// StoreBlob writes data to the blob store when it is not there yet and
// returns its content hash. Storing the same bytes twice leaves one file:
// identical prompts and tool sets across sessions share one blob.
func StoreBlob(data []byte) (string, error) {
	hash := BlobHash(data)
	path := BlobPath(hash)
	if _, err := os.Stat(path); err == nil {
		return hash, nil
	}
	if err := os.MkdirAll(blobDir(), 0o755); err != nil {
		return "", err
	}
	// Temp file and rename, so a crash mid-write never leaves a truncated
	// blob behind for the Stat fast path above to mistake for the real one.
	// Two sessions storing the same new blob race on the rename; both wrote
	// the same bytes, so either outcome is the wanted file.
	tmp, err := os.CreateTemp(blobDir(), "blob-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return "", err
	}
	return hash, nil
}

// ReadBlob returns the blob stored under hash.
func ReadBlob(hash string) ([]byte, error) {
	raw, err := os.ReadFile(BlobPath(hash))
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// RunSlugForSpec is the archive's run key for a spec path: the spec slug, or
// "adhoc" for a session outside any run. specSlug already yields filesystem
// -safe names; only the empty case needs a fallback.
func RunSlugForSpec(spec string) string {
	if strings.TrimSpace(spec) == "" {
		return "adhoc"
	}
	if slug := specSlug(spec); slug != "" {
		return slug
	}
	return "adhoc"
}

// SystemPrompt is the rule set every emberloop session carries, stored once in the
// blob store at session start so its cost can be counted. It is the same
// prompt the harness commands append to.
func SystemPrompt() string { return systemPrompt }

// ToolDefsJSON marshals the tool definitions served for a role, in the shape
// the MCP server lists them (name, description, inputSchema, readOnlyHint
// where set). The same role serves the same bytes, so its sessions share one
// blob.
func ToolDefsJSON(served []tools.Tool) ([]byte, error) {
	listed := make([]map[string]any, 0, len(served))
	for _, t := range served {
		entry := map[string]any{
			"name": t.Name(), "description": t.Description(), "inputSchema": t.Schema(),
		}
		if ro, ok := t.(tools.ReadOnly); ok && ro.ReadOnly() {
			entry["annotations"] = map[string]any{"readOnlyHint": true}
		}
		listed = append(listed, entry)
	}
	return json.Marshal(listed)
}

// FlagsJSON marshals the harness flags a session started with. Only the
// stable choice (harness, model, effort) is stored, never the prompt, so
// sessions launched the same way share one blob.
func FlagsJSON(harness, model, effort string) []byte {
	raw, _ := json.Marshal(map[string]string{
		"harness": harness, "model": model, "effort": effort,
	})
	return raw
}

// TranscriptFile is the transcript copy's name inside an agent's archive dir.
const TranscriptFile = "transcript.jsonl"

// SessionSnapshot is one session's start-of-session capture: who it is and
// the three contents to store once by hash.
type SessionSnapshot struct {
	Run, Agent, Harness, Role, Started string
	SystemPrompt                       string
	ToolsJSON, FlagsJSON               []byte
	TranscriptSource                   string
}

// SessionRecord is the session.json on disk in an agent's archive dir: the
// session's identity and the blob hashes its prompt, tools and flags share
// with every identical session.
type SessionRecord struct {
	Run              string `json:"run"`
	Agent            string `json:"agent"`
	Harness          string `json:"harness,omitempty"`
	Role             string `json:"role,omitempty"`
	Started          string `json:"started,omitempty"`
	SystemPrompt     string `json:"system_prompt,omitempty"`
	Tools            string `json:"tools,omitempty"`
	Flags            string `json:"flags,omitempty"`
	Transcript       string `json:"transcript,omitempty"`
	TranscriptSource string `json:"transcript_source,omitempty"`
}

// SessionFilePath is one agent's archive record, for readers to poll as a
// plain file.
func SessionFilePath(run, agent string) string {
	return filepath.Join(ArchiveAgentDir(run, agent), "session.json")
}

// SnapshotSession stores the snapshot's prompt, tool definitions and flags in
// the blob store and writes (or refreshes) the session's archive record with
// their hashes. It is idempotent: snapshotting the same session twice keeps
// one record, and identical contents across sessions keep one blob each.
func SnapshotSession(s SessionSnapshot) (SessionRecord, error) {
	return snapshotSession(s, time.Now().UTC().Format(time.RFC3339))
}

func snapshotSession(s SessionSnapshot, started string) (SessionRecord, error) {
	if s.Run == "" || s.Agent == "" {
		return SessionRecord{}, &SnapshotError{"run and agent are required"}
	}
	var rec SessionRecord
	if raw, err := os.ReadFile(SessionFilePath(s.Run, s.Agent)); err == nil {
		_ = json.Unmarshal(raw, &rec)
	}
	if s.Started != "" {
		rec.Started = s.Started
	} else if rec.Started == "" {
		rec.Started = started
	}
	for _, field := range []struct {
		data []byte
		set  func(string)
	}{
		{[]byte(s.SystemPrompt), func(h string) { rec.SystemPrompt = h }},
		{s.ToolsJSON, func(h string) { rec.Tools = h }},
		{s.FlagsJSON, func(h string) { rec.Flags = h }},
	} {
		if len(field.data) == 0 {
			continue
		}
		h, err := StoreBlob(field.data)
		if err != nil {
			return SessionRecord{}, err
		}
		field.set(h)
	}
	if s.Run != "" {
		rec.Run = s.Run
	}
	if s.Agent != "" {
		rec.Agent = s.Agent
	}
	if s.Harness != "" {
		rec.Harness = s.Harness
	}
	if s.Role != "" {
		rec.Role = s.Role
	}
	if s.TranscriptSource != "" {
		rec.TranscriptSource = s.TranscriptSource
	}
	if rec.Transcript == "" {
		rec.Transcript = TranscriptFile
	}
	if err := writeSessionRecord(rec); err != nil {
		return SessionRecord{}, err
	}
	return rec, nil
}

// writeSessionRecord stores the session's archive record through a temp file
// and a rename, so a reader never sees a half-written file.
func writeSessionRecord(rec SessionRecord) error {
	if err := os.MkdirAll(ArchiveAgentDir(rec.Run, rec.Agent), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	path := SessionFilePath(rec.Run, rec.Agent)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// recordTranscriptSource remembers where the archived transcript came from.
// The snapshot at session start runs before the harness writes its first
// turn, so it usually records nothing; the first copy that finds the
// transcript fills the session record in, and the final copy refreshes it.
// Without a session record there is nothing to fill in.
func recordTranscriptSource(run, agent, src string) error {
	return withFileLock("archive-"+sanitizeArchiveName(run)+"-"+sanitizeArchiveName(agent), func() error {
		raw, err := os.ReadFile(SessionFilePath(run, agent))
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		var rec SessionRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		if rec.TranscriptSource == src {
			return nil
		}
		rec.TranscriptSource = src
		return writeSessionRecord(rec)
	})
}

// SnapshotError is a caller error: the snapshot was never taken.
type SnapshotError struct{ msg string }

func (e *SnapshotError) Error() string { return e.msg }

// TranscriptCopyPath is the archive's copy of one agent's harness transcript.
// It is appended to by byte offset, so it stays current while the agent runs
// and outlives the harness cleaning its own transcript up.
func TranscriptCopyPath(run, agent string) string {
	return filepath.Join(ArchiveAgentDir(run, agent), TranscriptFile)
}

// TranscriptOffsetPath is where the copy's progress lives: the source offset
// already copied, followed by the archive copy's own size. The second number
// repairs a torn tail: bytes appended after a crash that took the process
// before the offset was stored are cut back, never re-copied.
func TranscriptOffsetPath(run, agent string) string {
	return filepath.Join(ArchiveAgentDir(run, agent), "transcript.offset")
}

// TranscriptOffset reports how many source bytes are already archived.
func TranscriptOffset(run, agent string) (int64, error) {
	off, _, err := transcriptProgress(run, agent)
	return off, err
}

// transcriptProgress reads the offset file: "<source offset> <copy size>".
// A file with only the offset (written by no released version) repairs
// nothing and reports a copy size of -1.
func transcriptProgress(run, agent string) (off, size int64, err error) {
	raw, err := os.ReadFile(TranscriptOffsetPath(run, agent))
	if os.IsNotExist(err) {
		return 0, -1, nil
	}
	if err != nil {
		return 0, -1, err
	}
	fields := strings.Fields(strings.TrimSpace(string(raw)))
	if len(fields) == 0 || len(fields) > 2 {
		return 0, -1, fmt.Errorf("transcript offset is corrupt: %q", strings.TrimSpace(string(raw)))
	}
	off, err = strconv.ParseInt(fields[0], 10, 64)
	if err != nil || off < 0 {
		return 0, -1, fmt.Errorf("transcript offset is corrupt: %q", strings.TrimSpace(string(raw)))
	}
	size = -1
	if len(fields) == 2 {
		size, err = strconv.ParseInt(fields[1], 10, 64)
		if err != nil || size < 0 {
			return 0, -1, fmt.Errorf("transcript offset is corrupt: %q", strings.TrimSpace(string(raw)))
		}
	}
	return off, size, nil
}

func writeTranscriptProgress(run, agent string, off, size int64) error {
	return os.WriteFile(TranscriptOffsetPath(run, agent),
		[]byte(strconv.FormatInt(off, 10)+" "+strconv.FormatInt(size, 10)+"\n"), 0o644)
}

// CopyTranscript appends whatever the harness transcript at src grew by since
// the last copy to the agent's archive, and advances the stored offset. It is
// how the archive tracks a running session and how a settled session takes
// its final copy with one last call. A source that is not there yet (the
// harness has written nothing) or is gone (harness cleanup) is not an error:
// the copy keeps whatever it already holds. A source shorter than the offset
// was replaced, so the copy restarts from its beginning.
func CopyTranscript(src, run, agent string) (int64, error) {
	var copied int64
	err := withFileLock("archive-"+sanitizeArchiveName(run)+"-"+sanitizeArchiveName(agent), func() error {
		n, err := copyTranscript(src, run, agent)
		copied = n
		return err
	})
	return copied, err
}

func copyTranscript(src, run, agent string) (int64, error) {
	off, wantSize, err := transcriptProgress(run, agent)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(src)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(ArchiveAgentDir(run, agent), 0o755); err != nil {
		return 0, err
	}
	dst := TranscriptCopyPath(run, agent)
	if info.Size() < off {
		// The harness replaced its transcript: restart the copy rather
		// than seek past the new file's start, and record the clean
		// state even when the new transcript is still empty.
		if err := os.WriteFile(dst, nil, 0o644); err != nil {
			return 0, err
		}
		off, wantSize = 0, 0
		if err := writeTranscriptProgress(run, agent, 0, 0); err != nil {
			return 0, err
		}
	}
	if wantSize >= 0 {
		st, statErr := os.Stat(dst)
		switch {
		case os.IsNotExist(statErr) && (wantSize > 0 || off > 0):
			// The copy is gone while progress says otherwise: rebuild it
			// from the source start rather than append a tail onto nothing.
			off = 0
			if err := writeTranscriptProgress(run, agent, 0, 0); err != nil {
				return 0, err
			}
		case os.IsNotExist(statErr):
			// Fresh: neither copy nor progress yet; the append below
			// creates the copy.
		case statErr != nil:
			return 0, statErr
		case st.Size() > wantSize:
			// The last copy appended bytes it never recorded (a crash
			// between the append and the offset write): cut the torn
			// tail back instead of copying those bytes a second time.
			// Truncate never extends here: a shorter copy is rebuilt
			// below. This runs before the up-to-date check, so a torn
			// tail is repaired even when the source has nothing new.
			f, err := os.OpenFile(dst, os.O_WRONLY, 0o644)
			if err != nil {
				return 0, err
			}
			if err := f.Truncate(wantSize); err != nil {
				_ = f.Close()
				return 0, err
			}
			if err := f.Close(); err != nil {
				return 0, err
			}
		case st.Size() < wantSize:
			// The copy lost bytes it had recorded: rebuild it from the
			// source start. The source still holds everything the copy
			// should contain, so nothing is lost.
			if err := os.WriteFile(dst, nil, 0o644); err != nil {
				return 0, err
			}
			off = 0
			if err := writeTranscriptProgress(run, agent, 0, 0); err != nil {
				return 0, err
			}
		}
	}
	if info.Size() == off {
		return 0, nil
	}
	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer in.Close()
	if _, err := in.Seek(off, 0); err != nil {
		return 0, err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if syncErr != nil {
		return 0, syncErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	st, err := os.Stat(dst)
	if err != nil {
		return 0, err
	}
	if err := writeTranscriptProgress(run, agent, off+n, st.Size()); err != nil {
		return 0, err
	}
	return n, nil
}

// TranscriptSource finds the harness transcript of the session named by a
// session id (or, when the harness keys transcripts by label, an agent id)
// under an optional session dir. It reports "" while the harness has written
// nothing: the archiver retries on its next tick.
func TranscriptSource(harness, sessionID, agentID, sessionDir string) string {
	names := []string{}
	if h := Registered(harness); h != nil {
		names = []string{harness}
	} else {
		names = HarnessNames()
	}
	keys := []string{}
	for _, k := range []string{sessionID, agentID} {
		if k != "" {
			keys = append(keys, k)
		}
	}
	for _, name := range names {
		h := Registered(name)
		if h == nil {
			continue
		}
		root := h.SessionRoot(Session{ID: sessionID, Dir: sessionDir})
		for _, k := range keys {
			if p := findSession(root, k); p != "" {
				return p
			}
		}
	}
	return ""
}

// archiveEnv reads the archiver's session out of the environment the launcher
// set (roleEnv plus the launch choice). Empty means a emberloop-mcp started by
// hand, which archives nothing.
func archiveEnv() (run, agent, harness, role, sessionID, sessionDir, model, effort string) {
	agent = os.Getenv("EMBERLOOP_AGENT_ID")
	sessionID = os.Getenv("EMBERLOOP_SESSION_ID")
	if sessionID == "" {
		sessionID = agent
	}
	return RunSlugForSpec(os.Getenv("EMBERLOOP_SPEC")), agent,
		os.Getenv("EMBERLOOP_HARNESS"), os.Getenv("EMBERLOOP_ROLE"),
		sessionID, os.Getenv("EMBERLOOP_SESSION_DIR"),
		os.Getenv("EMBERLOOP_MODEL"), os.Getenv("EMBERLOOP_EFFORT")
}

// SnapshotFromEnv stores the calling session's system prompt, served tool
// definitions and harness flags in the blob store and references them from
// the session's archive. emberloop-mcp calls it once at startup, so the blobs
// exist before any request pays for them. A process with no agent id (a
// hand-started server) records nothing.
func SnapshotFromEnv(served []tools.Tool) error {
	run, agent, harness, role, sessionID, sessionDir, model, effort := archiveEnv()
	if agent == "" {
		return nil
	}
	toolsJSON, err := ToolDefsJSON(served)
	if err != nil {
		return err
	}
	_, err = SnapshotSession(SessionSnapshot{
		Run: run, Agent: agent, Harness: harness, Role: role,
		SystemPrompt: systemPrompt, ToolsJSON: toolsJSON,
		FlagsJSON:        FlagsJSON(harness, model, effort),
		TranscriptSource: TranscriptSource(harness, sessionID, agent, sessionDir),
	})
	return err
}

// CopyFromEnv appends whatever the calling session's harness transcript grew
// by to its archive, and records where the transcript lives the first time
// it is found. The archiver calls it every tick and once more when the
// session settles, so the copy is current while the agent runs and final
// afterwards. No transcript yet, or no agent id, copies nothing.
func CopyFromEnv() (int64, error) {
	run, agent, harness, _, sessionID, sessionDir, _, _ := archiveEnv()
	if agent == "" {
		return 0, nil
	}
	src := TranscriptSource(harness, sessionID, agent, sessionDir)
	if src == "" {
		return 0, nil
	}
	if err := recordTranscriptSource(run, agent, src); err != nil {
		return 0, err
	}
	return CopyTranscript(src, run, agent)
}

// archivePoll is how often the session's own emberloop process tails its harness
// transcript into the archive. A var so tests can shrink it.
var archivePoll = 5 * time.Second

// StartArchiveLoop tails the calling session's harness transcript into its
// archive until the returned stop is called. emberloop-mcp starts it while it
// serves and stops it before its final copy, so the archive stays current
// whether or not the TUI is open. Without an agent id it does nothing.
func StartArchiveLoop() (stop func()) {
	if os.Getenv("EMBERLOOP_AGENT_ID") == "" {
		return func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(archivePoll)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = CopyFromEnv()
			}
		}
	}()
	return func() { cancel(); <-done }
}
