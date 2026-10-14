package el

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// findSession returns the transcript under root whose file name carries id.
// The archiver and the usage tailer resolve harness transcripts through it.
func findSession(root, id string) string {
	var found string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".jsonl") && strings.Contains(d.Name(), id) {
			found = p
			return fs.SkipAll
		}
		if os.IsNotExist(err) {
			return fs.SkipAll
		}
		return nil
	})
	return found
}
