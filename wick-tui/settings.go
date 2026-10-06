package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	wick "example.com/wick"
	"example.com/wick/tools"
	"example.com/wick/tools/systemone"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// The Settings tab edits the global settings file through the same
// load/save functions the CLI uses, with model choices from each harness's
// cached Models() list. The tab never hard-codes a model list: every picker
// row comes from the backend's Models, and free text stays allowed.

// SettingsBackend is the seam the Settings tab edits through. The live tab
// uses LiveBackend; tests point Load/Save at a temp settings file (the
// shared functions, so a tab edit is one the CLI reads back) and stub the
// model lists.
type SettingsBackend struct {
	Load        func() (tools.Settings, error)
	Save        func(tools.Settings) error
	Models      func(harness string, refresh bool) ([]wick.Model, error)
	Harnesses   func() []string
	ArchiveSize func() int64
}

// LiveBackend wires the tab to the global settings file, the harness model
// caches and the archive, the way `wick-pig settings` reads them.
func LiveBackend() SettingsBackend {
	return SettingsBackend{
		Load: func() (tools.Settings, error) { return tools.LoadSettings(), nil },
		Save: tools.SaveSettings,
		Models: func(harness string, refresh bool) ([]wick.Model, error) {
			return wick.CachedModels(harness, wick.OSExec, refresh)
		},
		Harnesses:   wick.HarnessNames,
		ArchiveSize: wick.ArchiveSize,
	}
}

// settingsEdit is which value the free-text editor holds.
type settingsEdit int

const (
	editNone settingsEdit = iota
	editSlotModel
	editDecision
	editThreshold
)

// settingsState is the Settings tab: the loaded file, the cached model
// lists per harness, the cursor, and whichever picker or editor is open. Row
// 0..len(RoleSlots)-1 is a role slot, then the decision model, then the
// threshold. On a role row col 0 is the harness, col 1 the model, and col
// 2+i the row's i'th effort level; the other rows have only col 0.
type settingsState struct {
	backend      SettingsBackend
	loaded       bool
	values       tools.Settings
	harnesses    []string
	lists        map[string][]wick.Model
	listErr      map[string]string
	archiveBytes int64
	row, col     int
	pickerOpen   bool
	pickerSlot   string
	pickerItems  []wick.Model
	pickerIndex  int
	editing      settingsEdit
	editSlot     string
	editor       textinput.Model
	notice       string
}

// settingsRows is the tab's row count: every role slot, then the decision
// model, then the threshold.
func settingsRows() int { return len(tools.RoleSlots) + 2 }

// settingsSlot is the role slot row r names.
func settingsSlot(r int) string { return tools.RoleSlots[r] }

// orSetting is value, or the fallback when value is zero.
func orSetting[T comparable](value, fallback T) T {
	var zero T
	if value == zero {
		return fallback
	}
	return value
}

// roleSource is where a role field came from, the way the settings command
// prints it: the file owns any value it holds, an unset field is the
// built-in default.
func roleSource(value string) string {
	if value == "" {
		return "default"
	}
	return "file"
}

// effectiveHarness is the harness a slot launches with: the stored one, or
// the resolver's built-in default when nothing is stored.
func (s *settingsState) effectiveHarness(slot string) string {
	return orSetting(s.values.Roles[slot].Harness, wick.DefaultHarness)
}

// effortLevels is the row the tab shows under a slot: the selected model's
// levels when the model is known, the union over the harness's list when no
// model is chosen (the way the settings command judges a model-less effort),
// and none for a free-text model the list does not know.
func (s *settingsState) effortLevels(slot string) []string {
	role := s.values.Roles[slot]
	list := s.lists[s.effectiveHarness(slot)]
	if role.Model != "" {
		if m, ok := findSettingsModel(list, role.Model); ok {
			return m.Efforts
		}
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range list {
		for _, e := range m.Efforts {
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// maxCol is the rightmost cursor column of a row.
func (s *settingsState) maxCol(row int) int {
	if row < len(tools.RoleSlots) {
		return 1 + len(s.effortLevels(settingsSlot(row)))
	}
	return 0
}

// findSettingsModel finds one model by id.
func findSettingsModel(list []wick.Model, id string) (wick.Model, bool) {
	for _, m := range list {
		if m.ID == id {
			return m, true
		}
	}
	return wick.Model{}, false
}

// loadSettingsTab loads the file, the harness names, every harness's cached
// model list and the archive size. Lists come from the cache only: a stale
// list refreshes explicitly with `r`, never behind the cursor's back.
func (m *Model) loadSettingsTab() {
	s := &m.settings
	if s.backend.Load == nil {
		return
	}
	if v, err := s.backend.Load(); err == nil {
		s.values = v
	} else {
		s.notice = "settings load failed: " + err.Error()
	}
	s.harnesses = nil
	if s.backend.Harnesses != nil {
		s.harnesses = s.backend.Harnesses()
	}
	s.lists = map[string][]wick.Model{}
	s.listErr = map[string]string{}
	if s.backend.Models != nil {
		for _, h := range s.harnesses {
			list, err := s.backend.Models(h, false)
			if err != nil {
				s.listErr[h] = err.Error()
				continue
			}
			s.lists[h] = list
		}
	}
	if s.backend.ArchiveSize != nil {
		s.archiveBytes = s.backend.ArchiveSize()
	}
	s.loaded = true
	if s.pickerOpen {
		s.pickerItems = s.lists[s.effectiveHarness(s.pickerSlot)]
		s.pickerIndex = min(max(s.pickerIndex, 0), max(len(s.pickerItems)-1, 0))
	}
	s.row = min(max(s.row, 0), settingsRows()-1)
	s.col = min(max(s.col, 0), s.maxCol(s.row))
}

// reloadSettingsValues re-reads the file on a poll tick, so a CLI edit shows
// up without disturbing the cursor, an open picker or an editor.
func (m *Model) reloadSettingsValues() {
	s := &m.settings
	if m.tab != tabSettings || !s.loaded || s.editing != editNone || s.pickerOpen {
		return
	}
	if s.backend.Load == nil {
		return
	}
	if v, err := s.backend.Load(); err == nil {
		s.values = v
		s.col = min(s.col, s.maxCol(s.row))
	}
}

// viewSettings renders the tab: every role slot with harness, model and
// effort plus source, the effort row under it, then the decision model, the
// threshold and the archive size.
func (m Model) viewSettings() string {
	s := &m.settings
	var b strings.Builder
	for r := 0; r < len(tools.RoleSlots); r++ {
		slot := settingsSlot(r)
		role := s.values.Roles[slot]
		fmt.Fprintf(&b, "%s: harness %s (%s), model %s (%s), effort %s (%s)\n",
			slot,
			m.settingsZone(settingsHarnessZone(slot), r, 0, orSetting(role.Harness, wick.DefaultHarness)),
			roleSource(role.Harness),
			m.settingsZone(settingsModelZone(slot), r, 1, orSetting(role.Model, "-")),
			roleSource(role.Model),
			orSetting(role.Effort, "-"),
			roleSource(role.Effort))
		if levels := s.effortLevels(slot); len(levels) > 0 {
			b.WriteString("  " + m.effortRow(slot, r, levels) + "\n")
		}
		if s.pickerOpen && s.pickerSlot == slot {
			b.WriteString(m.viewPicker())
		}
		if s.editing == editSlotModel && s.editSlot == slot {
			fmt.Fprintf(&b, "  model for %s: %s (enter saves, esc cancels)\n", slot, s.editor.View())
		}
	}

	decision := orSetting(s.values.DecisionModel, systemone.JevModelPinned)
	fmt.Fprintf(&b, "decision-model: %s (%s)\n",
		m.settingsZone(settingsDecisionZone(), len(tools.RoleSlots), 0, decision),
		s.values.SourceOf("decision_model"))
	threshold := s.values.Threshold
	fmt.Fprintf(&b, "threshold: %s (%s)\n",
		m.settingsZone(settingsThresholdZone(), len(tools.RoleSlots)+1, 0, fmt.Sprint(orSetting(threshold, tools.DefaultThreshold))),
		s.values.SourceOf("threshold"))
	if s.editing == editDecision {
		fmt.Fprintf(&b, "  decision-model: %s (enter saves, esc cancels)\n", s.editor.View())
	}
	if s.editing == editThreshold {
		fmt.Fprintf(&b, "  threshold: %s (enter saves, esc cancels)\n", s.editor.View())
	}
	fmt.Fprintf(&b, "archive: %s\n", formatBytes(s.archiveBytes))
	if s.notice != "" {
		fmt.Fprintf(&b, "! %s\n", strings.TrimSpace(s.notice))
	}
	return b.String()
}

// settingsZone wraps a value in its click region, bolding the focused one.
func (m Model) settingsZone(id string, row, col int, text string) string {
	if m.settings.row == row && m.settings.col == col {
		text = styleSel.Render(text)
	}
	return m.zone.Mark(id, text)
}

// effortRow renders a slot's levels with the chosen one bracketed.
func (m Model) effortRow(slot string, row int, levels []string) string {
	chosen := m.settings.values.Roles[slot].Effort
	parts := make([]string, 0, len(levels))
	for i, l := range levels {
		text := l
		if l == chosen {
			text = "[" + l + "]"
		}
		if m.settings.row == row && m.settings.col == 2+i {
			text = styleSel.Render(text)
		}
		parts = append(parts, m.zone.Mark(settingsLevelZone(slot, i), text))
	}
	return strings.Join(parts, "  ")
}

// Zone ids name what a click changes: a slot's harness, model or i'th
// effort level, the decision model, the threshold, or the picker's i'th
// model.
func settingsHarnessZone(slot string) string { return "seth:" + slot }
func settingsModelZone(slot string) string   { return "setm:" + slot }
func settingsLevelZone(slot string, i int) string {
	return "sete:" + slot + ":" + fmt.Sprint(i)
}
func settingsDecisionZone() string    { return "setd" }
func settingsThresholdZone() string   { return "sett" }
func settingsPickerZone(i int) string { return "setp:" + fmt.Sprint(i) }
func sortedHarnesses(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}

// formatBytes renders a byte count for the archive line. The view layer owns
// formatting, the way it owns the status glyphs.
func formatBytes(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	f := float64(n) / 1024
	for _, u := range []string{"KB", "MB", "GB"} {
		if f < 1024 || u == "GB" {
			return fmt.Sprintf("%.1f %s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%d B", n)
}

// updateSettings routes Settings keys, clicks and refresh results. A click
// selects and changes in one press.
func (m Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.settingsKey(msg)
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			m.settingsClick(msg)
		}
		return m, nil
	case settingsRefreshMsg:
		m.settings.applyRefresh(msg)
		return m, nil
	}
	return m, nil
}

// settingsClick changes the value whose region holds the press: a harness
// cycles, a model opens its picker, a level or picker option applies, and
// the decision model and threshold open their editors.
func (m *Model) settingsClick(msg tea.MouseMsg) {
	s := &m.settings
	if s.pickerOpen {
		for i := range s.pickerItems {
			if m.zone.Get(settingsPickerZone(i)).InBounds(msg) {
				s.row, s.col = s.slotRow(s.pickerSlot), 1
				s.pickerIndex = i
				s.pickHighlighted()
				m.lastClick, m.lastZone = time.Now(), settingsPickerZone(i)
				return
			}
		}
		return
	}
	for r := 0; r < len(tools.RoleSlots); r++ {
		slot := settingsSlot(r)
		switch {
		case m.zone.Get(settingsHarnessZone(slot)).InBounds(msg):
			s.row, s.col = r, 0
			m.lastClick, m.lastZone = time.Now(), settingsHarnessZone(slot)
			s.cycleHarness(slot)
			return
		case m.zone.Get(settingsModelZone(slot)).InBounds(msg):
			s.row, s.col = r, 1
			m.lastClick, m.lastZone = time.Now(), settingsModelZone(slot)
			s.openPicker(slot)
			return
		}
		for i, level := range s.effortLevels(slot) {
			if m.zone.Get(settingsLevelZone(slot, i)).InBounds(msg) {
				s.row, s.col = r, 2+i
				m.lastClick, m.lastZone = time.Now(), settingsLevelZone(slot, i)
				s.setEffort(slot, level)
				return
			}
		}
	}
	if m.zone.Get(settingsDecisionZone()).InBounds(msg) {
		s.row, s.col = len(tools.RoleSlots), 0
		m.lastClick, m.lastZone = time.Now(), settingsDecisionZone()
		s.activate()
		return
	}
	if m.zone.Get(settingsThresholdZone()).InBounds(msg) {
		s.row, s.col = len(tools.RoleSlots)+1, 0
		m.lastClick, m.lastZone = time.Now(), settingsThresholdZone()
		s.activate()
	}
}

// slotRow is the tab row naming a role slot.
func (s *settingsState) slotRow(slot string) int {
	for r := 0; r < len(tools.RoleSlots); r++ {
		if settingsSlot(r) == slot {
			return r
		}
	}
	return 0
}

// settingsKey drives the tab: rows and fields move, enter changes the
// focused value, q quits. An open editor takes every key but esc, enter
// and ctrl-c; an open picker takes arrows, enter to pick and esc to close,
// and any text straight into free text (so even ids starting with j, k or
// r stay typeable — `r` refreshes only with the picker closed).
func (m Model) settingsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := &m.settings
	if s.editing != editNone {
		switch msg.Type {
		case tea.KeyEsc:
			s.editing = editNone
			return m, nil
		case tea.KeyEnter:
			s.commitEditor()
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		var cmd tea.Cmd
		s.editor, cmd = s.editor.Update(msg)
		return m, cmd
	}
	if s.pickerOpen {
		switch {
		case msg.Type == tea.KeyEsc:
			s.pickerOpen = false
			return m, nil
		case msg.Type == tea.KeyEnter:
			s.pickHighlighted()
			return m, nil
		case msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case msg.Type == tea.KeyUp:
			s.movePicker(-1)
			return m, nil
		case msg.Type == tea.KeyDown:
			s.movePicker(1)
			return m, nil
		case msg.Type == tea.KeyRunes && len(msg.Runes) >= 1:
			cmd := s.openModelEditor()
			s.editor, cmd = s.editor.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	switch {
	case msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc:
		return m, tea.Quit
	case isSettingsRune(msg, 'q'):
		return m, tea.Quit
	case msg.Type == tea.KeyUp || isSettingsRune(msg, 'k'):
		s.moveCursor(-1, 0)
		return m, nil
	case msg.Type == tea.KeyDown || isSettingsRune(msg, 'j'):
		s.moveCursor(1, 0)
		return m, nil
	case msg.Type == tea.KeyLeft || isSettingsRune(msg, 'h'):
		s.moveCursor(0, -1)
		return m, nil
	case msg.Type == tea.KeyRight || isSettingsRune(msg, 'l'):
		s.moveCursor(0, 1)
		return m, nil
	case msg.Type == tea.KeyEnter || isSettingsRune(msg, ' '):
		s.activate()
		return m, nil
	case isSettingsRune(msg, 'r'):
		return m, m.refreshModelsCmd()
	}
	return m, nil
}

// settingsRefreshMsg carries rebuilt model lists back: successes replace
// the cache, failures keep it.
type settingsRefreshMsg struct {
	lists map[string][]wick.Model
	errs  map[string]string
}

// refreshModelsCmd rebuilds every harness's model list off the main loop —
// a live rebuild shells out to the harness CLIs and must not freeze the
// tab — and re-reads the archive size.
func (m Model) refreshModelsCmd() tea.Cmd {
	be := m.settings.backend
	var harnesses []string
	if be.Harnesses != nil {
		harnesses = be.Harnesses()
	}
	return func() tea.Msg {
		msg := settingsRefreshMsg{lists: map[string][]wick.Model{}, errs: map[string]string{}}
		if be.Models == nil {
			return msg
		}
		for _, h := range harnesses {
			list, err := be.Models(h, true)
			if err != nil {
				msg.errs[h] = err.Error()
				continue
			}
			msg.lists[h] = list
		}
		return msg
	}
}

// applyRefresh merges rebuilt lists: successes replace, failures keep the
// cached list with a notice naming the harness.
func (s *settingsState) applyRefresh(msg settingsRefreshMsg) {
	if s.lists == nil {
		s.lists = map[string][]wick.Model{}
	}
	for h, list := range msg.lists {
		s.lists[h] = list
		delete(s.listErr, h)
	}
	for h, e := range msg.errs {
		if s.listErr == nil {
			s.listErr = map[string]string{}
		}
		s.listErr[h] = e
	}
	if s.backend.ArchiveSize != nil {
		s.archiveBytes = s.backend.ArchiveSize()
	}
	if s.pickerOpen {
		s.pickerItems = s.lists[s.effectiveHarness(s.pickerSlot)]
		s.pickerIndex = min(max(s.pickerIndex, 0), max(len(s.pickerItems)-1, 0))
	}
	if len(msg.errs) == 0 {
		s.notice = "model lists refreshed"
		return
	}
	names := make([]string, 0, len(msg.errs))
	for h := range msg.errs {
		names = append(names, h)
	}
	sort.Strings(names)
	s.notice = fmt.Sprintf("refresh: %s unavailable (%s); kept cached list", strings.Join(names, ", "), msg.errs[names[0]])
}

// isSettingsRune reports a single-rune key press.
func isSettingsRune(msg tea.KeyMsg, r rune) bool {
	return msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == r
}

// moveCursor moves the focus, clearing the last notice.
func (s *settingsState) moveCursor(dr, dc int) {
	s.row = min(max(s.row+dr, 0), settingsRows()-1)
	s.col = min(max(s.col+dc, 0), s.maxCol(s.row))
	s.notice = ""
}

// activate changes the focused value: the harness cycles, the model opens
// its picker, a level applies, and the decision model and threshold open
// their editors.
func (s *settingsState) activate() {
	if s.row == len(tools.RoleSlots) {
		s.openEditor(editDecision, "", s.values.DecisionModel, systemone.JevModelPinned)
		return
	}
	if s.row == len(tools.RoleSlots)+1 {
		prefill := ""
		if s.values.Threshold != 0 {
			prefill = fmt.Sprint(s.values.Threshold)
		}
		s.openEditor(editThreshold, "", prefill, fmt.Sprint(tools.DefaultThreshold))
		return
	}
	if s.row >= len(tools.RoleSlots) {
		return
	}
	slot := settingsSlot(s.row)
	switch s.col {
	case 0:
		s.cycleHarness(slot)
	case 1:
		s.openPicker(slot)
	default:
		if levels := s.effortLevels(slot); s.col-2 >= 0 && s.col-2 < len(levels) {
			s.setEffort(slot, levels[s.col-2])
		}
	}
}

// openEditor opens free text for one value, prefilled with what is stored
// and hinting the default in force.
func (s *settingsState) openEditor(kind settingsEdit, slot, prefill, placeholder string) tea.Cmd {
	s.pickerOpen = false
	s.editing = kind
	s.editSlot = slot
	ed := textinput.New()
	ed.SetValue(prefill)
	ed.Placeholder = placeholder
	cmd := ed.Focus()
	s.editor = ed
	return cmd
}

// setEffort saves one effort level, which the row only offers when the
// model takes it.
func (s *settingsState) setEffort(slot, effort string) {
	role := s.values.Roles[slot]
	role.Effort = effort
	s.storeRole(slot, role, "")
}

// openPicker opens the model list of a slot's harness, highlighting the
// saved model.
func (s *settingsState) openPicker(slot string) {
	s.editing = editNone
	s.pickerOpen = true
	s.pickerSlot = slot
	s.pickerItems = s.lists[s.effectiveHarness(slot)]
	s.pickerIndex = 0
	for i, item := range s.pickerItems {
		if item.ID == s.values.Roles[slot].Model {
			s.pickerIndex = i
			break
		}
	}
}

// movePicker moves the picker highlight.
func (s *settingsState) movePicker(d int) {
	if len(s.pickerItems) == 0 {
		return
	}
	s.pickerIndex = min(max(s.pickerIndex+d, 0), len(s.pickerItems)-1)
}

// pickHighlighted saves the highlighted model.
func (s *settingsState) pickHighlighted() {
	if !s.pickerOpen {
		return
	}
	s.pickerOpen = false
	if len(s.pickerItems) == 0 {
		return
	}
	s.saveSlotModel(s.pickerSlot, s.pickerItems[min(max(s.pickerIndex, 0), len(s.pickerItems)-1)].ID)
}

// openModelEditor leaves the picker for free text, starting empty with the
// highlighted id as the hint. The triggering keystroke is fed to the fresh
// editor by the caller.
func (s *settingsState) openModelEditor() tea.Cmd {
	placeholder := ""
	if len(s.pickerItems) > 0 {
		placeholder = s.pickerItems[min(max(s.pickerIndex, 0), len(s.pickerItems)-1)].ID
	}
	return s.openEditor(editSlotModel, s.pickerSlot, "", placeholder)
}

// saveSlotModel saves a model id — listed or free text — clearing an effort
// the new model does not take, the way the settings command drops it. A
// model the list does not know, or a list that cannot be built, is no
// verdict and clears nothing.
func (s *settingsState) saveSlotModel(slot, id string) {
	role := s.values.Roles[slot]
	role.Model = id
	notice := ""
	if role.Effort != "" && s.listTrusted(s.effectiveHarness(slot)) {
		if m, known := findSettingsModel(s.lists[s.effectiveHarness(slot)], id); known && !settingsContains(m.Efforts, role.Effort) {
			notice = fmt.Sprintf("model %q on %s does not take effort %q; cleared", id, s.effectiveHarness(slot), role.Effort)
			role.Effort = ""
		}
	}
	s.storeRole(slot, role, notice)
}

// commitEditor saves the open editor. A rejected threshold keeps the editor
// open so the value can be fixed.
func (s *settingsState) commitEditor() {
	switch s.editing {
	case editSlotModel:
		s.saveSlotModel(s.editSlot, strings.TrimSpace(s.editor.Value()))
		s.editing = editNone
	case editDecision:
		s.saveDecision(strings.TrimSpace(s.editor.Value()))
		s.editing = editNone
	case editThreshold:
		if s.saveThreshold(s.editor.Value()) {
			s.editing = editNone
		}
	}
}

// saveDecision saves the find decisions model, canonicalized the way the
// settings command stores it; empty clears back to the default.
func (s *settingsState) saveDecision(id string) {
	s.values.DecisionModel = systemone.CanonicalModel(id)
	s.saveValues("")
}

// saveThreshold saves find's passage cutoff. A value that is not a number
// above zero is a notice, not a save, and reports false.
func (s *settingsState) saveThreshold(raw string) bool {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || v <= 0 {
		s.notice = fmt.Sprintf("threshold %q is not a number above zero", strings.TrimSpace(raw))
		return false
	}
	s.values.Threshold = v
	s.saveValues("")
	return true
}

// saveValues persists the file through the shared save function, then
// re-reads it so the source labels match what was just saved instead of
// lagging a poll tick behind.
func (s *settingsState) saveValues(notice string) {
	if s.backend.Save == nil {
		return
	}
	if err := s.backend.Save(s.values); err != nil {
		s.notice = "save failed: " + err.Error()
		return
	}
	if s.backend.Load != nil {
		if v, err := s.backend.Load(); err == nil {
			s.values = v
		}
	}
	s.notice = notice
}

// cycleHarness moves a slot to the next harness, keeping the model and
// effort the new harness offers and resetting the rest with a notice.
func (s *settingsState) cycleHarness(slot string) {
	names := sortedHarnesses(s.harnesses)
	if len(names) == 0 {
		s.notice = "no harnesses known"
		return
	}
	next := names[0]
	for i, n := range names {
		if n == s.effectiveHarness(slot) {
			next = names[(i+1)%len(names)]
			break
		}
	}
	role, notice := carrySettingsRole(s.lists[next], s.listTrusted(next), slot, next, s.values.Roles[slot])
	s.storeRole(slot, role, notice)
}

// listTrusted reports whether a harness's cached list can judge values: a
// missing or empty list is no verdict, the way the settings command treats
// an unlistable harness.
func (s *settingsState) listTrusted(harness string) bool {
	if _, bad := s.listErr[harness]; bad {
		return false
	}
	return len(s.lists[harness]) > 0
}

// carrySettingsRole is the model and effort a slot keeps when its harness
// changes, mirroring the settings command: the ones the new harness
// offers, cleared otherwise with a notice; an unlistable harness keeps both
// with a warning.
func carrySettingsRole(list []wick.Model, trusted bool, slot, harness string, role tools.Role) (tools.Role, string) {
	out := tools.Role{Harness: harness}
	if role.Model == "" && role.Effort == "" {
		return out, ""
	}
	if !trusted {
		out.Model, out.Effort = role.Model, role.Effort
		return out, fmt.Sprintf("%s: cannot list %s models; kept model %q, effort %q", slot, harness, role.Model, role.Effort)
	}
	m, found := findSettingsModel(list, role.Model)
	if found {
		out.Model = role.Model
	}
	switch {
	case role.Effort == "":
	case found && settingsContains(m.Efforts, role.Effort):
		out.Effort = role.Effort
	case !found && role.Model == "" && settingsAnyOffers(list, role.Effort):
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
	return out, fmt.Sprintf("%s: %s does not offer %s; cleared", slot, harness, strings.Join(dropped, " and "))
}

// storeRole saves one slot through the shared save function.
func (s *settingsState) storeRole(slot string, role tools.Role, notice string) {
	if s.values.Roles == nil {
		s.values.Roles = map[string]tools.Role{}
	}
	s.values.Roles[slot] = role
	s.saveValues(notice)
}

// settingsContains reports whether a model's levels take an effort.
func settingsContains(levels []string, want string) bool {
	for _, l := range levels {
		if l == want {
			return true
		}
	}
	return false
}

// settingsAnyOffers reports whether any model of a harness takes an effort.
func settingsAnyOffers(list []wick.Model, effort string) bool {
	for _, m := range list {
		if settingsContains(m.Efforts, effort) {
			return true
		}
	}
	return false
}

// settingsHelp is the tab's help line for its sub-mode: the picker moves
// on arrows and sends text to free text, the editor saves or cancels.
func (m Model) settingsHelp() string {
	switch {
	case m.settings.editing != editNone:
		return "type • enter saves • esc cancels"
	case m.settings.pickerOpen:
		return "↑↓ move • enter picks • type free text • esc cancels"
	default:
		return "↑↓/k j row • ←→/h l field • enter change • r refresh models • tab tabs • q quit"
	}
}

// viewPicker renders the open model list under its slot: the cached ids
// with their effort levels, the highlight, and the free-text hint.
func (m Model) viewPicker() string {
	s := &m.settings
	var b strings.Builder
	fmt.Fprintf(&b, "  pick model for %s (enter picks, type free text, esc cancels):\n", s.pickerSlot)
	if len(s.pickerItems) == 0 {
		b.WriteString("  (no cached models — type free text)\n")
		return b.String()
	}
	for i, item := range s.pickerItems {
		mark := " "
		if i == s.pickerIndex {
			mark = ">"
		}
		line := mark + " " + item.ID
		if item.Label != "" && item.Label != item.ID {
			line += "  " + item.Label
		}
		if len(item.Efforts) > 0 {
			line += "  (" + strings.Join(item.Efforts, " ") + ")"
		}
		b.WriteString("  " + m.zone.Mark(settingsPickerZone(i), line) + "\n")
	}
	return b.String()
}

// capturing reports whether the tab holds the keyboard: an open editor or
// picker keeps tab and typing for itself.
func (s *settingsState) capturing() bool {
	return s.editing != editNone || s.pickerOpen
}
