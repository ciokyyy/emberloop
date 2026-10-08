package emberloop

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DriverWorld is the injected world the deterministic driver runs against:
// the ticket files, a fake clock, and the operations the driver calls. Tests
// build it from a map and a fake; the CLI fills the files from disk.
type DriverWorld struct {
	// Files holds every ticket file by path. RunDriver only reads entries
	// under the spec's tickets directory ending in .md.
	Files map[string]string
	// Now is the clock the driver reads. Nil means time.Now; tests inject a
	// fake. Check mode keeps no time, but the seam stays one entry point for
	// the later tickets that retry with backoff.
	Now func() time.Time
	// StartWorker starts one ticket's worker. The graph check runs before any
	// worker would start, so check mode never calls it.
	StartWorker func(DriverTicket) error
}

// DriverTicket is one ticket as the driver sees it: its id (the file base),
// its number (the leading digits), and its blockers as resolved ticket ids.
type DriverTicket struct {
	ID       string
	Num      int
	Path     string
	Blockers []string
}

// DriverResult is what the driver computed: the tickets in the order work
// would start, by ticket number among tickets whose blockers are done.
type DriverResult struct {
	Order []DriverTicket
}

// RunDriver is the driver's one seam: the spec plus the injected world in,
// the start order out. It reads every ticket's Blocked by line first, and a
// line that does not parse, an id that names no ticket, or a dependency cycle
// fails before any worker starts.
func RunDriver(spec SpecRun, world DriverWorld) (DriverResult, error) {
	tickets, err := readDriverTickets(spec, world.Files)
	if err != nil {
		return DriverResult{}, err
	}
	order, err := orderDriverTickets(tickets)
	if err != nil {
		return DriverResult{}, err
	}
	return DriverResult{Order: order}, nil
}

// RunDriverCheck reads the spec's tickets from disk, checks the graph through
// RunDriver, and prints the start order, one ticket id per line. It is what
// `emberloop implement <spec> --driver --check` runs.
func RunDriverCheck(spec SpecRun, stdout io.Writer) error {
	entries, err := os.ReadDir(spec.Tickets)
	if err != nil {
		return err
	}
	files := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(spec.Tickets, e.Name()))
		if err != nil {
			return err
		}
		files[filepath.Join(spec.Tickets, e.Name())] = string(raw)
	}
	res, err := RunDriver(spec, DriverWorld{Files: files})
	if err != nil {
		return err
	}
	for _, tk := range res.Order {
		fmt.Fprintln(stdout, tk.ID)
	}
	return nil
}

// readDriverTickets parses every ticket file under the spec's tickets
// directory: the Blocked by line, then each id resolved against the tickets
// present. The first failure names the file (and the line for a bad line).
func readDriverTickets(spec SpecRun, files map[string]string) (map[string]*DriverTicket, error) {
	dir := filepath.Clean(spec.Tickets)
	var paths []string
	for path := range files {
		if filepath.Dir(filepath.Clean(path)) != dir || !strings.HasSuffix(path, ".md") {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no tickets under %s", spec.Tickets)
	}
	tickets := make(map[string]*DriverTicket, len(paths))
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		num, ok := driverNumber(base)
		if !ok {
			return nil, fmt.Errorf("ticket %s has no ticket number", path)
		}
		value, line, found, err := driverBlockedValue(files[path])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if !found {
			return nil, fmt.Errorf("ticket %s has no Blocked by line", path)
		}
		refs, err := parseBlockerRefs(value)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		tickets[base] = &DriverTicket{ID: base, Num: num, Path: path}
		tickets[base].Blockers = refs
	}
	if err := resolveDriverRefs(tickets); err != nil {
		return nil, err
	}
	out := make(map[string]*DriverTicket, len(tickets))
	for id, tk := range tickets {
		out[id] = tk
	}
	return out, nil
}

// driverBlockedValue returns the text after the colon of the ticket's first
// Blocked by line, with its 1-based line number. The line is a field line: it
// starts with Blocked by (bold markers, quotes and list dashes allowed), so a
// mention deeper in the text never matches. A Blocked by line without a colon
// is found but unparsable, so it still reports its line.
func driverBlockedValue(content string) (value string, line int, found bool, err error) {
	for i, raw := range strings.Split(content, "\n") {
		field := strings.TrimSpace(raw)
		field = strings.TrimSpace(strings.TrimLeft(field, ">-"))
		field = strings.TrimLeft(field, "*_")
		if !strings.HasPrefix(field, "Blocked by") {
			continue
		}
		rest := strings.TrimSpace(field[len("Blocked by"):])
		if !strings.HasPrefix(rest, ":") {
			return "", i + 1, true, fmt.Errorf("cannot parse Blocked by line %q", strings.TrimSpace(raw))
		}
		// The line is usually bold markdown (`**Blocked by:** ...`), so the
		// value carries the closing markers: strip emphasis around it.
		value := strings.TrimSpace(strings.Trim(strings.TrimSpace(rest[1:]), "*_"))
		return value, i + 1, true, nil
	}
	return "", 0, false, nil
}

// parseBlockerRefs reads the ticket ids at the start of a Blocked by value.
// The ids come first; everything after is free text for humans. Empty, None
// and nothing mean no blockers. Parenthesized asides are free text too, so
// commas inside them never split the id list. Free text may itself hold
// commas: a comma segment with no leading id continues the free text once
// free text has started, while a segment with a leading id still adds
// blockers. A segment with no leading id before any free text fails, so a
// typo never reads as no blockers.
func parseBlockerRefs(value string) ([]string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if first := strings.Fields(trimmed)[0]; strings.EqualFold(first, "none") || strings.EqualFold(first, "nothing") {
		return nil, nil
	}
	rest, ok := stripParens(trimmed)
	if !ok {
		return nil, fmt.Errorf("cannot parse Blocked by line %q", value)
	}
	fail := func() ([]string, error) {
		return nil, fmt.Errorf("cannot parse Blocked by line %q", value)
	}
	var refs []string
	freeText := false
	for _, segment := range strings.Split(rest, ",") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			return fail()
		}
		fields := strings.Fields(segment)
		ids := 0
		for _, field := range fields {
			if !isTicketRef(field) {
				break
			}
			refs = append(refs, field)
			ids++
		}
		if ids == 0 {
			// No leading id: free-text continuation once free text started,
			// else a line that does not parse.
			if len(refs) == 0 || !freeText {
				return fail()
			}
			continue
		}
		if ids < len(fields) {
			freeText = true
		}
	}
	return refs, nil
}

// stripParens drops every parenthesized run, which is always free text. An
// unbalanced paren leaves the line unparsable.
func stripParens(s string) (string, bool) {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return "", false
			}
			depth--
		default:
			if depth == 0 {
				b.WriteRune(r)
			}
		}
	}
	if depth != 0 {
		return "", false
	}
	return b.String(), true
}

// isTicketRef reports whether a field is one ticket id: digits with an
// optional -slug, like 02 or 02-ticket-graph.
func isTicketRef(field string) bool {
	head, tail, dashed := field, "", false
	if i := strings.Index(field, "-"); i >= 0 {
		head, tail, dashed = field[:i], field[i+1:], true
	}
	if head == "" || (dashed && tail == "") {
		return false
	}
	for _, r := range head {
		if r < '0' || r > '9' {
			return false
		}
	}
	if tail == "" {
		return true
	}
	for _, r := range tail {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

// driverNumber reads the leading digits of a ticket id: 02-ticket-graph is
// ticket 2.
func driverNumber(id string) (int, bool) {
	i := 0
	for i < len(id) && id[i] >= '0' && id[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(id[:i])
	if err != nil {
		return 0, false
	}
	return n, true
}

// resolveDriverRefs turns each ticket's raw refs into ticket ids. A bare
// number matches the ticket with that number; a full slug matches the file
// base. Unknown and ambiguous refs fail together naming every ticket involved.
func resolveDriverRefs(tickets map[string]*DriverTicket) error {
	byNum := make(map[int][]string)
	for id, tk := range tickets {
		byNum[tk.Num] = append(byNum[tk.Num], id)
	}
	for _, ids := range byNum {
		sort.Strings(ids)
	}
	var problems []string
	ids := make([]string, 0, len(tickets))
	for id := range tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		tk := tickets[id]
		resolved := make([]string, 0, len(tk.Blockers))
		for _, ref := range tk.Blockers {
			if strings.Contains(ref, "-") {
				if _, ok := tickets[ref]; ok {
					resolved = append(resolved, ref)
					continue
				}
				problems = append(problems, fmt.Sprintf("ticket %q blocked by unknown ticket %q", id, ref))
				continue
			}
			n, _ := strconv.Atoi(ref)
			switch matches := byNum[n]; len(matches) {
			case 1:
				resolved = append(resolved, matches[0])
			case 0:
				problems = append(problems, fmt.Sprintf("ticket %q blocked by unknown ticket %q", id, ref))
			default:
				problems = append(problems, fmt.Sprintf("ticket %q blocked by ambiguous ticket %q: %s", id, ref, strings.Join(matches, ", ")))
			}
		}
		sort.Slice(resolved, func(i, j int) bool {
			if tickets[resolved[i]].Num != tickets[resolved[j]].Num {
				return tickets[resolved[i]].Num < tickets[resolved[j]].Num
			}
			return resolved[i] < resolved[j]
		})
		tk.Blockers = resolved
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

// orderDriverTickets sorts tickets so every ticket follows its blockers. Among
// the tickets whose blockers are done the lowest ticket number goes first. A
// cycle fails naming the tickets in the cycle.
func orderDriverTickets(tickets map[string]*DriverTicket) ([]DriverTicket, error) {
	dependents := make(map[string][]string, len(tickets))
	pending := make(map[string]int, len(tickets))
	for id, tk := range tickets {
		pending[id] = len(tk.Blockers)
		for _, b := range tk.Blockers {
			dependents[b] = append(dependents[b], id)
		}
	}
	var ready []string
	for id, n := range pending {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	var order []DriverTicket
	for len(ready) > 0 {
		sort.Slice(ready, func(i, j int) bool {
			if tickets[ready[i]].Num != tickets[ready[j]].Num {
				return tickets[ready[i]].Num < tickets[ready[j]].Num
			}
			return ready[i] < ready[j]
		})
		next := ready[0]
		ready = ready[1:]
		order = append(order, *tickets[next])
		for _, dep := range dependents[next] {
			pending[dep]--
			if pending[dep] == 0 {
				ready = append(ready, dep)
			}
		}
		delete(pending, next)
	}
	if len(pending) > 0 {
		return nil, fmt.Errorf("dependency cycle: %s", strings.Join(driverCycle(tickets, pending), " -> "))
	}
	return order, nil
}

// driverCycle finds one cycle among the tickets Kahn's pass could not order,
// so the error names the tickets in the cycle rather than every ticket stuck
// behind it.
func driverCycle(tickets map[string]*DriverTicket, stuck map[string]int) []string {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := make(map[string]int, len(stuck))
	var stack []string
	var cycle []string
	var visit func(id string) bool
	visit = func(id string) bool {
		color[id] = grey
		stack = append(stack, id)
		blockers := append([]string(nil), tickets[id].Blockers...)
		sort.Strings(blockers)
		for _, b := range blockers {
			if _, ok := stuck[b]; !ok {
				continue
			}
			switch color[b] {
			case grey:
				start := 0
				for stack[start] != b {
					start++
				}
				cycle = append(append([]string(nil), stack[start:]...), b)
				return true
			case white:
				if visit(b) {
					return true
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return false
	}
	stuckIDs := make([]string, 0, len(stuck))
	for id := range stuck {
		stuckIDs = append(stuckIDs, id)
	}
	sort.Strings(stuckIDs)
	for _, id := range stuckIDs {
		if color[id] == white && visit(id) {
			return cycle
		}
	}
	return stuckIDs
}
