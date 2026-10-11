package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// SudoGate, when set, sees every bash command that invokes sudo before it
// runs. It returns the tool result and true to take the command over: the
// command then never runs here. A host that wants a user to approve
// privileged commands installs one; the default is nil and bash is unchanged.
var SudoGate func(ctx context.Context, root, command string) (string, bool)

var sudoWord = regexp.MustCompile(`(^|[;&|(\x60]|\$\(|\s)sudo(\s|$)`)

// UsesSudo reports whether a shell command invokes sudo as a word.
func UsesSudo(command string) bool {
	return sudoWord.MatchString(command)
}

// Environment names that bind a bash process to a gating Daemon.
const (
	SudoGateSessionEnv = "EL_WEB_PARENT"
	SudoGateDaemonEnv  = "EL_WEB_DAEMON_URL"
	SudoGateTokenEnv   = "EL_WEB_TOKEN"
)

// SudoGateFromEnv returns an HTTP gate when the environment names a Daemon
// and a Session, else nil.
func SudoGateFromEnv() func(ctx context.Context, root, command string) (string, bool) {
	url, session := os.Getenv(SudoGateDaemonEnv), os.Getenv(SudoGateSessionEnv)
	if url == "" || session == "" {
		return nil
	}
	return HTTPSudoGate(url, session, os.Getenv(SudoGateTokenEnv))
}

// HTTPSudoGate posts the command to the Daemon at daemonURL as a sudo
// request of one Session, presenting token as the machine token when set. A
// Daemon that cannot be reached refuses the command rather than running it
// unapproved.
func HTTPSudoGate(daemonURL, sessionID, token string) func(ctx context.Context, root, command string) (string, bool) {
	daemonURL = strings.TrimSuffix(daemonURL, "/")
	return func(ctx context.Context, root, command string) (string, bool) {
		body, _ := json.Marshal(map[string]string{"command": command, "cwd": root})
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, daemonURL+"/api/sessions/"+sessionID+"/sudo", bytes.NewReader(body))
		if err != nil {
			return "sudo refused: " + err.Error(), true
		}
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "sudo refused: approval service unreachable: " + err.Error(), true
		}
		defer resp.Body.Close()
		var out struct {
			ID    string `json:"id"`
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		if resp.StatusCode != http.StatusCreated {
			return fmt.Sprintf("sudo refused: approval service answered %d %s", resp.StatusCode, out.Error), true
		}
		return SudoPendingMessage(out.ID), true
	}
}

// SudoPendingMessage is the tool result the model sees once a request is
// queued for the user.
func SudoPendingMessage(id string) string {
	return fmt.Sprintf("Request %s sent to the user: this command needs sudo and was not run. Keep working on anything else. When the user decides, the output arrives as a sudo_result message.", id)
}

// SanitizeOutput strips terminal escapes from command output, as bash does.
func SanitizeOutput(s string) string { return sanitize(s) }
