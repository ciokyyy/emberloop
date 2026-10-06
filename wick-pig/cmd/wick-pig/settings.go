package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"

	"example.com/wick"
	"example.com/wick/tools"
	"example.com/wick/tools/systemone"
)

// settingsUsage is printed on an unknown subcommand and on a bad argument.
const settingsUsage = `usage: wick-pig settings [decision-model <id>|threshold <n>|max-workers <n>]
       wick-pig settings models [--refresh] [--harness h]
       wick-pig settings role <slot> harness|model|effort <value>
       wick-pig settings tool <name> enabled|placement|rename <value>
       wick-pig settings tool <name> role <role> enabled|placement <value>
       wick-pig settings mcp-server <name> command|args|placement|include|exclude <value>
       wick-pig settings mcp-server <name> env <KEY=VALUE>
       wick-pig settings mcp-server <name> rename <ORIG=NEW>
       wick-pig settings mcp-server <name> role <role> enabled|placement <value>
       wick-pig settings mcp-server remove <name>`

// defaultThreshold is find's passage cutoff when nothing is stored, shared
// with the dashboard's Settings tab.
const defaultThreshold = tools.DefaultThreshold

// defaultHarness is the harness a slot with nothing stored launches with; the
// resolver's built-in default is the single source for it.
const defaultHarness = wick.DefaultHarness

// modelSource lists the models one harness offers. refresh rebuilds the list
// from the harness instead of reading the cache. A function value, so the
// command's tests need no agent CLI.
type modelSource func(harness string, refresh bool) ([]wick.Model, error)

// runSettings serves the settings subcommand. argv is empty to print what is in
// force; models is the harness listing, injected so tests run no CLI. Edits
// write the global file; the printout shows the effective settings, global
// folded with the project file where the command runs.
func runSettings(argv []string, out io.Writer, models modelSource) error {
	if len(argv) == 0 {
		printSettings(out, tools.EffectiveSettings(projectRoot()))
		return nil
	}
	s := tools.LoadSettings()
	var note string
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
	case "max-workers":
		if len(argv) < 2 {
			return fmt.Errorf("max-workers needs a number\n%s", settingsUsage)
		}
		v, err := strconv.Atoi(argv[1])
		if err != nil || v <= 0 {
			return fmt.Errorf("max-workers %q is not a whole number above zero\n%s", argv[1], settingsUsage)
		}
		s.MaxWorkers = v
	case "role":
		warn, err := setRole(&s, argv[1:], models)
		if err != nil {
			return err
		}
		note = warn
	case "tool":
		n, err := setTool(&s, argv[1:])
		if err != nil {
			return err
		}
		note = n
	case "mcp-server":
		n, err := setMCPServer(&s, argv[1:])
		if err != nil {
			return err
		}
		note = n
	case "models":
		return listHarnessModels(argv[1:], out, models)
	default:
		return fmt.Errorf("unknown settings command %q\n%s", argv[0], settingsUsage)
	}
	if err := tools.SaveSettings(s); err != nil {
		return err
	}
	fmt.Fprint(out, note)
	printSettings(out, tools.EffectiveSettings(projectRoot()))
	return nil
}

// projectRoot is the directory the project settings file is read from: where
// the command runs.
func projectRoot() string {
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return "."
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

// setTool applies `tool <name> enabled|placement|rename <value>` or
// `tool <name> role <role> enabled|placement <value>` to s. Any non-empty
// name is accepted: the CLI cannot know every MCP-provided name, and an
// unknown one warns at serve time. A rename of "-" clears the rename.
func setTool(s *tools.Settings, argv []string) (string, error) {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return "", fmt.Errorf("tool needs a name\n%s", settingsUsage)
	}
	name := argv[0]
	if len(argv) > 1 && argv[1] == "role" {
		if len(argv) != 5 {
			return "", fmt.Errorf("tool role needs a name, a role, a field and a value\n%s", settingsUsage)
		}
		role, field, value := argv[2], argv[3], argv[4]
		if err := checkToolRole(role); err != nil {
			return "", err
		}
		e := toolEntry(s, name)
		if e.Roles == nil {
			e.Roles = map[string]tools.ToolRoleOverride{}
		}
		ov := e.Roles[role]
		switch field {
		case "enabled":
			v, err := parseEnabled(value)
			if err != nil {
				return "", err
			}
			ov.Enabled = &v
		case "placement":
			if err := checkPlacement(value); err != nil {
				return "", err
			}
			ov.Placement = value
		default:
			return "", fmt.Errorf("unknown tool role field %q (have enabled, placement)\n%s", field, settingsUsage)
		}
		e.Roles[role] = ov
		s.Tools[name] = e
		return "", nil
	}
	if len(argv) != 3 {
		return "", fmt.Errorf("tool needs a name, a field and a value\n%s", settingsUsage)
	}
	field, value := argv[1], argv[2]
	e := toolEntry(s, name)
	switch field {
	case "enabled":
		v, err := parseEnabled(value)
		if err != nil {
			return "", err
		}
		e.Enabled = &v
	case "placement":
		if err := checkPlacement(value); err != nil {
			return "", err
		}
		e.Placement = value
	case "rename":
		if value == "-" {
			value = ""
		}
		e.Rename = value
	default:
		return "", fmt.Errorf("unknown tool field %q (have enabled, placement, rename)\n%s", field, settingsUsage)
	}
	s.Tools[name] = e
	return "", nil
}

// toolEntry returns the named tool entry, creating the map when needed.
func toolEntry(s *tools.Settings, name string) tools.ToolSetting {
	if s.Tools == nil {
		s.Tools = map[string]tools.ToolSetting{}
	}
	return s.Tools[name]
}

// setMCPServer applies `mcp-server ...` to s: field edits on a named server,
// or `mcp-server remove <name>`. Lists (args, include, exclude) are comma
// separated and replace; an empty value clears them. env takes one KEY=VALUE
// pair (an empty value deletes the key); rename takes one ORIG=NEW pair (an
// empty NEW deletes the mapping).
func setMCPServer(s *tools.Settings, argv []string) (string, error) {
	if len(argv) == 0 {
		return "", fmt.Errorf("mcp-server needs a name\n%s", settingsUsage)
	}
	if argv[0] == "remove" {
		if len(argv) != 2 {
			return "", fmt.Errorf("mcp-server remove needs a name\n%s", settingsUsage)
		}
		if _, ok := s.MCPServers[argv[1]]; !ok {
			return "", fmt.Errorf("unknown mcp server %q\n%s", argv[1], settingsUsage)
		}
		s.RemoveMCPServer(argv[1])
		return "", nil
	}
	name := argv[0]
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("mcp-server needs a name\n%s", settingsUsage)
	}
	if len(argv) > 1 && argv[1] == "role" {
		if len(argv) != 5 {
			return "", fmt.Errorf("mcp-server role needs a name, a role, a field and a value\n%s", settingsUsage)
		}
		role, field, value := argv[2], argv[3], argv[4]
		if err := checkToolRole(role); err != nil {
			return "", err
		}
		e := serverEntry(s, name)
		if e.Roles == nil {
			e.Roles = map[string]tools.MCPServerRoleOverride{}
		}
		ov := e.Roles[role]
		switch field {
		case "enabled":
			v, err := parseEnabled(value)
			if err != nil {
				return "", err
			}
			ov.Enabled = &v
		case "placement":
			if err := checkPlacement(value); err != nil {
				return "", err
			}
			ov.Placement = value
		default:
			return "", fmt.Errorf("unknown mcp-server role field %q (have enabled, placement)\n%s", field, settingsUsage)
		}
		e.Roles[role] = ov
		s.MCPServers[name] = e
		return "", nil
	}
	if len(argv) != 3 {
		return "", fmt.Errorf("mcp-server needs a name, a field and a value\n%s", settingsUsage)
	}
	field, value := argv[1], argv[2]
	e := serverEntry(s, name)
	switch field {
	case "command":
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("command needs a command\n%s", settingsUsage)
		}
		e.Command = value
	case "args":
		e.Args = splitList(value)
	case "env":
		k, v, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return "", fmt.Errorf("env needs KEY=VALUE\n%s", settingsUsage)
		}
		if e.Env == nil {
			e.Env = map[string]string{}
		}
		if v == "" {
			delete(e.Env, k)
		} else {
			e.Env[k] = v
		}
	case "placement":
		if err := checkPlacement(value); err != nil {
			return "", err
		}
		e.Placement = value
	case "include", "exclude":
		globs := splitList(value)
		for _, g := range globs {
			if _, err := path.Match(g, ""); err != nil {
				return "", fmt.Errorf("invalid glob %q: %v\n%s", g, err, settingsUsage)
			}
		}
		if field == "include" {
			e.Include = globs
		} else {
			e.Exclude = globs
		}
	case "rename":
		orig, renamed, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(orig) == "" {
			return "", fmt.Errorf("rename needs ORIG=NEW\n%s", settingsUsage)
		}
		if e.Rename == nil {
			e.Rename = map[string]string{}
		}
		if renamed == "" {
			delete(e.Rename, orig)
		} else {
			e.Rename[orig] = renamed
		}
	default:
		return "", fmt.Errorf("unknown mcp-server field %q (have command, args, env, placement, include, exclude, rename)\n%s", field, settingsUsage)
	}
	s.MCPServers[name] = e
	return "", nil
}

// serverEntry returns the named server entry, creating the map when needed.
func serverEntry(s *tools.Settings, name string) tools.MCPServerSetting {
	if s.MCPServers == nil {
		s.MCPServers = map[string]tools.MCPServerSetting{}
	}
	return s.MCPServers[name]
}

// splitList cuts a comma-separated flag value. Empty entries drop out, and an
// empty value is nil: clearing the list.
func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseEnabled parses a strict true or false.
func parseEnabled(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("enabled needs true or false, got %q\n%s", value, settingsUsage)
}

// checkPlacement rejects anything but the two placements.
func checkPlacement(value string) error {
	if value != tools.PlacementMain && value != tools.PlacementExecute {
		return fmt.Errorf("placement needs main or execute, got %q\n%s", value, settingsUsage)
	}
	return nil
}

// checkToolRole rejects a role no session serves tools under.
func checkToolRole(role string) error {
	for _, valid := range wick.RoleNames() {
		if role == valid {
			return nil
		}
	}
	return fmt.Errorf("unknown role %q (have %s)\n%s", role, strings.Join(wick.RoleNames(), ", "), settingsUsage)
}

// printSettings shows the stored choices, or the default in force when one is
// unset, and where each value came from. s is the effective settings, so a
// project value prints with a project source. Server env prints its keys
// only: values may hold secrets.
func printSettings(out io.Writer, s tools.Settings) {
	fmt.Fprintf(out, "settings: %s\n", tools.SettingsPath())
	fmt.Fprintf(out, "decision-model: %s (%s)\n", orDefault(s.DecisionModel, systemone.JevModelPinned), s.SourceOf("decision_model"))
	fmt.Fprintf(out, "threshold: %g (%s)\n", orDefault(s.Threshold, defaultThreshold), s.SourceOf("threshold"))
	fmt.Fprintf(out, "max-workers: %d (%s)\n", s.EffectiveMaxWorkers(), maxWorkersSource(s))
	for _, slot := range tools.RoleSlots {
		r := s.Roles[slot]
		fmt.Fprintf(out, "%s: harness %s (%s), model %s (%s), effort %s (%s)\n",
			slot,
			orDefault(r.Harness, defaultHarness), s.SourceOf("roles."+slot+".harness"),
			orDefault(r.Model, "-"), s.SourceOf("roles."+slot+".model"),
			orDefault(r.Effort, "-"), s.SourceOf("roles."+slot+".effort"))
	}
	printToolSettings(out, s)
	printMCPServers(out, s)
}

// printToolSettings shows one line per tuned tool with its effective values,
// plus a line per role override the files hold.
func printToolSettings(out io.Writer, s tools.Settings) {
	names := make([]string, 0, len(s.Tools))
	for name := range s.Tools {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(out, "tools: (none)")
		return
	}
	fmt.Fprintln(out, "tools:")
	for _, name := range names {
		e := s.Tools[name]
		line := fmt.Sprintf("  %s: enabled %t (%s), placement %s (%s)",
			name, enabledInForce(e.Enabled), s.SourceOf("tools."+name+".enabled"),
			orDefault(e.Placement, tools.PlacementMain), s.SourceOf("tools."+name+".placement"))
		if e.Rename != "" {
			line += fmt.Sprintf(", rename %s (%s)", e.Rename, s.SourceOf("tools."+name+".rename"))
		}
		fmt.Fprintln(out, line)
		roles := make([]string, 0, len(e.Roles))
		for role := range e.Roles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			ov := e.Roles[role]
			// The role line folds the entry, the way the resolver does: an
			// override the files do not set inherits the entry's value.
			enabled, enabledSrc := foldEnabled("tools."+name+".roles."+role+".enabled", ov.Enabled, "tools."+name+".enabled", e.Enabled, s)
			placement, placementSrc := foldPlacement("tools."+name+".roles."+role+".placement", ov.Placement, "tools."+name+".placement", e.Placement, tools.PlacementMain, s)
			fmt.Fprintf(out, "    %s: enabled %t (%s), placement %s (%s)\n",
				role, enabled, enabledSrc, placement, placementSrc)
		}
	}
}

// printMCPServers shows one line per registered server, with role overrides
// beneath it.
func printMCPServers(out io.Writer, s tools.Settings) {
	names := make([]string, 0, len(s.MCPServers))
	for name := range s.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		fmt.Fprintln(out, "mcp-servers: (none)")
		return
	}
	fmt.Fprintln(out, "mcp-servers:")
	for _, name := range names {
		e := s.MCPServers[name]
		line := fmt.Sprintf("  %s: command %q (%s), placement %s (%s)",
			name, e.Command, s.SourceOf("mcp_servers."+name+".command"),
			orDefault(e.Placement, tools.PlacementExecute), s.SourceOf("mcp_servers."+name+".placement"))
		if e.Args != nil {
			line += fmt.Sprintf(", args [%s] (%s)", strings.Join(e.Args, " "), s.SourceOf("mcp_servers."+name+".args"))
		}
		if e.Env != nil {
			keys := make([]string, 0, len(e.Env))
			for k := range e.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			line += fmt.Sprintf(", env [%s] (%s)", strings.Join(keys, " "), s.SourceOf("mcp_servers."+name+".env"))
		}
		if e.Include != nil {
			line += fmt.Sprintf(", include [%s] (%s)", strings.Join(e.Include, " "), s.SourceOf("mcp_servers."+name+".include"))
		}
		if e.Exclude != nil {
			line += fmt.Sprintf(", exclude [%s] (%s)", strings.Join(e.Exclude, " "), s.SourceOf("mcp_servers."+name+".exclude"))
		}
		if e.Rename != nil {
			pairs := make([]string, 0, len(e.Rename))
			for orig, renamed := range e.Rename {
				pairs = append(pairs, orig+"="+renamed)
			}
			sort.Strings(pairs)
			line += fmt.Sprintf(", rename {%s} (%s)", strings.Join(pairs, " "), s.SourceOf("mcp_servers."+name+".rename"))
		}
		fmt.Fprintln(out, line)
		roles := make([]string, 0, len(e.Roles))
		for role := range e.Roles {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		for _, role := range roles {
			ov := e.Roles[role]
			enabled, enabledSrc := foldEnabled("mcp_servers."+name+".roles."+role+".enabled", ov.Enabled, "", nil, s)
			placement, placementSrc := foldPlacement("mcp_servers."+name+".roles."+role+".placement", ov.Placement, "mcp_servers."+name+".placement", e.Placement, tools.PlacementExecute, s)
			fmt.Fprintf(out, "    %s: enabled %t (%s), placement %s (%s)\n",
				role, enabled, enabledSrc, placement, placementSrc)
		}
	}
}

// foldEnabled folds an override flag over its entry flag: the override wins
// when set, else the entry, else true. It returns the value in force and the
// source of the layer that provided it.
func foldEnabled(ovKey string, ov *bool, entryKey string, entry *bool, s tools.Settings) (bool, string) {
	if ov != nil {
		return *ov, s.SourceOf(ovKey)
	}
	if entry != nil {
		return *entry, s.SourceOf(entryKey)
	}
	return true, "default"
}

// foldPlacement folds an override placement over its entry placement the same
// way, defaulting to fallback.
func foldPlacement(ovKey, ov, entryKey, entry, fallback string, s tools.Settings) (string, string) {
	if ov != "" {
		return ov, s.SourceOf(ovKey)
	}
	if entry != "" {
		return entry, s.SourceOf(entryKey)
	}
	return fallback, "default"
}

// maxWorkersSource is where the worker cap came from. The env wins when set,
// then the project file, then the global file. A missing or non-positive
// value stays the default, the way decode treats it: the value and the name
// it prints must agree. A just-saved value reads as the file's even though
// the loaded source map still says default, the way a role field's source
// follows its value.
func maxWorkersSource(s tools.Settings) string {
	switch s.SourceOf("max_workers") {
	case "env", "project":
		return s.SourceOf("max_workers")
	}
	if s.MaxWorkers > 0 {
		return "file"
	}
	return "default"
}

// enabledInForce is the enabled flag in force: the stored value, or true
// when nothing is stored.
// orDefault returns value, or the fallback when value is zero.
func enabledInForce(stored *bool) bool {
	if stored != nil {
		return *stored
	}
	return true
}

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
