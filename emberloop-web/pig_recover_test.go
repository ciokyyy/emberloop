package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A stack wedged inside a provider stream is replaced by one resumed from the
// saved pig Session: the Daemon id lives on, the transcript carries over, and
// the fresh stack answers the next prompt.
func TestPigRecoverWedgedRebuildsFromSavedSession(t *testing.T) {
	t.Parallel()
	pb := newPigTestBackend(t)
	pb.LogDir = tempDir(t)
	meta := SessionMeta{ID: "wedge1", Harness: "pig", Model: pigFauxModel, WorkDir: tempDir(t)}
	emit := func(kind, text, status string) {}
	if err := pb.CreateSession(meta, emit); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pb.CloseSession(meta.ID) })

	pb.OnPrompt(meta.ID, "What is 20+22?", func(kind, text, status string) {})

	old := pb.sessions[meta.ID]
	fresh, err := pb.recoverWedged(meta.ID)
	if err != nil {
		t.Fatalf("recoverWedged: %v", err)
	}
	if fresh == old {
		t.Fatal("recover returned the stuck stack")
	}
	if fresh.sess.ID() == "" {
		t.Fatal("rebuilt stack has no pig Session id")
	}
	if !pb.alive(meta.ID) {
		t.Fatal("session not registered after recovery")
	}

	var kinds []string
	pb.OnPrompt(meta.ID, "What is 20+22?", func(kind, text, status string) { kinds = append(kinds, kind+status) })
	if len(kinds) == 0 || kinds[len(kinds)-1] != "statusidle" {
		t.Fatalf("rebuilt stack did not finish a turn: %v", kinds)
	}

	b, err := os.ReadFile(filepath.Join(pb.LogDir, "pig-wedge1.log"))
	if err != nil {
		t.Fatalf("pig log missing: %v", err)
	}
	if !strings.Contains(string(b), `"kind":"recover"`) || !strings.Contains(string(b), `"kind":"sdk_event"`) {
		t.Fatalf("log lacks recover or sdk_event lines:\n%s", b)
	}
}

func TestPigLogNilIsSilent(t *testing.T) {
	t.Parallel()
	var l *pigLog
	l.write("x", nil) // must not panic
	if newPigLog("", "id") != nil {
		t.Fatal("empty dir must give a nil log")
	}
}
