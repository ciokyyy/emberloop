package el

import (
	"os"
	"path/filepath"
	"testing"
)

const (
	errRec  = `{"type":"message","message":{"role":"assistant","stopReason":"error","errorMessage":"Provider returned an empty response","content":[]}}` + "\n"
	doneRec = `{"type":"message","message":{"role":"assistant","stopReason":"stop","content":[{"type":"text","text":"ok"}]}}` + "\n"
	runRec  = `{"type":"message","message":{"role":"assistant","stopReason":"toolUse","content":[]}}` + "\n"
)

func TestTurnState(t *testing.T) {
	d := t.TempDir()
	for rec, want := range map[string]string{errRec: TurnFailed, doneRec: TurnDone, runRec: TurnRunning} {
		p := filepath.Join(d, "t.jsonl")
		os.WriteFile(p, []byte(rec), 0o644)
		if got, _ := (pigHarness{}).TurnState(p); got != want {
			t.Errorf("%q: %s, want %s", rec, got, want)
		}
	}
}
