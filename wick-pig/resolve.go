package wick

import "example.com/wick/tools"

// DefaultHarness is the harness a slot with nothing chosen launches with.
const DefaultHarness = "pig"

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
