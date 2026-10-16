package web

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRunTicketTypeOverrideRoundTripAndClear(t *testing.T) {
	file := "# Ticket\n\n**Type:** code\n"
	run := &runState{
		ID: "type-run", Status: runStatusPaused,
		Tickets:       []staticTicket{{ID: "01-ticket", Type: "code", Path: "ticket.md"}},
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
	if got := effectiveRunTicketType(restored, restored.Tickets[0]); got != "design" {
		t.Fatalf("effective type after restart = %q, want design", got)
	}
	files := applyRunTypeOverrides(restored, map[string]string{"ticket.md": file})
	if got := files["ticket.md"]; got != "# Ticket\n\n**Type:** design\n" {
		t.Fatalf("driver file = %q, want overridden type", got)
	}
	if file != "# Ticket\n\n**Type:** code\n" {
		t.Fatalf("source ticket file was mutated: %q", file)
	}

	delete(restored.TypeOverrides, "01-ticket")
	if got := effectiveRunTicketType(restored, restored.Tickets[0]); got != "code" {
		t.Fatalf("effective type after clear = %q, want code", got)
	}
	files = applyRunTypeOverrides(restored, map[string]string{"ticket.md": file})
	if files["ticket.md"] != file {
		t.Fatalf("cleared driver file = %q, want original", files["ticket.md"])
	}
}

func TestSetTicketTypeRouteValidationAndWorkerGuard(t *testing.T) {
	d := runTestDaemon(t, tempDir(t), NewFakeBackend())
	run := &runState{
		ID: "type-run", Status: runStatusPaused, Owner: "owner",
		Tickets:  []staticTicket{{ID: "01-ticket", Type: "code"}},
		Sessions: map[string]string{}, notify: make(chan struct{}),
	}
	d.runs[run.ID] = run

	call := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("PUT", "/api/runs/type-run/tickets/01-ticket/type", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		d.handleRunSub(w, req)
		return w
	}
	if got := call(`{"type":"invalid"}`).Code; got != 400 {
		t.Fatalf("invalid type status = %d, want 400", got)
	}
	if got := call(`{"type":"design"}`).Code; got != 200 {
		t.Fatalf("set type status = %d, want 200", got)
	}
	if run.TypeOverrides["01-ticket"] != "design" {
		t.Fatalf("override = %q, want design", run.TypeOverrides["01-ticket"])
	}
	run.Sessions["01-ticket"] = "worker"
	if got := call(`{"type":"code"}`).Code; got != 409 {
		t.Fatalf("started Ticket status = %d, want 409", got)
	}
	delete(run.Sessions, "01-ticket")
	if got := call(`{"type":""}`).Code; got != 200 {
		t.Fatalf("clear type status = %d, want 200", got)
	}
	if _, ok := run.TypeOverrides["01-ticket"]; ok {
		t.Fatal("clear left the type override persisted")
	}
}
