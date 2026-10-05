package main

import (
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"example.com/wick"
	"example.com/wick/tools"
	"example.com/wick/tools/systemone"
)

// settingsUsage is printed on an unknown subcommand and on a bad argument.
const settingsUsage = `usage: wick-pig settings [decision-model <id>|threshold <n>]
       wick-pig settings models [--refresh] [--harness h]
       wick-pig settings role <slot> harness|model|effort <value>`

// defaultThreshold is find's passage cutoff when nothing is stored.
const defaultThreshold = 0.20

// defaultHarness is the harness a slot with nothing stored launches with; the
// resolver's built-in default is the single source for it.
const defaultHarness = wick.DefaultHarness

// modelSource lists the models one harness offers. refresh rebuilds the list
// from the harness instead of reading the cache. A function value, so the
// command's tests need no agent CLI.
type modelSource func(harness string, refresh bool) ([]wick.Model, error)

// runSettings serves the settings subcommand. argv is empty to print what is in
// force; models is the harness listing, injected so tests run no CLI.
func runSettings(argv []string, out io.Writer, models modelSource) error {
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
		warn, err := setRole(&s, argv[1:], models)
		if err != nil {
			return err
		}
		fmt.Fprint(out, warn)
	case "models":
		return listHarnessModels(argv[1:], out, models)
	default:
		return fmt.Errorf("unknown settings command %q\n%s", argv[0], settingsUsage)
	}
	if err := tools.SaveSettings(s); err != nil {
		return err
	}
	printSettings(out, s)
	return nil
}

// listHarnessModels prints `settings models`: every harness's models, or one
// with --harness, rebuilt with --refresh. A harness that cannot be listed is a
// line saying so, so one missing CLI does not hide the others.
func listHarnessModels(argv []string, out io.Writer, models modelSource) error {
	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	refresh := fs.Bool("refresh", false, "rebuild each list instead of using the cache")
	harness := fs.String("harness", "", "list one harness")
	if err := fs.Parse(argv); err != nil {
		return fmt.Errorf("%v\n%s", err, settingsUsage)
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("models takes no arguments\n%s", settingsUsage)
	}
	names := wick.HarnessNames()
	if *harness != "" {
		if wick.Registered(*harness) == nil {
			return fmt.Errorf("unknown harness %q (have %s)\n%s", *harness, strings.Join(names, ", "), settingsUsage)
		}
		names = []string{*harness}
	}
	for _, name := range names {
		list, err := models(name, *refresh)
		if err != nil {
			fmt.Fprintf(out, "%s: unavailable (%v)\n", name, err)
			continue
		}
		printModels(out, name, list)
	}
	return nil
}

// printModels shows a harness's models, the effort levels each takes beside it.
func printModels(out io.Writer, harness string, list []wick.Model) {
	fmt.Fprintf(out, "%s:\n", harness)
	if len(list) == 0 {
		fmt.Fprintln(out, "  (no models)")
		return
	}
	for _, m := range list {
		fmt.Fprintf(out, "  %s", m.ID)
		if m.Label != "" && m.Label != m.ID {
			fmt.Fprintf(out, "  %s", m.Label)
		}
		if len(m.Efforts) > 0 {
			fmt.Fprintf(out, "  (%s)", strings.Join(m.Efforts, " "))
		}
		fmt.Fprintln(out)
	}
}

// setRole applies `role <slot> harness|model|effort <value>` to s. A harness
// change keeps the model and effort the new harness still offers and clears the
// ones it does not, with a notice. A saved value the harness cannot be asked
// about is kept, with a warning: an unlistable harness must not block editing.
// The warning is the caller's to print.
func setRole(s *tools.Settings, argv []string, models modelSource) (string, error) {
	if len(argv) != 3 {
		return "", fmt.Errorf("role needs a slot, a field and a value\n%s", settingsUsage)
	}
	slot, field, value := argv[0], argv[1], argv[2]
	if !tools.IsRoleSlot(slot) {
		return "", fmt.Errorf("unknown role slot %q (have %s)\n%s", slot, strings.Join(tools.RoleSlots, ", "), settingsUsage)
	}
	if s.Roles == nil {
		s.Roles = map[string]tools.Role{}
	}
	role := s.Roles[slot]
	var warn string
	var err error
	switch field {
	case "harness":
		if wick.Registered(value) == nil {
			return "", fmt.Errorf("unknown harness %q (have %s)\n%s", value, strings.Join(wick.HarnessNames(), ", "), settingsUsage)
		}
		// Compare against the harness in force: an unset slot already means the
		// built-in default, so naming that default is not a change.
		if value == orDefault(role.Harness, defaultHarness) {
			role.Harness = value
			break
		}
		role, warn = carryOver(models, slot, value, role)
	case "model":
		harness := orDefault(role.Harness, defaultHarness)
		if warn, err = checkModel(models, harness, value); err != nil {
			return "", err
		}
		role.Model = value
		warn += dropEffort(models, harness, &role)
	case "effort":
		harness := orDefault(role.Harness, defaultHarness)
		if warn, err = checkEffort(models, harness, role.Model, value); err != nil {
			return "", err
		}
		role.Effort = value
	default:
		return "", fmt.Errorf("unknown role field %q (have harness, model, effort)\n%s", field, settingsUsage)
	}
	s.Roles[slot] = role
	return warn, nil
}

// modelsFor returns a harness's models. want names something the caller looks
// for; when the cached list does not hold it, the list is rebuilt once before
// giving up. trusted is false when no list could be built at all -- that is no
// verdict, so the caller allows the value.
func modelsFor(src modelSource, harness, want string) (list []wick.Model, trusted bool) {
	list, err := src(harness, false)
	if err != nil || len(list) == 0 {
		return nil, false
	}
	if _, ok := hasModel(list, want); want == "" || ok {
		return list, true
	}
	if fresh, err := src(harness, true); err == nil && len(fresh) > 0 {
		return fresh, true
	}
	return list, true
}

// hasModel finds one model by id.
func hasModel(list []wick.Model, id string) (wick.Model, bool) {
	for _, m := range list {
		if m.ID == id {
			return m, true
		}
	}
	return wick.Model{}, false
}

// anyOffers reports whether any model of a harness takes an effort, the check
// for an effort saved with no model beside it.
func anyOffers(list []wick.Model, effort string) bool {
	for _, m := range list {
		if contains(m.Efforts, effort) {
			return true
		}
	}
	return false
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// checkModel rejects a model the harness does not offer, after one refresh, and
// warns instead when the harness's list cannot be built.
func checkModel(models modelSource, harness, id string) (string, error) {
	list, trusted := modelsFor(models, harness, id)
	if !trusted {
		return fmt.Sprintf("%s: cannot list models; model %q saved unchecked\n", harness, id), nil
	}
	if _, ok := hasModel(list, id); !ok {
		return "", fmt.Errorf("%s does not offer model %q; see `wick-pig settings models --harness %s`\n%s",
			harness, id, harness, settingsUsage)
	}
	return "", nil
}

// checkEffort rejects an effort the chosen model does not take. With no model
// chosen the effort is checked against every model of the harness; a model the
// list does not know is no verdict.
func checkEffort(models modelSource, harness, model, effort string) (string, error) {
	list, trusted := modelsFor(models, harness, model)
	if !trusted {
		return fmt.Sprintf("%s: cannot list models; effort %q saved unchecked\n", harness, effort), nil
	}
	m, known := hasModel(list, model)
	// The effort may be newer than the cache: rebuild once before rejecting. A
	// model the cache does not know was already looked for with one rebuild, so
	// only a known model missing the effort, or no model at all, rebuilds here.
	if (known && !contains(m.Efforts, effort)) || (!known && model == "") {
		if fresh, err := models(harness, true); err == nil && len(fresh) > 0 {
			list = fresh
			m, known = hasModel(list, model)
		}
	}
	switch {
	case known && len(m.Efforts) == 0:
		return "", fmt.Errorf("model %q on %s takes no effort\n%s", model, harness, settingsUsage)
	case known && !contains(m.Efforts, effort):
		return "", fmt.Errorf("model %q on %s does not take effort %q (have %s)\n%s",
			model, harness, effort, strings.Join(m.Efforts, ", "), settingsUsage)
	case !known && model == "" && !anyOffers(list, effort):
		return "", fmt.Errorf("%s takes no effort %q; see `wick-pig settings models --harness %s`\n%s",
			harness, effort, harness, settingsUsage)
	}
	return "", nil
}

// dropEffort clears an effort a newly saved model does not take, so a model
// change cannot leave a pair the harness would refuse. A list that cannot be
// built, or a model it does not know, is no verdict and clears nothing.
func dropEffort(models modelSource, harness string, role *tools.Role) string {
	if role.Effort == "" {
		return ""
	}
	list, trusted := modelsFor(models, harness, role.Model)
	if !trusted {
		return ""
	}
	m, known := hasModel(list, role.Model)
	if !known || contains(m.Efforts, role.Effort) {
		return ""
	}
	effort := role.Effort
	role.Effort = ""
	return fmt.Sprintf("model %q on %s does not take effort %q; cleared\n", role.Model, harness, effort)
}

// carryOver is the model and effort a slot keeps when its harness changes: the
// ones the new harness offers, cleared otherwise. A list that cannot be built
// keeps both and says so.
func carryOver(models modelSource, slot, harness string, role tools.Role) (tools.Role, string) {
	out := tools.Role{Harness: harness}
	if role.Model == "" && role.Effort == "" {
		return out, ""
	}
	list, trusted := modelsFor(models, harness, role.Model)
	if !trusted {
		out.Model, out.Effort = role.Model, role.Effort
		return out, fmt.Sprintf("%s: cannot list %s models; kept model %q, effort %q\n",
			slot, harness, role.Model, role.Effort)
	}
	m, found := hasModel(list, role.Model)
	if found {
		out.Model = role.Model
	}
	switch {
	case role.Effort == "":
	case found && contains(m.Efforts, role.Effort):
		out.Effort = role.Effort
	case !found && role.Model == "" && anyOffers(list, role.Effort):
		out.Effort = role.Effort
	}
	var dropped []string
	if role.Model != "" && !found {
		dropped = append(dropped, fmt.Sprintf("model %q", role.Model))
	}
	if role.Effort != "" && out.Effort == "" {
		dropped = append(dropped, fmt.Sprintf("effort %q", role.Effort))
	}
	if len(dropped) == 0 {
		return out, ""
	}
	return out, fmt.Sprintf("%s: %s does not offer %s; cleared\n", slot, harness, strings.Join(dropped, " and "))
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

// cachedModels is the live model source for the settings command: each
// harness's list, read from the state dir's cache and rebuilt on --refresh.
func cachedModels(harness string, refresh bool) ([]wick.Model, error) {
	return wick.CachedModels(harness, wick.OSExec, refresh)
}
