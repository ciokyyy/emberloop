package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// autoDaemon starts a fake-Backend Daemon with auto-compress on, a passing
// compress model and a scripted summarizer. UsageSize controls how fast the
// context meter fills; threshold and cooldown set the auto knobs.
func autoDaemon(t *testing.T, usageSize, threshold, cooldown int) (*Daemon, *FakeBackend, string, string) {
	t.Helper()
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":`+strconv.Itoa(threshold)+`,"compress_cooldown":`+strconv.Itoa(cooldown)+`}`)
	be := NewFakeBackend()
	be.UsageSize = usageSize
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	t.Cleanup(srv.Close)
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	return d, be, srv.URL, id
}

func compressedCount(t *testing.T, base, id string) int {
	t.Helper()
	return len(eventsOfKind(transcriptOf(t, base, id), "compressed"))
}

// With auto on, a Session at or past the threshold compresses by Handoff;
// below it nothing runs.
func TestAutoCompressFiresAtThreshold(t *testing.T) {
	_, _, base, id := autoDaemon(t, 40, 10, 0)
	sendPrompt(t, base, id, "hello")
	evs := waitIdleAfter(t, base, id, 1)
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed = %d, want 1 past the threshold", n)
	}
	hand := eventsOfKind(evs, "prompt")
	last := hand[len(hand)-1]
	if !last.Handoff || last.Sender != "system" {
		t.Fatalf("last prompt = %+v, want the system Handoff", last)
	}

	// Below the threshold nothing runs.
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":99,"compress_cooldown":0}`)
	be := NewFakeBackend()
	be.UsageSize = 1000000
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	quiet := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, quiet, "hello")
	waitStatus(t, srv.URL, quiet, "idle", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if n := compressedCount(t, srv.URL, quiet); n != 0 {
		t.Fatalf("compressed = %d below the threshold, want 0", n)
	}
}

// Auto fires only after the turn ends, never mid-token: while the Session
// works nothing compresses, even past the threshold.
func TestAutoCompressWaitsForTurnEnd(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":10,"compress_cooldown":0}`)
	be := NewFakeBackend()
	be.Delay = 800 * time.Millisecond
	be.UsageSize = 40
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))

	sendPrompt(t, srv.URL, id, "slow job")
	waitStatus(t, srv.URL, id, "working", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if n := compressedCount(t, srv.URL, id); n != 0 {
		t.Fatalf("compressed = %d mid-turn, want 0", n)
	}
	evs := waitIdleAfter(t, srv.URL, id, 1)
	if n := len(eventsOfKind(evs, "compressed")); n != 1 {
		t.Fatalf("compressed = %d after the turn, want 1", n)
	}
}

// The idle gate holds while a turn runs even when the meter is already past
// the threshold: a direct auto check mid-turn starts nothing, and the turn
// end still compresses.
func TestAutoCompressSkipsWhileBusy(t *testing.T) {
	d, be, base, id := autoDaemon(t, 40, 10, 0)
	sendPrompt(t, base, id, "one")
	waitIdleAfter(t, base, id, 1)
	be.Delay = 800 * time.Millisecond
	sendPrompt(t, base, id, "two")
	waitStatus(t, base, id, "working", 5*time.Second)
	d.appendEvent(id, "usage", usageText(90, 100), "")
	d.maybeAutoCompress(id)
	time.Sleep(200 * time.Millisecond)
	if n := compressedCount(t, base, id); n != 1 {
		t.Fatalf("compressed = %d while busy past the threshold, want 1", n)
	}
	evs := waitIdleAfter(t, base, id, 2)
	if n := len(eventsOfKind(evs, "compressed")); n != 2 {
		t.Fatalf("compressed = %d after the turn, want 2", n)
	}
}

// An open tool call holds auto at the boundary: over the threshold with a
// tool started and never finished, no Handoff runs. Once the tool finishes,
// the next turn end compresses.
func TestAutoCompressWaitsForToolBoundary(t *testing.T) {
	d, _, base, id := autoDaemon(t, 40, 10, 0)
	sendPrompt(t, base, id, "first")
	waitStatus(t, base, id, "idle", 5*time.Second)
	// First prompt already auto-compressed once; seed an open tool after it.
	d.appendEvent(id, "tool_call_started", `{"id":"t1","name":"read","input":"{}","output":"","status":"running"}`, "running")
	d.appendEvent(id, "usage", usageText(90, 100), "")
	d.maybeAutoCompress(id)
	time.Sleep(200 * time.Millisecond)
	if n := compressedCount(t, base, id); n != 1 {
		t.Fatalf("compressed = %d with an open tool, want 1 (no new Handoff)", n)
	}
	d.appendEvent(id, "tool_call_updated", `{"id":"t1","name":"read","input":"{}","output":"ok","status":"completed"}`, "completed")
	sendPrompt(t, base, id, "after the tool")
	evs := waitIdleAfter(t, base, id, 2)
	if n := len(eventsOfKind(evs, "compressed")); n != 2 {
		t.Fatalf("compressed = %d after the tool finished, want 2", n)
	}
}

// Cooldown counts turns since the last compress: with cooldown 2 the next
// prompt stays on the old handle and the one after compresses again.
func TestAutoCompressCooldown(t *testing.T) {
	_, _, base, id := autoDaemon(t, 40, 10, 2)
	sendPrompt(t, base, id, "one")
	waitIdleAfter(t, base, id, 1)

	sendPrompt(t, base, id, "two")
	waitStatus(t, base, id, "idle", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if n := compressedCount(t, base, id); n != 1 {
		t.Fatalf("compressed = %d within cooldown, want 1", n)
	}

	sendPrompt(t, base, id, "three")
	evs := waitIdleAfter(t, base, id, 2)
	if n := len(eventsOfKind(evs, "compressed")); n != 2 {
		t.Fatalf("compressed = %d past cooldown, want 2", n)
	}
}

// With the compress model unset, auto shows its warning and does not run:
// the Session GET names the reason and the log holds no Handoff.
func TestAutoCompressUnsetModelWarns(t *testing.T) {
	useSettings(t, `{"auto_compress":true,"compress_threshold_percent":10,"compress_cooldown":0}`)
	be := NewFakeBackend()
	be.UsageSize = 40
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &scriptedSummarizer{replies: []string{goodSummary}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	waitStatus(t, srv.URL, id, "idle", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if n := compressedCount(t, srv.URL, id); n != 0 {
		t.Fatalf("compressed = %d with no model, want 0", n)
	}
	if note := sessionField(t, srv.URL, id, "compressNote"); !strings.Contains(note, "no compress model") {
		t.Fatalf("compressNote = %q, want the unset warning", note)
	}
}

// A blocked Session does not auto-compress again until Retry or dismiss:
// the first auto blocks on credits, the second auto starts nothing new.
func TestAutoCompressBlockedStaysBlocked(t *testing.T) {
	t.Setenv("PIG_TEST_FAUX", "1")
	useSettings(t, `{"compress_model":"test-faux/faux-1","auto_compress":true,"compress_threshold_percent":10,"compress_cooldown":0}`)
	be := NewFakeBackend()
	be.UsageSize = 40
	d := NewDaemon(tempDir(t), be)
	d.summarizer = &stepSummarizer{steps: []summaryStep{{err: errString("402 Payment Required")}}}
	srv := httptest.NewServer(authedHandler(t, d))
	defer srv.Close()
	id := createSession(t, srv.URL, "fake", "fake-model", tempDir(t))
	sendPrompt(t, srv.URL, id, "hello")
	evs := waitSettled(t, srv.URL, id, hasBlockedEvent)
	if n := len(eventsOfKind(evs, "compressed")); n != 0 {
		t.Fatalf("compressed = %d on a block, want 0", n)
	}
	d.maybeAutoCompress(id)
	time.Sleep(200 * time.Millisecond)
	evs = transcriptOf(t, srv.URL, id)
	if n := len(eventsOfKind(evs, "compress_blocked")); n != 1 {
		t.Fatalf("blocked events = %d after a second auto, want 1", n)
	}
	if meta := sessionMetaOf(t, srv.URL, id); meta.CompressBlock == "" {
		t.Fatal("block cleared by an auto retry, want it held")
	}
	if code, _ := postMode(t, srv.URL, id, "prompt", "while blocked"); code != http.StatusAccepted {
		t.Fatalf("held prompt status = %d, want 202", code)
	}
	time.Sleep(200 * time.Millisecond)
	if n := len(eventsOfKind(transcriptOf(t, srv.URL, id), "compress_blocked")); n != 1 {
		t.Fatalf("blocked events grew while blocked")
	}
}
