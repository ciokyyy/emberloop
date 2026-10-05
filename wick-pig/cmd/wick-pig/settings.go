package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"example.com/wick"
	"example.com/wick/tools"
	"example.com/wick/tools/systemone"
)

// settingsUsage is printed on an unknown subcommand and on a bad argument.
const settingsUsage = `usage: wick-pig settings [decision-model <id>|threshold <n>|models]
       wick-pig settings role <slot> harness|model|effort <value>`

// defaultThreshold is find's passage cutoff when nothing is stored.
const defaultThreshold = 0.20

// defaultHarness is the harness a slot with nothing stored launches with; the
// resolver's built-in default is the single source for it.
const defaultHarness = wick.DefaultHarness

// runSettings serves the settings subcommand. argv is empty to print what is in
// force; models is a function value so a test lists ids without a network call.
func runSettings(argv []string, out io.Writer, models func() ([]string, error)) error {
	s := tools.LoadSettings()
	if len(argv) == 0 {
		printSettings(out, s)
		return nil
	}
	switch argv[0] {
	case "decision-model":
		if len(argv) < 2 {
			return fmt.Errorf("decision-model needs an id\n%s", settingsUsage)
		}
		s.DecisionModel = systemone.CanonicalModel(argv[1])
	case "threshold":
		if len(argv) < 2 {
			return fmt.Errorf("threshold needs a number\n%s", settingsUsage)
		}
		v, err := strconv.ParseFloat(argv[1], 64)
		if err != nil || v <= 0 {
			return fmt.Errorf("threshold %q is not a number above zero\n%s", argv[1], settingsUsage)
		}
		s.Threshold = v
	case "role":
		if err := setRole(&s, argv[1:]); err != nil {
			return err
		}
	case "models":
		ids, err := models()
		if err != nil {
			return err
		}
		for _, id := range ids {
			fmt.Fprintln(out, id)
		}
		return nil
	default:
		return fmt.Errorf("unknown settings command %q\n%s", argv[0], settingsUsage)
	}
	if err := tools.SaveSettings(s); err != nil {
		return err
	}
	printSettings(out, s)
	return nil
}

// setRole applies `role <slot> harness|model|effort <value>` to s. A harness
// change clears the model and effort: they were chosen for the old harness and
// ticket 02 is what re-checks whether the new one supports them.
func setRole(s *tools.Settings, argv []string) error {
	if len(argv) != 3 {
		return fmt.Errorf("role needs a slot, a field and a value\n%s", settingsUsage)
	}
	slot, field, value := argv[0], argv[1], argv[2]
	if !tools.IsRoleSlot(slot) {
		return fmt.Errorf("unknown role slot %q (have %s)\n%s", slot, strings.Join(tools.RoleSlots, ", "), settingsUsage)
	}
	if s.Roles == nil {
		s.Roles = map[string]tools.Role{}
	}
	role := s.Roles[slot]
	switch field {
	case "harness":
		if wick.Registered(value) == nil {
			return fmt.Errorf("unknown harness %q (have %s)\n%s", value, strings.Join(wick.HarnessNames(), ", "), settingsUsage)
		}
		// Compare against the harness in force: an unset slot already means the
		// built-in default, so naming that default is not a change.
		if value != orDefault(role.Harness, defaultHarness) {
			role.Model, role.Effort = "", ""
		}
		role.Harness = value
	case "model":
		role.Model = value
	case "effort":
		role.Effort = value
	default:
		return fmt.Errorf("unknown role field %q (have harness, model, effort)\n%s", field, settingsUsage)
	}
	s.Roles[slot] = role
	return nil
}

// printSettings shows the stored choices, or the default in force when one is
// unset, and where each value came from. The env overrides already won inside
// LoadSettings.
func printSettings(out io.Writer, s tools.Settings) {
	fmt.Fprintf(out, "settings: %s\n", tools.SettingsPath())
	fmt.Fprintf(out, "decision-model: %s (%s)\n", orDefault(s.DecisionModel, systemone.JevModelPinned), s.SourceOf("decision_model"))
	fmt.Fprintf(out, "threshold: %g (%s)\n", orDefault(s.Threshold, defaultThreshold), s.SourceOf("threshold"))
	for _, slot := range tools.RoleSlots {
		r := s.Roles[slot]
		fmt.Fprintf(out, "%s: harness %s (%s), model %s (%s), effort %s (%s)\n",
			slot,
			orDefault(r.Harness, defaultHarness), sourceOf(r.Harness),
			orDefault(r.Model, "-"), sourceOf(r.Model),
			orDefault(r.Effort, "-"), sourceOf(r.Effort))
	}
}

// sourceOf is where a role field came from: the file owns any value it holds,
// and an unset field is the built-in default.
func sourceOf(value string) string {
	if value == "" {
		return "default"
	}
	return "file"
}

// orDefault returns value, or the fallback when value is zero.
func orDefault[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}

// listModels is the live id source for `settings models`.
func listModels() ([]string, error) {
	return systemone.ListModels(context.Background())
}
