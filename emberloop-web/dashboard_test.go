package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestDashboardHTTPGroupsActiveSessionsAndRunsByProject(t *testing.T) {
	t.Parallel()

	d := NewDaemon(tempDir(t), NewFakeBackend())
	parentPath := filepath.Join(tempDir(t), "workspace")
	nestedPath := filepath.Join(parentPath, "nested")
	d.projects = []Project{
		{ID: "project-parent", Name: "Workspace", Path: parentPath},
		{ID: "project-nested", Name: "Nested", Path: nestedPath},
	}
	handler := d.Handler()
	cookie := signedIn(t, handler, "admin")
	user := userOf(t, handler, cookie)

	last := time.Date(2026, time.October, 14, 10, 0, 0, 0, time.UTC)
	makeSession := func(id, projectID, status, workDir string, owner string) *session {
		return &session{
			meta: SessionMeta{
				ID: id, Owner: owner, ProjectID: projectID, Status: status,
				Harness: "fake", Model: "test-model", WorkDir: workDir,
			},
			last: last,
		}
	}
	d.mu.Lock()
	d.sessions = map[string]*session{
		"working-nested": makeSession("working-nested", "project-nested", "working", nestedPath, user.ID),
		"failed-parent":  makeSession("failed-parent", "project-parent", "failed", parentPath, user.ID),
		"idle-parent":    makeSession("idle-parent", "project-parent", "idle", parentPath, user.ID),
		"ended-parent":   makeSession("ended-parent", "project-parent", "ended", parentPath, user.ID),
		"home-session":   makeSession("home-session", "", "working", d.users.homeDir(user.Username), user.ID),
		"needs-session":  makeSession("needs-session", "project-parent", runStatusNeedsYou, parentPath, user.ID),
		"ownerless":      makeSession("ownerless", "project-nested", "working", nestedPath, ""),
	}
	d.runs = map[string]*runState{
		"running-nested": {ID: "running-nested", Repo: filepath.Join(nestedPath, "repo"), Status: runStatusRunning, Owner: user.ID},
		"needs-parent":   {ID: "needs-parent", Repo: filepath.Join(parentPath, "repo"), Status: runStatusNeedsYou, Owner: user.ID},
		"paused-nested":  {ID: "paused-nested", Repo: filepath.Join(nestedPath, "repo"), Status: runStatusPaused, Owner: user.ID},
		"done-nested":    {ID: "done-nested", Repo: filepath.Join(nestedPath, "repo"), Status: runStatusDone, Owner: user.ID},
		"ownerless-run":  {ID: "ownerless-run", Repo: filepath.Join(nestedPath, "repo"), Status: runStatusRunning},
		"unscoped-run":   {ID: "unscoped-run", Repo: filepath.Join(tempDir(t), "outside"), Status: runStatusRunning, Owner: user.ID},
	}
	d.mu.Unlock()

	srv := httptest.NewServer(withCookie(handler, cookie))
	defer srv.Close()
	resp, err := srv.Client().Get(srv.URL + "/api/dashboard")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/dashboard = %d, want 200", resp.StatusCode)
	}
	var out dashboardResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Groups) != 3 {
		t.Fatalf("Dashboard groups = %#v, want nested, parent and none scopes", out.Groups)
	}

	assertDashboardGroup := func(projectID string, sessionIDs, runIDs []string) {
		t.Helper()
		group, ok := out.Groups[projectID]
		if !ok {
			t.Fatalf("Dashboard has no group %q: %#v", projectID, out.Groups)
		}
		gotSessions := make([]string, 0, len(group.Sessions))
		for _, s := range group.Sessions {
			gotSessions = append(gotSessions, s.ID)
		}
		gotRuns := make([]string, 0, len(group.Runs))
		for _, run := range group.Runs {
			gotRuns = append(gotRuns, run.ID)
		}
		if fmt.Sprint(gotSessions) != fmt.Sprint(sessionIDs) {
			t.Errorf("group %q Sessions = %v, want %v", projectID, gotSessions, sessionIDs)
		}
		if fmt.Sprint(gotRuns) != fmt.Sprint(runIDs) {
			t.Errorf("group %q Runs = %v, want %v", projectID, gotRuns, runIDs)
		}
	}
	assertDashboardGroup("project-nested", []string{"working-nested"}, []string{"running-nested"})
	assertDashboardGroup("project-parent", []string{"needs-session", "failed-parent"}, []string{"needs-parent"})
	assertDashboardGroup(dashboardNoneProjectID, []string{"home-session"}, []string{"unscoped-run"})
}
