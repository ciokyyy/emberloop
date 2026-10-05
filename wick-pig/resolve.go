package wick

import (
	"os"

	"example.com/wick/tools"
)

// DefaultHarness is the harness a slot with nothing chosen launches with.
const DefaultHarness = "pig"

// The settings slots wick resolves at a launch site, named as tools.RoleSlots
// spells them. The orchestrator slot is resolved in the launcher's own package.
const (
	slotImplementer = "implementer"
	slotCodeReview  = "code-review"
	slotSpecReview  = "spec-review"
	slotScout       = "scout"
)

// Choice is a harness, model and effort at one precedence layer: the CLI flags,
// an agent type's frontmatter or a spawn entry, or the settings file. An empty
// field means "nothing at this layer".
type Choice struct {
	Harness string
	Model   string
	Effort  string
}

// Resolve gives the harness, model and effort for a launch slot, per field,
// with precedence flags > typeOrSpawn > the settings file > the built-in
// default. A slot with nothing stored falls back to the default, so an empty
// or missing settings file still launches.
func Resolve(slot string, flags, typeOrSpawn Choice) Choice {
	role := tools.LoadSettings().Roles[slot]
	return Choice{
		Harness: firstSet(flags.Harness, typeOrSpawn.Harness, role.Harness, DefaultHarness),
		Model:   firstSet(flags.Model, typeOrSpawn.Model, role.Model),
		Effort:  firstSet(flags.Effort, typeOrSpawn.Effort, role.Effort),
	}
}

// firstSet is the first non-empty value, or "" when every layer is empty.
func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// readonlyHarnessChoice is WICK_READONLY_HARNESS as the resolver's flag layer.
// It is kept as a high-precedence override for compatibility, so it wins over
// whatever slot or agent type the launch would otherwise use.
func readonlyHarnessChoice() Choice {
	if h := os.Getenv("WICK_READONLY_HARNESS"); h != "" {
		return Choice{Harness: h}
	}
	return Choice{}
}
