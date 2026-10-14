package web

import (
	"net/http"
	"sort"
	"time"
)

const dashboardNoneProjectID = "none"

// dashboardGroup is the active Session and Run payload for one Project scope.
type dashboardGroup struct {
	Sessions []SessionMeta `json:"sessions"`
	Runs     []*runSummary `json:"runs"`
}

type dashboardResponse struct {
	Groups map[string]dashboardGroup `json:"groups"`
}

type dashboardSession struct {
	meta SessionMeta
	last time.Time
}

// handleDashboard returns active work grouped by Project id. Idle Session and
// settled Run history stays on the scoped lists, not the Dashboard.
func (d *Daemon) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	d.mu.Lock()
	sessions := make([]dashboardSession, 0, len(d.sessions))
	for _, s := range d.sessions {
		if s.meta.Owner == "" || !dashboardSessionActive(s.meta) {
			continue
		}
		sessions = append(sessions, dashboardSession{meta: s.meta, last: s.last})
	}
	runs := make([]*runState, 0, len(d.runs))
	for _, rs := range d.runs {
		if rs != nil && rs.Owner != "" && (rs.Status == runStatusRunning || rs.Status == runStatusNeedsYou) {
			runs = append(runs, rs)
		}
	}
	d.mu.Unlock()

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].last.Equal(sessions[j].last) {
			return sessions[i].meta.ID > sessions[j].meta.ID
		}
		return sessions[i].last.After(sessions[j].last)
	})

	d.projMu.Lock()
	projectIDs := make(map[string]struct{}, len(d.projects))
	for _, project := range d.projects {
		projectIDs[project.ID] = struct{}{}
	}
	d.projMu.Unlock()

	groups := make(map[string]dashboardGroup)
	groupFor := func(projectID string) (string, dashboardGroup) {
		_, exists := projectIDs[projectID]
		if projectID == "" || !exists {
			projectID = dashboardNoneProjectID
		}
		group, ok := groups[projectID]
		if !ok {
			group = dashboardGroup{Sessions: []SessionMeta{}, Runs: []*runSummary{}}
		}
		return projectID, group
	}
	for _, s := range sessions {
		projectID, group := groupFor(s.meta.ProjectID)
		group.Sessions = append(group.Sessions, s.meta)
		groups[projectID] = group
	}
	for _, rs := range runs {
		summary := d.runSummary(rs)
		if summary == nil || (summary.Status != runStatusRunning && summary.Status != runStatusNeedsYou) || summary.Owner == "" {
			continue
		}
		projectID, group := groupFor(summary.ProjectID)
		group.Runs = append(group.Runs, summary)
		groups[projectID] = group
	}
	for projectID, group := range groups {
		sort.Slice(group.Runs, func(i, j int) bool { return group.Runs[i].ID < group.Runs[j].ID })
		groups[projectID] = group
	}
	writeJSON(w, http.StatusOK, dashboardResponse{Groups: groups})
}

// dashboardSessionActive includes only a working Session or one explicitly
// marked as needing the User's attention.
func dashboardSessionActive(meta SessionMeta) bool {
	return meta.Status == "working" || meta.Status == runStatusNeedsYou
}
