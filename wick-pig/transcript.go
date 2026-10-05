package wick

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// newSessionID is a random UUID. claude requires one; pig takes any string.
func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// findSession returns the transcript under root whose file name carries id.
func findSession(root, id string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") && strings.Contains(d.Name(), id) {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// lastRecord decodes the final complete JSON line that satisfies pick.
// ponytail: rereads the whole file each poll; fine for review-sized sessions.
func lastRecord(path string, pick func(map[string]any) (string, bool)) (text string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var rec map[string]any
		if json.Unmarshal(sc.Bytes(), &rec) != nil {
			continue
		}
		if t, hit := pick(rec); hit {
			text, ok = t, true
		}
	}
	return text, ok
}

// textOf joins the text blocks of a message content array.
func textOf(content any) string {
	blocks, _ := content.([]any)
	var parts []string
	for _, b := range blocks {
		if m, _ := b.(map[string]any); m["type"] == "text" {
			if s, _ := m["text"].(string); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

// WaitReply polls the session transcript until the agent ends a turn, and
// returns that last message. It is the same text a human sees last in the pane.
func WaitReply(h Harness, s Session, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if p := findSession(h.SessionRoot(s), s.ID); p != "" {
			if text, ok := h.LastReply(p); ok && text != "" {
				return text, nil
			}
		}
		time.Sleep(pollEvery)
	}
	return "", fmt.Errorf("%s did not finish a turn within %s", h.Name(), timeout)
}
