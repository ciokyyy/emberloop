package main

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"example.com/wick/tools"
	"example.com/wick/tools/systemone"
)

// settingsUsage is printed on an unknown subcommand and on a bad argument.
const settingsUsage = `usage: wick-pig settings [decision-model <id>|threshold <n>|models]`

// defaultThreshold is find's passage cutoff when nothing is stored.
const defaultThreshold = 0.20

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

// printSettings shows the stored choices, or the default in force when one is
// unset. The env overrides already won inside LoadSettings.
func printSettings(out io.Writer, s tools.Settings) {
	model := s.DecisionModel
	if model == "" {
		model = fmt.Sprintf("(%s, the default)", systemone.JevModelPinned)
	}
	threshold := s.Threshold
	if threshold == 0 {
		threshold = defaultThreshold
	}
	fmt.Fprintf(out, "settings: %s\n", tools.SettingsPath())
	fmt.Fprintf(out, "decision-model: %s\n", model)
	fmt.Fprintf(out, "threshold: %g\n", threshold)
}

// listModels is the live id source for `settings models`.
func listModels() ([]string, error) {
	return systemone.ListModels(context.Background())
}
