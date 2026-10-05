package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seed(t *testing.T, body string) (dir, path, tag string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, path, contentTag([]byte(body))
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const five = "one\ntwo\nthree\nfour\nfive\n"

func TestHashlineOps(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  string
	}{
		{"replace one line", "PUT 2.=2:\n+two-b", "one\ntwo-b\nthree\nfour\nfive\n"},
		{"replace a range", "PUT 2.=3:\n+x\n+y", "one\nx\ny\nfour\nfive\n"},
		{"insert before", "PUT <1:\n+zero", "zero\none\ntwo\nthree\nfour\nfive\n"},
		{"insert after", "PUT >2:\n+two-b", "one\ntwo\ntwo-b\nthree\nfour\nfive\n"},
		{"append", "PUT >$:\n+six", "one\ntwo\nthree\nfour\nfive\nsix\n"},
		{"cut one", "CUT 2", "one\nthree\nfour\nfive\n"},
		{"cut a range", "CUT 2.=3", "one\nfour\nfive\n"},
		{
			// The point of one-pass application: the later op keeps the
			// line number the model read, despite an earlier insert above it.
			name:  "two ops keep original anchors",
			patch: "PUT >1:\n+one-b\nCUT 4",
			want:  "one\none-b\ntwo\nthree\nfive\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, path, tag := seed(t, five)
			got, err := applyHashline(dir, "[a.txt#"+tag+"]\n"+tc.patch)
			if err != nil {
				t.Fatalf("apply: %v", err)
			}
			if body := read(t, path); body != tc.want {
				t.Errorf("file:\n got %q\nwant %q", body, tc.want)
			}
			if !strings.Contains(got, "#") {
				t.Errorf("result must carry the new tag, got %q", got)
			}
		})
	}
}

func TestHashlineTagChangesAfterEdit(t *testing.T) {
	dir, path, tag := seed(t, five)
	if _, err := applyHashline(dir, "[a.txt#"+tag+"]\nCUT 2"); err != nil {
		t.Fatal(err)
	}
	newTag := contentTag([]byte(read(t, path)))
	if newTag == tag {
		t.Fatal("editing the file must change its tag")
	}
	// The old tag is now stale and must be refused rather than applied.
	if _, err := applyHashline(dir, "[a.txt#"+tag+"]\nCUT 2"); err == nil {
		t.Error("a stale tag must be refused")
	} else if !strings.Contains(err.Error(), "stale") {
		t.Errorf("want a staleness refusal, got %v", err)
	}
}

func TestHashlineRefusals(t *testing.T) {
	cases := []struct {
		name     string
		patch    string
		noHeader bool
	}{
		{"no header", "PUT 1.=1:\n+x", true},
		{"overlapping ops", "PUT 1.=2:\n+x\nPUT 2.=3:\n+y", false},
		{"empty PUT body", "PUT 1.=1:", false},
		{"line past end", "PUT 99.=99:\n+x", false},
		{"content on the PUT line", "PUT 1.=1: +x", false},
		{"body without an op", "+orphan", false},
		{"bad CUT", "CUT one", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, path, tag := seed(t, five)
			patch := tc.patch
			if !tc.noHeader {
				patch = "[a.txt#" + tag + "]\n" + patch
			}
			if _, err := applyHashline(dir, patch); err == nil {
				t.Fatalf("expected a refusal; file is now %q", read(t, path))
			}
			if body := read(t, path); body != five {
				t.Errorf("a refused patch must not touch the file, got %q", body)
			}
		})
	}
}

func TestHashlineRefusesPathEscape(t *testing.T) {
	dir, _, _ := seed(t, five)
	if _, err := applyHashline(dir, "[../escape.txt#ABCD]\nCUT 1"); err == nil {
		t.Fatal("a patch naming a path outside the workspace must be refused")
	}
}

func TestReadPlainCarriesTagAndNumbers(t *testing.T) {
	dir, _, _ := seed(t, five)

	full, err := readPlain(dir, "a.txt", 0, 0)
	if err != nil {
		t.Fatalf("a plain text file must be rendered: %v", err)
	}
	if !strings.HasPrefix(full, "[a.txt#"+contentTag([]byte(five))+"]") {
		t.Errorf("read must open with the snapshot tag, got:\n%s", full)
	}
	if !strings.Contains(full, "     1→one") {
		t.Errorf("read must number lines from 1, got:\n%s", full)
	}

	window, err := readPlain(dir, "a.txt", 2, 2)
	if err != nil {
		t.Fatalf("ranged read failed: %v", err)
	}
	if strings.Contains(window, "→one") || !strings.Contains(window, "→two") {
		t.Errorf("offset/limit not honoured, got:\n%s", window)
	}
}

func TestReadPlainOpensAbsolutePathsAnywhere(t *testing.T) {
	dir, path, _ := seed(t, five)
	// A model routinely hands back the absolute path it was just given.
	if _, err := readPlain(dir, path, 0, 0); err != nil {
		t.Fatalf("an absolute path inside the workspace must be served: %v", err)
	}
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.txt")
	if err := os.WriteFile(elsewhere, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPlain(dir, elsewhere, 0, 0); err != nil {
		t.Fatalf("an absolute path outside the workspace must be served: %v", err)
	}
}

// There is no delegation to pig: ctx.ExecuteTool("read") resolves back to this
// tool, so every refusal has to carry its own reason.
func TestReadPlainRefusesWithAReason(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), []byte{0x00, 0x01, 0x02, 0xff}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.txt"), make([]byte, maxServedFile+1), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ path, want string }{
		{"blob.bin", "binary"},
		{"big.txt", "read limit"},
		{"missing.txt", "no such file"},
	} {
		_, err := readPlain(dir, tc.path, 0, 0)
		if err == nil {
			t.Errorf("readPlain(%q) rendered a file it must refuse", tc.path)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("readPlain(%q) = %q, must say %q so the model knows what to do", tc.path, err, tc.want)
		}
	}
}
