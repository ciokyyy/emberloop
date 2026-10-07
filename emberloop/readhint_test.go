package emberloop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"example.com/emberloop/tools"
)

// servedRead finds the served read tool for a role, failing the test when the
// role serves none.
func servedRead(t *testing.T, role string) tools.Tool {
	t.Helper()
	served, err := RoleTools(role)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range served {
		if tl.Name() == "read" {
			return tl
		}
	}
	t.Fatalf("%s serves no read tool, have %v", role, toolNames(served))
	return nil
}

// oversizedFile writes a file over the read limit and returns its path. read
// opens absolute paths anywhere, so the temp dir needs no workspace.
func oversizedFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, make([]byte, 3<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readPathErr(t *testing.T, rd tools.Tool, path string) error {
	t.Helper()
	args, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	_, err = rd.Execute(context.Background(), t.TempDir(), args)
	return err
}

// Ticket 18: a refused oversized read names bash for a role that has it.
func TestReadOversizeNamesBashForBashRoles(t *testing.T) {
	hermeticSettings(t)
	path := oversizedFile(t)
	for _, role := range []string{"work", "plan", "full"} {
		err := readPathErr(t, servedRead(t, role), path)
		if err == nil {
			t.Errorf("%s read rendered a file over the limit", role)
			continue
		}
		if !strings.Contains(err.Error(), "with bash") {
			t.Errorf("%s oversize refusal must name bash, got %q", role, err)
		}
	}
	t.Setenv("EMBERLOOP_AGENT_TYPE", "worker")
	t.Setenv("EMBERLOOP_AGENT_ID", "w1")
	if err := readPathErr(t, servedRead(t, "agent"), path); err == nil ||
		!strings.Contains(err.Error(), "with bash") {
		t.Errorf("a worker agent oversize refusal must name bash, got %v", err)
	}
}

// Ticket 18: a refused oversized read names tools.grep in execute or read
// with offset and limit for a role with no bash, never bash.
func TestReadOversizeNamesGrepForRolesWithoutBash(t *testing.T) {
	hermeticSettings(t)
	path := oversizedFile(t)
	for _, role := range []string{"implement", "review", "scout"} {
		err := readPathErr(t, servedRead(t, role), path)
		if err == nil {
			t.Errorf("%s read rendered a file over the limit", role)
			continue
		}
		msg := err.Error()
		for _, want := range []string{"tools.grep", "execute", "offset", "limit"} {
			if !strings.Contains(msg, want) {
				t.Errorf("%s oversize refusal must name %q, got %q", role, want, msg)
			}
		}
		if strings.Contains(msg, "bash") {
			t.Errorf("%s has no bash, so its refusal must not name it, got %q", role, msg)
		}
	}
	t.Setenv("EMBERLOOP_AGENT_TYPE", "scout")
	t.Setenv("EMBERLOOP_AGENT_ID", "s1")
	if err := readPathErr(t, servedRead(t, "agent"), path); err == nil ||
		!strings.Contains(err.Error(), "tools.grep") ||
		strings.Contains(err.Error(), "bash") {
		t.Errorf("a scout agent oversize refusal must name tools.grep and not bash, got %v", err)
	}
}
