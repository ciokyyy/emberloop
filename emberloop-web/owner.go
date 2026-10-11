package web

import "net/http"

// creatorOf returns the signed-in User behind a request that creates a
// Session or Run. The machine token is no User, so it answers 403.
func (d *Daemon) creatorOf(w http.ResponseWriter, r *http.Request) (User, bool) {
	rq, ok := d.requesterOf(r)
	if !ok || rq.user == nil {
		http.Error(w, "only a signed-in User creates Sessions and Runs", http.StatusForbidden)
		return User{}, false
	}
	return *rq.user, true
}

// senderOf types the prompt a request carries: a signed-in User sends a
// "user" prompt with its ID; the machine token (agents, scripts) sends a
// "system" prompt.
func (d *Daemon) senderOf(r *http.Request) promptSender {
	if rq, ok := d.requesterOf(r); ok && rq.user != nil {
		return userSender(*rq.user)
	}
	return systemSender
}

// hiddenSession reports whether id names a Session with no Owner. Such
// Sessions predate owners: lists skip them and every route answers 404.
func (d *Daemon) hiddenSession(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	s, ok := d.sessions[id]
	return ok && s.meta.Owner == ""
}

// hiddenRun is hiddenSession for Runs.
func (d *Daemon) hiddenRun(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	rs, ok := d.runs[id]
	return ok && rs.Owner == ""
}
