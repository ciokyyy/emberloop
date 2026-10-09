package el

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// SpecTitle reads a spec file's title: its first level-1 markdown heading.
// It reports "" when the file cannot be read or holds no heading.
func SpecTitle(specPath string) string {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return ""
	}
	return headingTitle(string(raw))
}

// headingTitle returns the first "# ..." heading of a markdown document,
// with an optional leading ticket number ("01: ...") stripped. Later
// headings never count: the title is the document's own heading.
func headingTitle(content string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		head := strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
		i := 0
		for i < len(head) && head[i] >= '0' && head[i] <= '9' {
			i++
		}
		if i > 0 && i < len(head) && (head[i] == ':' || head[i] == '-' || head[i] == '.') {
			head = strings.TrimSpace(head[i+1:])
		}
		return head
	}
	return ""
}

// TicketPreview is one ticket as the Runs preview shows it: its id, number
// and title, its blockers as resolved ticket ids, and its checkbox progress.
// Error names a ticket the preview could not parse; it blocks nothing else
// in the list. An error row carries no blockers.
type TicketPreview struct {
	ID       string   `json:"id"`
	Num      int      `json:"num"`
	Title    string   `json:"title"`
	Blockers []string `json:"blockers"`
	Done     int      `json:"done"`
	Total    int      `json:"total"`
	Error    string   `json:"error,omitempty"`
}

// ticketTitle reads a ticket document's title: its first "# ..." heading
// with the leading ticket number stripped.
func ticketTitle(content string) string {
	return headingTitle(content)
}

// countCheckboxes counts a ticket document's checkboxes and how many tick.
func countCheckboxes(content string) (done, total int) {
	for _, line := range strings.Split(content, "\n") {
		_, checked, ok := checkboxText(line)
		if !ok {
			continue
		}
		total++
		if checked {
			done++
		}
	}
	return done, total
}

// PreviewTickets builds the per-ticket preview for a spec from its ticket
// files. Unlike the driver, one bad ticket fails only itself: its row
// carries the error and the rest still resolve. The spec file itself never
// counts as a ticket.
func PreviewTickets(spec SpecRun, files map[string]string) []TicketPreview {
	dir := filepath.Clean(spec.Tickets)
	var paths []string
	for path := range files {
		if path == spec.Spec {
			continue
		}
		if filepath.Dir(filepath.Clean(path)) != dir || !strings.HasSuffix(path, ".md") {
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	type parsed struct {
		preview TicketPreview
		refs    []string
	}
	var tickets []parsed
	for _, path := range paths {
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		p := parsed{preview: TicketPreview{ID: base, Blockers: []string{}}}
		p.preview.Title = ticketTitle(files[path])
		p.preview.Done, p.preview.Total = countCheckboxes(files[path])
		num, ok := driverNumber(base)
		if !ok {
			p.preview.Error = fmt.Sprintf("ticket %s has no ticket number", path)
			tickets = append(tickets, p)
			continue
		}
		p.preview.Num = num
		value, line, found, err := driverBlockedValue(files[path])
		if err != nil {
			p.preview.Error = fmt.Sprintf("%s:%d: %s", path, line, err)
			tickets = append(tickets, p)
			continue
		}
		if !found {
			p.preview.Error = fmt.Sprintf("ticket %s has no Blocked by line", path)
			tickets = append(tickets, p)
			continue
		}
		refs, err := parseBlockerRefs(value)
		if err != nil {
			p.preview.Error = fmt.Sprintf("%s:%d: %s", path, line, err)
			tickets = append(tickets, p)
			continue
		}
		p.refs = refs
		tickets = append(tickets, p)
	}
	ids := make(map[string]int, len(tickets))
	byNum := make(map[int][]string)
	for _, t := range tickets {
		if t.preview.Error != "" {
			continue
		}
		ids[t.preview.ID] = t.preview.Num
		byNum[t.preview.Num] = append(byNum[t.preview.Num], t.preview.ID)
	}
	for i := range tickets {
		p := &tickets[i]
		if p.preview.Error != "" || len(p.refs) == 0 {
			continue
		}
		var resolved []string
		fail := func(format string, args ...any) {
			if p.preview.Error == "" {
				p.preview.Error = fmt.Sprintf(format, args...)
			}
		}
		for _, ref := range p.refs {
			if strings.Contains(ref, "-") {
				if _, ok := ids[ref]; ok {
					resolved = append(resolved, ref)
				} else {
					fail("ticket %q blocked by unknown ticket %q", p.preview.ID, ref)
				}
				continue
			}
			n, _ := strconv.Atoi(ref)
			switch matches := byNum[n]; len(matches) {
			case 1:
				resolved = append(resolved, matches[0])
			case 0:
				fail("ticket %q blocked by unknown ticket %q", p.preview.ID, ref)
			default:
				sort.Strings(matches)
				fail("ticket %q blocked by ambiguous ticket %q: %s", p.preview.ID, ref, strings.Join(matches, ", "))
			}
		}
		if p.preview.Error != "" {
			resolved = []string{}
		}
		sort.Slice(resolved, func(a, b int) bool {
			if ids[resolved[a]] != ids[resolved[b]] {
				return ids[resolved[a]] < ids[resolved[b]]
			}
			return resolved[a] < resolved[b]
		})
		p.preview.Blockers = resolved
	}
	out := make([]TicketPreview, 0, len(tickets))
	for _, t := range tickets {
		out = append(out, t.preview)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Num != out[j].Num {
			return out[i].Num < out[j].Num
		}
		return out[i].ID < out[j].ID
	})
	return out
}
