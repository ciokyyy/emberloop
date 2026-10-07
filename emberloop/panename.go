package emberloop

import (
	"os"
	"time"
)

// OrchestratorPaneName is the herdr pane name of the user's top-level
// session. The header strip ticket will read its name chip from the same
// mapping below.
const OrchestratorPaneName = "orchestrator"

// PaneNameForType maps an agent type to its herdr pane name. Built-in types
// map to the readable names the spec uses; any other type keeps its own
// name. It is the one mapping herdr renames go through, so the header
// strip ticket can read the same value for its name chip.
func PaneNameForType(typeName string) string {
	switch typeName {
	case "worker":
		return "implementer"
	case "reviewer-spec":
		return "spec-reviewer"
	case "reviewer-standards":
		return "code-reviewer"
	case "scout":
		return "scout"
	case "orchestrator":
		return OrchestratorPaneName
	default:
		return typeName
	}
}

// AgentPaneName is the pane name of an agent of type t.
func AgentPaneName(t AgentType) string { return PaneNameForType(t.Name) }

// placedPaneName is the pane name of a session launchPlacedAgent placed: the
// session's agent type, or scout for a one-shot read-only run with no type.
// The session itself is untouched, so the launch env carries no EMBERLOOP_AGENT_TYPE
// it did not already have.
func placedPaneName(s Session) string {
	if s.AgentType == "" {
		return "scout"
	}
	return PaneNameForType(s.AgentType)
}

// renamePane names pane in herdr. A rename that fails is silent: a name is
// cosmetic, and it must never block or fail a spawn.
func renamePane(run Exec, pane, name string) {
	if run == nil || pane == "" || name == "" {
		return
	}
	_, _ = run(herdrBin(), []string{"pane", "rename", pane, name}, ExecOpts{Timeout: 15 * time.Second})
}

// NameOrchestratorPane names the top-level session's own pane orchestrator.
// Outside herdr there is no pane, so it does nothing.
func NameOrchestratorPane(run Exec) {
	pane := os.Getenv("HERDR_PANE_ID")
	if pane == "" {
		return
	}
	renamePane(run, pane, OrchestratorPaneName)
}

// defaultWorkerType names the type of a worker SpawnInput that carries
// none: direct Spawn callers predate AgentType, and every one is a worker.
func defaultWorkerType(t string) string {
	if t == "" {
		return "worker"
	}
	return t
}

// TopLevelChipName is the strip chip of a top-level session. A typed agent
// keeps its type's name. An untyped session is the orchestrator only when it
// runs the implement role, which is how the orchestrator launches; any other
// role (a bare `emberloop claude` plans, `--role full` has every tool) shows
// its own role name, so the chip never claims a job the session does not do.
func TopLevelChipName(agentType, role string) string {
	if agentType != "" {
		return PaneNameForType(agentType)
	}
	if role == "" || role == "implement" {
		return OrchestratorPaneName
	}
	return role
}
