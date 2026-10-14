package el

import (
	"os"
	"path/filepath"
	"strings"
)

// docsNamespace is the directory under the skills root that holds el's
// own agent docs, served as el://docs/<name>.
const docsNamespace = "docs"

// docsSourceDir is the checkout's agent docs: docs/agent beside the module
// the install builds from. EL_DOCS_DIR points it elsewhere.
func docsSourceDir() string {
	if dir := os.Getenv("EL_DOCS_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(installSourceDir(), "..", "docs", "agent")
}

// SyncAgentDocs copies every markdown page of the checkout's docs/agent into
// the docs namespace, so a session reads them with el://docs/<name>.
// A checkout without the directory is no pages, not an error. Pages the
// checkout dropped are removed so the namespace mirrors the source.
func SyncAgentDocs() (int, error) {
	src := docsSourceDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	dst := filepath.Join(skillsRoot(), docsNamespace)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return 0, err
	}
	want := map[string]bool{}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return n, err
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), raw, 0o644); err != nil {
			return n, err
		}
		want[e.Name()] = true
		n++
	}
	old, _ := os.ReadDir(dst)
	for _, e := range old {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !want[e.Name()] {
			_ = os.Remove(filepath.Join(dst, e.Name()))
		}
	}
	return n, nil
}
