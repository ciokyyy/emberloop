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

// Resolve gives the harness, model and effort for a launch slot. The harness
// resolves first, with precedence flags > typeOrSpawn > the settings file >
// the built-in default. Model and effort then come only from layers whose
// harness matches the resolved one; a layer with no harness inherits it and
// still counts. So a layer that changes the harness drops a lower layer's
// model and effort for another harness, and the harness's own defaults apply.
// A slot with nothing stored falls back to the default, so an empty or
// missing settings file still launches.
func Resolve(slot string, flags, typeOrSpawn Choice) Choice {
	role := tools.LoadSettings().Roles[slot]
	file := Choice{Harness: role.Harness, Model: role.Model, Effort: role.Effort}
	harness := firstSet(flags.Harness, typeOrSpawn.Harness, file.Harness, DefaultHarness)
	var model, effort string
	for _, layer := range []Choice{flags, typeOrSpawn, file} {
		if layer.Harness != "" && layer.Harness != harness {
			continue
		}
		if model == "" {
			model = layer.Model
		}
		if effort == "" {
			effort = layer.Effort
		}
		if model != "" && effort != "" {
			break
		}
	}
	return Choice{Harness: harness, Model: model, Effort: effort}
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
