package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	el "example.com/emberloop-web/el"
)

func TestRunTicketTypeOverrideRoundTripAndClear(t *testing.T) {
	repo, specPath := initRunRepo(t, map[string]string{
		"01-ticket.md": "# 01: Ticket\n\n**Blocked by:** None\n\n**Type:** code\n",
	})
	ticketPath := filepath.Join(repo, ".scratch", "demo", "issues", "01-ticket.md")
	file := "# 01: Ticket\n\n**Blocked by:** None\n\n**Type:** code\n"
	specRun, err := el.ResolveSpec(specPath)
	if err != nil {
		t.Fatal(err)
	}
	baseFiles, err := el.ReadDriverFiles(specRun)
	if err != nil {
		t.Fatal(err)
	}
	run := &runState{
		ID: "type-run", Status: runStatusPaused, Repo: repo, Spec: specPath,
		Tickets:       []staticTicket{{ID: "01-ticket", Type: "code", Path: ticketPath}},
		TypeOverrides: map[string]string{"01-ticket": "design"},
	}
	stateDir := tempDir(t)
	runsDir := filepath.Join(stateDir, "enderbloom", "runs")
	if err := os.MkdirAll(runsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runsDir, run.ID+".json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	d := runTestDaemon(t, stateDir, NewFakeBackend())
	restored := d.runs[run.ID]
	if restored == nil {
		t.Fatal("Run was not restored")
	}
	files := applyRunTypeOverrides(restored, baseFiles)
	result, err := el.RunDriver(specRun, el.DriverWorld{Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Order) != 1 || result.Order[0].Type != "design" {
		t.Fatalf("driver order after restart = %+v, want design Ticket", result.Order)
	}
	if got := files[ticketPath]; got != "# 01: Ticket\n\n**Blocked by:** None\n\n**Type:** design\n" {
		t.Fatalf("driver file = %q, want overridden type", got)
	}
	if file != "# 01: Ticket\n\n**Blocked by:** None\n\n**Type:** code\n" {
		t.Fatalf("source ticket file was mutated: %q", file)
	}

	delete(restored.TypeOverrides, "01-ticket")
	files = applyRunTypeOverrides(restored, baseFiles)
	result, err = el.RunDriver(specRun, el.DriverWorld{Files: files})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Order) != 1 || result.Order[0].Type != "code" {
		t.Fatalf("driver order after clear = %+v, want code Ticket", result.Order)
	}
	if files[ticketPath] != file {
		t.Fatalf("cleared driver file = %q, want original", files[ticketPath])
	}
}

func TestSetTicketTypeRouteAuthorizationAndWorkerGuard(t *testing.T) {
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	h := d.Handler()
	adminCookie := signedIn(t, h, "admin")
	ownerCookie := signedIn(t, h, "owner")
	otherCookie := signedIn(t, h, "other")
	ownerID := userOf(t, h, ownerCookie).ID
	asAdmin := httptest.NewServer(withCookie(h, adminCookie))
	defer asAdmin.Close()
	asOwner := httptest.NewServer(withCookie(h, ownerCookie))
	defer asOwner.Close()
	asOther := httptest.NewServer(withCookie(h, otherCookie))
	defer asOther.Close()

	run := &runState{
		ID: "type-run", Status: runStatusPaused, Owner: ownerID,
		Tickets:  []staticTicket{{ID: "01-ticket", Type: "code"}},
		Sessions: map[string]string{}, notify: make(chan struct{}),
	}
	d.mu.Lock()
	d.runs[run.ID] = run
	d.mu.Unlock()
	urlPath := "/api/runs/type-run/tickets/01-ticket/type"
	if got := statusOf(t, http.MethodPut, asOwner.URL+urlPath, `{"type":"invalid"}`); got != http.StatusBadRequest {
		t.Fatalf("invalid type status = %d, want 400", got)
	}
	if got := statusOf(t, http.MethodPut, asOwner.URL+urlPath, `{"type":"design"}`); got != http.StatusOK {
		t.Fatalf("owner update status = %d, want 200", got)
	}
	if run.TypeOverrides["01-ticket"] != "design" {
		t.Fatalf("owner override = %q, want design", run.TypeOverrides["01-ticket"])
	}
	if got := statusOf(t, http.MethodPut, asOther.URL+urlPath, `{"type":"code"}`); got != http.StatusForbidden {
		t.Fatalf("non-owner update status = %d, want 403", got)
	}
	if run.TypeOverrides["01-ticket"] != "design" {
		t.Fatalf("unauthorized request changed override to %q", run.TypeOverrides["01-ticket"])
	}
	if got := statusOf(t, http.MethodPut, asAdmin.URL+urlPath, `{"type":"code"}`); got != http.StatusOK {
		t.Fatalf("admin update status = %d, want 200", got)
	}

	// runStartWorker sets this under d.mu before capturing the effective type.
	d.mu.Lock()
	run.startingWorkers = map[string]bool{"01-ticket": true}
	d.mu.Unlock()
	if got := statusOf(t, http.MethodPut, asOwner.URL+urlPath, `{"type":"design"}`); got != http.StatusConflict {
		t.Fatalf("in-progress start status = %d, want 409", got)
	}
	if run.TypeOverrides["01-ticket"] != "code" {
		t.Fatalf("in-progress start changed override to %q", run.TypeOverrides["01-ticket"])
	}
	d.mu.Lock()
	delete(run.startingWorkers, "01-ticket")
	run.Sessions["01-ticket"] = "worker"
	d.mu.Unlock()
	if got := statusOf(t, http.MethodPut, asOwner.URL+urlPath, `{"type":"design"}`); got != http.StatusConflict {
		t.Fatalf("started Ticket status = %d, want 409", got)
	}
	d.mu.Lock()
	delete(run.Sessions, "01-ticket")
	d.mu.Unlock()
	if got := statusOf(t, http.MethodPut, asOwner.URL+urlPath, `{"type":""}`); got != http.StatusOK {
		t.Fatalf("clear type status = %d, want 200", got)
	}
	if _, ok := run.TypeOverrides["01-ticket"]; ok {
		t.Fatal("clear left the type override persisted")
	}
}
