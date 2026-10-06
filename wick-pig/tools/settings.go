package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Role is the default harness, model and effort for one launch slot. An empty
// field means "nothing chosen": the resolver falls back to the built-in
// default.
type Role struct {
	Harness string `json:"harness,omitempty"`
	Model   string `json:"model,omitempty"`
	Effort  string `json:"effort,omitempty"`
}

// RoleSlots are the launch slots a settings file may set, in print order.
var RoleSlots = []string{"orchestrator", "implementer", "code-review", "spec-review", "scout"}

// IsRoleSlot reports whether name names a launch slot.
func IsRoleSlot(name string) bool {
	for _, s := range RoleSlots {
		if s == name {
			return true
		}
	}
	return false
}

// DefaultThreshold is find's passage cutoff when nothing is stored.
const DefaultThreshold = 0.20

// DefaultOverflowThresholdBytes is the tool-output size above which output is
// stored in the overflow store instead of entering context: about 8KB.
const DefaultOverflowThresholdBytes = 8 * 1024

// DefaultMaxWorkers caps how many workers run at once when nothing is stored.
const DefaultMaxWorkers = 3

// DefaultCompressThreshold is the context size in tokens past which a
// session compresses when nothing is stored: the ticket's 200k.
const DefaultCompressThreshold = 200000

// DefaultCompressCooldown is the tool calls after a compress during which a
// second auto-compress is refused, when nothing is stored: a bad run cannot
// compress repeatedly, but a real session passes it quickly.
const DefaultCompressCooldown = 5

// PlacementMain serves a tool in the top-level tool list; PlacementExecute
// serves it only inside the execute tool.
const (
	PlacementMain    = "main"
	PlacementExecute = "execute"
)

// DefaultPlacement is where a tool serves when settings name no placement:
// the read-only toolkit (grep, git, check) lives inside execute, everything
// else in the main list.
func DefaultPlacement(name string) string {
	switch name {
	case "grep", "git", "check":
		return PlacementExecute
	default:
		return PlacementMain
	}
}

// GraphSetting tunes the optional code graph.
type GraphSetting struct {
	Enabled  *bool  `json:"enabled,omitempty"`
	Binary   string `json:"binary,omitempty"`
	CacheDir string `json:"cache_dir,omitempty"`
}

// ToolRoleOverride overrides a tool entry for one role. Only enabled and
// placement are per-role; the rename is global to every role.
type ToolRoleOverride struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Placement string `json:"placement,omitempty"`
}

// ToolSetting tunes one native tool. Enabled defaults to true when unset;
// placement defaults to DefaultPlacement. Roles overrides enabled and
// placement per role.
type ToolSetting struct {
	Enabled   *bool                       `json:"enabled,omitempty"`
	Placement string                      `json:"placement,omitempty"`
	Rename    string                      `json:"rename,omitempty"`
	Roles     map[string]ToolRoleOverride `json:"roles,omitempty"`
}

// MCPServerRoleOverride overrides an MCP server entry for one role.
type MCPServerRoleOverride struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Placement string `json:"placement,omitempty"`
}

// MCPServerSetting registers one outside MCP server. Include and exclude are
// glob lists over the server's own tool names (empty include means all;
// exclude wins); rename maps an original name to the name agents see.
// Placement defaults to execute; a per-role block overrides enabled and
// placement for that role. Summary is a short human line the execute tool
// description shows beside the server's tool count. There is no
// server-level enabled: remove the entry or exclude its tools.
type MCPServerSetting struct {
	Command   string                           `json:"command,omitempty"`
	Args      []string                         `json:"args,omitempty"`
	Env       map[string]string                `json:"env,omitempty"`
	Summary   string                           `json:"summary,omitempty"`
	Placement string                           `json:"placement,omitempty"`
	Include   []string                         `json:"include,omitempty"`
	Exclude   []string                         `json:"exclude,omitempty"`
	Rename    map[string]string                `json:"rename,omitempty"`
	Roles     map[string]MCPServerRoleOverride `json:"roles,omitempty"`
}

// OverflowSettings tunes the overflow store. ThresholdBytes is the output
// size in bytes above which a tool result is stored with a ctx: handle;
// zero means the default.
type OverflowSettings struct {
	ThresholdBytes int `json:"threshold_bytes,omitempty"`
}

// ContextSettings tunes context tracking and compression. CompressThreshold
// is the global auto-compress threshold in tokens; HarnessThresholds
// overrides it per harness; Windows overrides the model window table per
// model. StripRows is the header strip height (1 or 2, default 2);
// StripFrame draws the agent-colored frame beside the agent screen (nil
// means off); StripRule draws the agent-hue rule line above the strip
// instead of the soft half-block edge (nil means the soft edge);
// StripPadLeft, StripPadRight and StripPadBottom inset the agent screen
// and the strip by that many columns and rows (nil means the defaults
// below). Padding is background only: nothing is drawn in it.
// CompressModels names the summarizer model per harness (unset
// means compression refuses with a warning, never a silent default).
// AutoCompress disables the automatic trigger when false (nil means on; the
// manual key keeps working); CompressCooldown is the tool calls after a
// compress that block a second auto-compress (zero means the default). A
// pointer, so an unset context stays out of the saved file.
// Default padding around the wrapped agent screen: four columns left and
// right, one empty row below the strip. Zero restores the edge-to-edge
// layout.
const (
	DefaultPadLeft   = 4
	DefaultPadRight  = 4
	DefaultPadBottom = 2
)

type ContextSettings struct {
	CompressThreshold int64             `json:"compress_threshold,omitempty"`
	HarnessThresholds map[string]int64  `json:"harness_thresholds,omitempty"`
	Windows           map[string]int64  `json:"windows,omitempty"`
	StripRows         int               `json:"strip_rows,omitempty"`
	StripFrame        *bool             `json:"strip_frame,omitempty"`
	StripRule         *bool             `json:"strip_rule,omitempty"`
	StripPadLeft      *int              `json:"strip_pad_left,omitempty"`
	StripPadRight     *int              `json:"strip_pad_right,omitempty"`
	StripPadBottom    *int              `json:"strip_pad_bottom,omitempty"`
	CompressModels    map[string]string `json:"compress_models,omitempty"`
	AutoCompress      *bool             `json:"auto_compress,omitempty"`
	CompressCooldown  int               `json:"compress_cooldown,omitempty"`
}

// Settings are the user's choices that tools read at call time. They live in
// the user's wick-pig directory, beside the skills, so pig, the MCP server and
// the TUI all read the same file.
type Settings struct {
	// Search holds the websearch defaults: which provider answers and whether
	// providers run as a fallback chain or concurrently. Nil means the
	// defaults (auto provider, fallback mode); a pointer, so omitempty keeps
	// a settings file without search free of an empty object on save.
	Search *SearchSetting `json:"search,omitempty"`
	// DecisionModel is the OpenRouter decisions model find asks. Empty means
	// the pinned Jev.
	DecisionModel string `json:"decision_model,omitempty"`
	// Threshold is find's passage cutoff. Zero means 0.20.
	// ponytail: 0.20 is the value old wick measured against jev-1.13; a different
	// decisions model may need its own, so re-measure before trusting it.
	Threshold float64 `json:"threshold,omitempty"`
	// MaxWorkers caps how many workers run at once. Zero means the default.
	MaxWorkers int `json:"max_workers,omitempty"`
	// Overflow tunes the overflow store. Nil means the defaults; a set
	// ThresholdBytes of zero means the default too. A pointer, so an unset
	// overflow stays out of the saved file entirely.
	Overflow *OverflowSettings `json:"overflow,omitempty"`
	// Context tunes the compress threshold and the model window table. Nil
	// means the defaults.
	Context *ContextSettings `json:"context,omitempty"`
	// Roles holds one default per launch slot, keyed by slot name.
	Roles map[string]Role `json:"roles,omitempty"`
	// Tools tunes native tools by name: enabled, placement, rename and per-role
	// overrides. Absent means the defaults (enabled, DefaultPlacement).
	Tools map[string]ToolSetting `json:"tools,omitempty"`
	// MCPServers registers outside MCP servers by name.
	MCPServers map[string]MCPServerSetting `json:"mcp_servers,omitempty"`
	// Graph tunes the optional code graph binary.
	Graph GraphSetting `json:"graph,omitempty"`
	// extra holds top-level keys wick does not know yet, so a save writes them
	// back and the file can grow without a version bump. Never written from the
	// struct itself; SaveSettings merges it in.
	extra map[string]json.RawMessage
	// overflowExtra holds keys inside the overflow object that wick does not
	// know yet. SaveSettings merges them back, so new keys survive an old wick.
	overflowExtra map[string]json.RawMessage
	// roleExtra holds keys inside a role object that wick does not know yet,
	// keyed by slot then field. SaveSettings merges it into Roles.
	roleExtra map[string]map[string]json.RawMessage
	// searchExtra holds keys inside the search object that wick does not know
	// yet. SaveSettings merges it back, so new search keys survive an old wick.
	searchExtra map[string]json.RawMessage
	// contextExtra holds keys inside the context object that wick does not
	// know yet. SaveSettings merges them back, so new keys survive an old wick.
	contextExtra map[string]json.RawMessage
	// toolExtra holds unknown fields inside a tools entry, keyed by tool then
	// field; mcpExtra the same for an mcp_servers entry. toolRoleExtra holds
	// unknown fields inside a tool's per-role override, keyed by tool, role
	// then field; mcpRoleExtra the same for a server override. A save writes
	// them back, so new keys at any level survive an old wick.
	toolExtra     map[string]map[string]json.RawMessage
	mcpExtra      map[string]map[string]json.RawMessage
	toolRoleExtra map[string]map[string]map[string]json.RawMessage
	mcpRoleExtra  map[string]map[string]map[string]json.RawMessage
	// graphExtra holds unknown fields inside the graph object.
	graphExtra map[string]json.RawMessage
	// src names where a top-level value came from: "file", "env" or "default".
	// Print uses it to show a value's origin.
	src map[string]string
}

// Search vocabularies: the providers websearch may answer with, and whether
// providers run as a fallback chain or concurrently. The tool's provider
// argument takes the same provider values; there is no mode argument.
const (
	SearchProviderAuto  = "auto"
	SearchProviderBrave = "brave"
	SearchProviderExa   = "exa"
	SearchModeFallback  = "fallback"
	SearchModeParallel  = "parallel"
)

// SearchSetting is the search object in the settings file: the default
// provider behind websearch's provider argument, and the mode the providers
// run in. Empty fields mean the defaults.
type SearchSetting struct {
	Provider string `json:"provider,omitempty"`
	Mode     string `json:"mode,omitempty"`
}

// validSearchProvider reports whether p names a provider the tool accepts.
func validSearchProvider(p string) bool {
	switch p {
	case SearchProviderAuto, SearchProviderBrave, SearchProviderExa:
		return true
	}
	return false
}

// validSearchMode reports whether m names a mode providers run in.
func validSearchMode(m string) bool {
	switch m {
	case SearchModeFallback, SearchModeParallel:
		return true
	}
	return false
}

// EffectiveSearchProvider is the provider in force: the stored value, or auto
// when nothing valid is stored. An unknown value reads as the default, the
// way an unparsable threshold does.
func (s Settings) EffectiveSearchProvider() string {
	if s.Search != nil && validSearchProvider(s.Search.Provider) {
		return s.Search.Provider
	}
	return SearchProviderAuto
}

// EffectiveSearchMode is the mode in force: the stored value, or fallback
// when nothing valid is stored.
func (s Settings) EffectiveSearchMode() string {
	if s.Search != nil && validSearchMode(s.Search.Mode) {
		return s.Search.Mode
	}
	return SearchModeFallback
}

// searchSetting returns the search object, creating it when absent, so an
// edit never dereferences a nil pointer.
func (s *Settings) searchSetting() *SearchSetting {
	if s.Search == nil {
		s.Search = &SearchSetting{}
	}
	return s.Search
}

func (s Settings) GraphEnabled() bool {
	if s.Graph.Enabled == nil {
		return true
	}
	return *s.Graph.Enabled
}

// GraphBinaryPath is the explicit graph binary from settings, or "".
func (s Settings) GraphBinaryPath() string {
	return s.Graph.Binary
}

// GraphCacheDir is the graph cache directory override, or "".
func (s Settings) GraphCacheDir() string {
	return s.Graph.CacheDir
}

// SourceOf names where a top-level value came from: "file", "env" or
// "default".
func (s Settings) SourceOf(key string) string {
	if where, ok := s.src[key]; ok {
		return where
	}
	return "default"
}

// EffectiveMaxWorkers is the worker cap in force: the stored value, or three
// when nothing valid is stored.
func (s Settings) EffectiveMaxWorkers() int {
	if s.MaxWorkers > 0 {
		return s.MaxWorkers
	}
	return DefaultMaxWorkers
}

// EffectiveOverflowThresholdBytes is the output size in bytes above which a
// tool result is stored in the overflow store: the stored value, or about
// 8KB when nothing valid is stored.
func (s Settings) EffectiveOverflowThresholdBytes() int {
	if s.Overflow != nil && s.Overflow.ThresholdBytes > 0 {
		return s.Overflow.ThresholdBytes
	}
	return DefaultOverflowThresholdBytes
}

// contextSetting returns the context object, creating it when absent, so an
// edit never dereferences a nil pointer.
func (s *Settings) contextSetting() *ContextSettings {
	if s.Context == nil {
		s.Context = &ContextSettings{}
	}
	return s.Context
}

// EffectiveCompressThreshold is the auto-compress threshold in force for a
// harness: the per-harness override, else the global value, else 200k. A
// non-positive value anywhere reads as the default.
func (s Settings) EffectiveCompressThreshold(harness string) int64 {
	if s.Context != nil {
		// The environment names one global value; it wins over every file
		// layer, including a per-harness override.
		if s.Context.CompressThreshold > 0 && s.src["context.compress_threshold"] == "env" {
			return s.Context.CompressThreshold
		}
		if v := s.Context.HarnessThresholds[harness]; v > 0 {
			return v
		}
		if s.Context.CompressThreshold > 0 {
			return s.Context.CompressThreshold
		}
	}
	return DefaultCompressThreshold
}

// EffectiveCompressModel is the summarizer model in force for a harness:
// the per-harness value, or "" when none is set. Empty means compression
// refuses and the header warns; there is no default.
func (s Settings) EffectiveCompressModel(harness string) string {
	if s.Context != nil {
		return s.Context.CompressModels[harness]
	}
	return ""
}

// EffectiveAutoCompress is the automatic trigger in force: on unless the
// file says otherwise. Off stops auto-compress only; the manual key keeps
// working.
func (s Settings) EffectiveAutoCompress() bool {
	if s.Context != nil && s.Context.AutoCompress != nil {
		return *s.Context.AutoCompress
	}
	return true
}

// EffectiveCompressCooldown is the tool calls after a compress that block a
// second auto-compress, in force: the stored value, or the default when
// nothing positive is stored.
func (s Settings) EffectiveCompressCooldown() int {
	if s.Context != nil && s.Context.CompressCooldown > 0 {
		return s.Context.CompressCooldown
	}
	return DefaultCompressCooldown
}

// CompressModels is the summarizer model per harness. The map is a copy:
// the caller cannot mutate the settings through it.
func (s Settings) CompressModels() map[string]string {
	out := map[string]string{}
	if s.Context != nil {
		for h, m := range s.Context.CompressModels {
			out[h] = m
		}
	}
	return out
}

// ContextWindows is the model window overrides in force, window tokens by
// model. The map is a copy: the caller cannot mutate the settings through it.
func (s Settings) ContextWindows() map[string]int64 {
	out := map[string]int64{}
	if s.Context != nil {
		for m, w := range s.Context.Windows {
			out[m] = w
		}
	}
	return out
}

// EffectiveStripRows is the header strip height in force: 1 or 2, default
// 2. Anything else reads as the default.
func (s Settings) EffectiveStripRows() int {
	if s.Context != nil && (s.Context.StripRows == 1 || s.Context.StripRows == 2) {
		return s.Context.StripRows
	}
	return 2
}

// StripFrameEnabled says whether the wrapper draws the agent-colored frame
// beside the agent screen: off unless the setting says otherwise (the
// borderless look separates areas by background tint, not lines).
func (s Settings) StripFrameEnabled() bool {
	if s.Context != nil && s.Context.StripFrame != nil {
		return *s.Context.StripFrame
	}
	return false
}

// StripRuleEnabled says whether the wrapper draws the agent-hue rule line
// above the strip: off unless the setting restores it (the borderless look
// keeps the soft half-block edge, separating areas by tint, not lines).
func (s Settings) StripRuleEnabled() bool {
	if s.Context != nil && s.Context.StripRule != nil {
		return *s.Context.StripRule
	}
	return false
}

// EffectivePadLeft is the left padding columns in force: the default two
// when unset, an explicit zero restoring the edge-to-edge layout, a
// negative reading as zero.
func (s Settings) EffectivePadLeft() int {
	if s.Context != nil && s.Context.StripPadLeft != nil {
		if *s.Context.StripPadLeft < 0 {
			return 0
		}
		return *s.Context.StripPadLeft
	}
	return DefaultPadLeft
}

// EffectivePadRight is the right padding columns in force, like the left.
func (s Settings) EffectivePadRight() int {
	if s.Context != nil && s.Context.StripPadRight != nil {
		if *s.Context.StripPadRight < 0 {
			return 0
		}
		return *s.Context.StripPadRight
	}
	return DefaultPadRight
}

// EffectivePadBottom is the bottom padding rows in force: the default one
// when unset, zero restoring today's layout.
func (s Settings) EffectivePadBottom() int {
	if s.Context != nil && s.Context.StripPadBottom != nil {
		if *s.Context.StripPadBottom < 0 {
			return 0
		}
		return *s.Context.StripPadBottom
	}
	return DefaultPadBottom
}

// unknownContextFields collects the keys inside the context object that are
// not the known context settings, so a save writes them back.
func unknownContextFields(rawContext json.RawMessage) map[string]json.RawMessage {
	var contextFields map[string]json.RawMessage
	if err := json.Unmarshal(rawContext, &contextFields); err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	for k, v := range contextFields {
		switch strings.ToLower(k) {
		case "compress_threshold", "harness_thresholds", "windows", "strip_rows", "strip_frame", "strip_rule", "strip_pad_left", "strip_pad_right", "strip_pad_bottom", "compress_models", "auto_compress", "compress_cooldown":
			continue
		}
		if out == nil {
			out = map[string]json.RawMessage{}
		}
		out[k] = v
	}
	return out
}

// mergeContextExtras writes the unknown fields of the context object back
// into the object already in fields.
func mergeContextExtras(fields map[string]json.RawMessage, contextExtra map[string]json.RawMessage) error {
	if len(contextExtra) == 0 {
		return nil
	}
	context := map[string]json.RawMessage{}
	if raw, ok := fields["context"]; ok {
		if err := json.Unmarshal(raw, &context); err != nil {
			return err
		}
	}
	for k, v := range contextExtra {
		if _, ok := context[k]; !ok {
			context[k] = v
		}
	}
	raw, err := json.Marshal(context)
	if err != nil {
		return err
	}
	fields["context"] = raw
	return nil
}

// mergeContext folds project context settings over global ones: scalars win
// when set, maps merge by key with the project winning each key. A value the
// environment set always wins over the project file.
func mergeContext(global, project *ContextSettings, src map[string]string) *ContextSettings {
	var out ContextSettings
	if global != nil {
		out = *global
		out.HarnessThresholds = cloneInt64Map(global.HarnessThresholds)
		out.Windows = cloneInt64Map(global.Windows)
		out.CompressModels = cloneStringMap(global.CompressModels)
		if global.StripFrame != nil {
			v := *global.StripFrame
			out.StripFrame = &v
		}
		if global.StripRule != nil {
			v := *global.StripRule
			out.StripRule = &v
		}
		if global.StripPadLeft != nil {
			v := *global.StripPadLeft
			out.StripPadLeft = &v
		}
		if global.StripPadRight != nil {
			v := *global.StripPadRight
			out.StripPadRight = &v
		}
		if global.StripPadBottom != nil {
			v := *global.StripPadBottom
			out.StripPadBottom = &v
		}
		if global.AutoCompress != nil {
			v := *global.AutoCompress
			out.AutoCompress = &v
		}
	}
	if project != nil {
		if project.CompressThreshold > 0 && src["context.compress_threshold"] != "env" {
			out.CompressThreshold = project.CompressThreshold
			src["context.compress_threshold"] = "project"
		}
		for h, v := range project.HarnessThresholds {
			if v <= 0 || src["context.harness_thresholds."+h] == "env" {
				continue
			}
			if out.HarnessThresholds == nil {
				out.HarnessThresholds = map[string]int64{}
			}
			out.HarnessThresholds[h] = v
			src["context.harness_thresholds."+h] = "project"
		}
		for m, w := range project.Windows {
			if w <= 0 {
				continue
			}
			if out.Windows == nil {
				out.Windows = map[string]int64{}
			}
			out.Windows[m] = w
			src["context.windows"] = "project"
		}
		if project.StripRows != 0 {
			out.StripRows = project.StripRows
			src["context.strip_rows"] = "project"
		}
		if project.StripFrame != nil {
			v := *project.StripFrame
			out.StripFrame = &v
			src["context.strip_frame"] = "project"
		}
		if project.StripRule != nil {
			v := *project.StripRule
			out.StripRule = &v
			src["context.strip_rule"] = "project"
		}
		if project.StripPadLeft != nil {
			v := *project.StripPadLeft
			out.StripPadLeft = &v
			src["context.strip_pad_left"] = "project"
		}
		if project.StripPadRight != nil {
			v := *project.StripPadRight
			out.StripPadRight = &v
			src["context.strip_pad_right"] = "project"
		}
		if project.StripPadBottom != nil {
			v := *project.StripPadBottom
			out.StripPadBottom = &v
			src["context.strip_pad_bottom"] = "project"
		}
		for h, m := range project.CompressModels {
			if m == "" {
				continue
			}
			if out.CompressModels == nil {
				out.CompressModels = map[string]string{}
			}
			out.CompressModels[h] = m
			src["context.compress_models."+h] = "project"
		}
		if project.AutoCompress != nil {
			v := *project.AutoCompress
			out.AutoCompress = &v
			src["context.auto_compress"] = "project"
		}
		if project.CompressCooldown > 0 {
			out.CompressCooldown = project.CompressCooldown
			src["context.compress_cooldown"] = "project"
		}
	}
	if out.CompressThreshold == 0 && len(out.HarnessThresholds) == 0 && len(out.Windows) == 0 &&
		out.StripRows == 0 && out.StripFrame == nil && out.StripRule == nil && out.StripPadLeft == nil && out.StripPadRight == nil && out.StripPadBottom == nil && len(out.CompressModels) == 0 &&
		out.AutoCompress == nil && out.CompressCooldown == 0 {
		return nil
	}
	return &out
}

// cloneStringMap copies a string-to-string map, so a merge never shares the
// map with its inputs.
func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// cloneInt64Map copies a string-to-int64 map, so a merge never shares the
// map with its inputs.
func cloneInt64Map(m map[string]int64) map[string]int64 {
	if m == nil {
		return nil
	}
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// SettingsPath is where settings are saved. WICK_SETTINGS overrides it for
// tests and for a second profile.
func SettingsPath() string {
	if p := os.Getenv("WICK_SETTINGS"); p != "" {
		return p
	}
	return filepath.Join(os.Getenv("HOME"), ".wick-pig", "settings.json")
}

// oldSettingsPath is the state-dir file settings lived in before the move
// beside the skills. It is read, never written, so an upgrade loses nothing.
func oldSettingsPath() string {
	return filepath.Join(os.Getenv("HOME"), ".pig", "state", "wick", "settings.json")
}

// LoadSettings reads the file; a missing or unreadable file is the defaults.
// When WICK_SETTINGS is unset and the new file is absent, the old state-dir
// file is read instead. WICK_DECISION_MODEL, WICK_FIND_THRESHOLD,
// WICK_MAX_WORKERS, WICK_SEARCH_PROVIDER, WICK_SEARCH_MODE and
// WICK_OVERFLOW_THRESHOLD_BYTES win over the file.
func LoadSettings() Settings {
	var s Settings
	if raw, err := os.ReadFile(SettingsPath()); err == nil {
		s = decodeSettings(raw)
	} else if os.Getenv("WICK_SETTINGS") == "" {
		// An explicit override is the whole world: it must not quietly read the
		// machine's old file too, or a hermetic test reads the developer's data.
		if raw, err := os.ReadFile(oldSettingsPath()); err == nil {
			s = decodeSettings(raw)
		}
	}
	if m := os.Getenv("WICK_DECISION_MODEL"); m != "" {
		s.DecisionModel = m
		s.setSource("decision_model", "env")
	}
	if t, err := strconv.ParseFloat(os.Getenv("WICK_FIND_THRESHOLD"), 64); err == nil && t > 0 {
		s.Threshold = t
		s.setSource("threshold", "env")
	}
	if w, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WICK_MAX_WORKERS"))); err == nil && w > 0 {
		s.MaxWorkers = w
		s.setSource("max_workers", "env")
	}
	if p := strings.ToLower(strings.TrimSpace(os.Getenv("WICK_SEARCH_PROVIDER"))); validSearchProvider(p) {
		s.searchSetting().Provider = p
		s.setSource("search.provider", "env")
	}
	if m := strings.ToLower(strings.TrimSpace(os.Getenv("WICK_SEARCH_MODE"))); validSearchMode(m) {
		s.searchSetting().Mode = m
		s.setSource("search.mode", "env")
	}
	if b, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WICK_OVERFLOW_THRESHOLD_BYTES"))); err == nil && b > 0 {
		s.Overflow = &OverflowSettings{ThresholdBytes: b}
		s.setSource("overflow.threshold_bytes", "env")
	}
	if b, err := strconv.Atoi(strings.TrimSpace(os.Getenv("WICK_COMPRESS_THRESHOLD"))); err == nil && b > 0 {
		s.contextSetting().CompressThreshold = int64(b)
		s.setSource("context.compress_threshold", "env")
	}
	return s
}

// decodeSettings parses one settings file. Garbage is the defaults; the known
// keys fill the struct and the rest are kept for the next save.
func decodeSettings(raw []byte) Settings {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Settings{}
	}
	var s Settings
	_ = json.Unmarshal(raw, &s)
	s.src = map[string]string{}
	s.extra = map[string]json.RawMessage{}
	for k, v := range fields {
		// encoding/json matches known fields case-insensitively, so this
		// comparison is case-insensitive too: a case variant of a known key is
		// the known key, not an unknown one kept alongside it.
		switch strings.ToLower(k) {
		case "roles":
			s.roleExtra = unknownRoleFields(v)
		case "decision_model", "threshold", "max_workers":
		case "search":
			s.searchExtra = unknownSearchFields(v)
		case "overflow":
			s.overflowExtra = unknownOverflowFields(v)
		case "context":
			s.contextExtra = unknownContextFields(v)
		case "tools":
			s.toolExtra = unknownEntryFields(v, []string{"enabled", "placement", "rename", "roles"})
			s.toolRoleExtra = unknownOverrideFields(v)
		case "mcp_servers":
			s.mcpExtra = unknownEntryFields(v, []string{"command", "args", "env", "summary", "placement", "include", "exclude", "rename", "roles"})
			s.mcpRoleExtra = unknownOverrideFields(v)
		case "graph":
			s.graphExtra = unknownGraphFields(v)
		default:
			s.extra[k] = v
		}
	}
	// A value that failed to parse or is empty stays the default, and the
	// source has to say so: the value and the name it prints must agree.
	if s.Search != nil {
		if s.Search.Provider != "" {
			s.src["search.provider"] = "file"
		}
		if s.Search.Mode != "" {
			s.src["search.mode"] = "file"
		}
	}
	if s.DecisionModel != "" {
		s.src["decision_model"] = "file"
	}
	if s.Threshold != 0 {
		s.src["threshold"] = "file"
	}
	if s.MaxWorkers != 0 {
		s.src["max_workers"] = "file"
	}
	if s.Overflow != nil && s.Overflow.ThresholdBytes != 0 {
		s.src["overflow.threshold_bytes"] = "file"
	}
	for slot, r := range s.Roles {
		base := "roles." + slot + "."
		if r.Harness != "" {
			s.src[base+"harness"] = "file"
		}
		if r.Model != "" {
			s.src[base+"model"] = "file"
		}
		if r.Effort != "" {
			s.src[base+"effort"] = "file"
		}
	}
	if s.Context != nil {
		if s.Context.CompressThreshold != 0 {
			s.src["context.compress_threshold"] = "file"
		}
		for h := range s.Context.HarnessThresholds {
			s.src["context.harness_thresholds."+h] = "file"
		}
		for h := range s.Context.CompressModels {
			s.src["context.compress_models."+h] = "file"
		}
		if len(s.Context.Windows) > 0 {
			s.src["context.windows"] = "file"
		}
		if s.Context.StripRows != 0 {
			s.src["context.strip_rows"] = "file"
		}
		if s.Context.StripFrame != nil {
			s.src["context.strip_frame"] = "file"
		}
		if s.Context.StripRule != nil {
			s.src["context.strip_rule"] = "file"
		}
		if s.Context.AutoCompress != nil {
			s.src["context.auto_compress"] = "file"
		}
		if s.Context.CompressCooldown != 0 {
			s.src["context.compress_cooldown"] = "file"
		}
	}
	markToolSources(&s)
	return s
}

// markToolSources records where each tool and server field came from, so the
// settings command can print a source per value. Keys are dotted paths like
// "tools.bash.enabled" or "mcp_servers.github.placement".
func markToolSources(s *Settings) {
	if len(s.Tools) > 0 {
		s.src["tools"] = "file"
	}
	for name, e := range s.Tools {
		base := "tools." + name + "."
		if e.Enabled != nil {
			s.src[base+"enabled"] = "file"
		}
		if e.Placement != "" {
			s.src[base+"placement"] = "file"
		}
		if e.Rename != "" {
			s.src[base+"rename"] = "file"
		}
		for role, ov := range e.Roles {
			rbase := base + "roles." + role + "."
			if ov.Enabled != nil {
				s.src[rbase+"enabled"] = "file"
			}
			if ov.Placement != "" {
				s.src[rbase+"placement"] = "file"
			}
		}
	}
	if len(s.MCPServers) > 0 {
		s.src["mcp_servers"] = "file"
	}
	for name, e := range s.MCPServers {
		base := "mcp_servers." + name + "."
		for _, f := range []struct {
			key string
			set bool
		}{
			{"command", e.Command != ""},
			{"args", e.Args != nil},
			{"env", e.Env != nil},
			{"summary", e.Summary != ""},
			{"placement", e.Placement != ""},
			{"include", e.Include != nil},
			{"exclude", e.Exclude != nil},
			{"rename", e.Rename != nil},
		} {
			if f.set {
				s.src[base+f.key] = "file"
			}
		}
		for role, ov := range e.Roles {
			rbase := base + "roles." + role + "."
			if ov.Enabled != nil {
				s.src[rbase+"enabled"] = "file"
			}
			if ov.Placement != "" {
				s.src[rbase+"placement"] = "file"
			}
		}
	}
	if s.Graph.Enabled != nil {
		s.src["graph.enabled"] = "file"
	}
	if s.Graph.Binary != "" {
		s.src["graph.binary"] = "file"
	}
	if s.Graph.CacheDir != "" {
		s.src["graph.cache_dir"] = "file"
	}
}

// unknownRoleFields collects, per slot, the keys inside a role object that are
// not harness, model or effort, so a save writes them back.
func unknownRoleFields(rawRoles json.RawMessage) map[string]map[string]json.RawMessage {
	var perSlot map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rawRoles, &perSlot); err != nil {
		return nil
	}
	var out map[string]map[string]json.RawMessage
	for slot, roleFields := range perSlot {
		for k, v := range roleFields {
			switch strings.ToLower(k) {
			case "harness", "model", "effort":
			default:
				if out == nil {
					out = map[string]map[string]json.RawMessage{}
				}
				if out[slot] == nil {
					out[slot] = map[string]json.RawMessage{}
				}
				out[slot][k] = v
			}
		}
	}
	return out
}

// unknownSearchFields collects the keys inside a search object that are not
// provider or mode, so a save writes them back.
func unknownSearchFields(rawSearch json.RawMessage) map[string]json.RawMessage {
	var searchFields map[string]json.RawMessage
	if err := json.Unmarshal(rawSearch, &searchFields); err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	for k, v := range searchFields {
		switch strings.ToLower(k) {
		case "provider", "mode":
		default:
			if out == nil {
				out = map[string]json.RawMessage{}
			}
			out[k] = v
		}
	}
	return out
}

// unknownGraphFields collects the keys inside a graph object that are not
// enabled, binary or cache_dir, so a save writes them back.
func unknownGraphFields(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	for k, v := range fields {
		switch strings.ToLower(k) {
		case "enabled", "binary", "cache_dir":
		default:
			if out == nil {
				out = map[string]json.RawMessage{}
			}
			out[k] = v
		}
	}
	return out
}

// mergeSearch folds the project search object over the global one, field by
// field, recording a project source for every field the project set. A value
// the environment set always wins over the project file. Either side may be
// nil; the result is nil only when both are.
func mergeSearch(global, project *SearchSetting, src map[string]string) *SearchSetting {
	var out SearchSetting
	if global != nil {
		out = *global
	}
	if project != nil {
		if project.Provider != "" && src["search.provider"] != "env" {
			out.Provider = project.Provider
			src["search.provider"] = "project"
		}
		if project.Mode != "" && src["search.mode"] != "env" {
			out.Mode = project.Mode
			src["search.mode"] = "project"
		}
	}
	if out == (SearchSetting{}) {
		return nil
	}
	return &out
}

// cloneSearchExtra copies the unknown search fields, so a merge never shares
// the map with its inputs.
func cloneSearchExtra(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return map[string]json.RawMessage{}
	}
	return cloneRawMap(m)
}

// mergeSearchExtras writes the unknown fields of the search object back into
// the object already in fields.
func mergeSearchExtras(fields map[string]json.RawMessage, searchExtra map[string]json.RawMessage) error {
	if len(searchExtra) == 0 {
		return nil
	}
	search := map[string]json.RawMessage{}
	if raw, ok := fields["search"]; ok {
		if err := json.Unmarshal(raw, &search); err != nil {
			return err
		}
	}
	for k, v := range searchExtra {
		if _, ok := search[k]; !ok {
			search[k] = v
		}
	}
	raw, err := json.Marshal(search)
	if err != nil {
		return err
	}
	fields["search"] = raw
	return nil
}

func (s *Settings) setSource(key, where string) {
	if s.src == nil {
		s.src = map[string]string{}
	}
	s.src[key] = where
}

// RemoveMCPServer drops one server registration. Unknown fields ride along,
// or the next save would resurrect the entry from the extras as a
// command-less server.
func (s *Settings) RemoveMCPServer(name string) {
	delete(s.MCPServers, name)
	delete(s.mcpExtra, name)
	delete(s.mcpRoleExtra, name)
}

// SaveSettings writes the file, creating its directory. Keys wick does not
// know survive the round trip, at the top level, inside a role object and
// inside a tools or mcp_servers entry. It writes the global file: call it
// with global settings, never merged ones, or project values leak upward.
func SaveSettings(s Settings) error {
	known, err := json.Marshal(s)
	if err != nil {
		return err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(known, &fields); err != nil {
		return err
	}
	for k, v := range s.extra {
		if _, ok := fields[k]; !ok {
			fields[k] = v
		}
	}
	if err := mergeRoleExtras(fields, s.roleExtra); err != nil {
		return err
	}
	if err := mergeSearchExtras(fields, s.searchExtra); err != nil {
		return err
	}
	if err := mergeOverflowExtras(fields, s.overflowExtra); err != nil {
		return err
	}
	if err := mergeContextExtras(fields, s.contextExtra); err != nil {
		return err
	}
	if err := mergeEntryExtras(fields, "tools", s.toolExtra); err != nil {
		return err
	}
	if err := mergeEntryExtras(fields, "mcp_servers", s.mcpExtra); err != nil {
		return err
	}
	if err := mergeOverrideExtras(fields, "tools", s.toolRoleExtra); err != nil {
		return err
	}
	if err := mergeOverrideExtras(fields, "mcp_servers", s.mcpRoleExtra); err != nil {
		return err
	}
	if err := mergeGraphExtras(fields, s.graphExtra); err != nil {
		return err
	}
	// A set-but-empty context pointer marshals as {} because omitempty does
	// not omit structs. Drop the key when nothing context-related is set, so
	// clearing the last override removes the object instead of leaving {}.
	if s.Context != nil && s.Context.CompressThreshold == 0 && len(s.Context.HarnessThresholds) == 0 && len(s.Context.Windows) == 0 && s.Context.StripRows == 0 && s.Context.StripFrame == nil && s.Context.StripRule == nil && len(s.Context.CompressModels) == 0 && s.Context.AutoCompress == nil && s.Context.CompressCooldown == 0 && len(s.contextExtra) == 0 {
		delete(fields, "context")
	}
	// A zero graph struct marshals as {} because encoding/json omitempty does
	// not omit structs. Drop the key when nothing graph-related is set, so
	// files that never had graph stay without it.
	if s.Graph == (GraphSetting{}) && len(s.graphExtra) == 0 {
		delete(fields, "graph")
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(SettingsPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(SettingsPath(), append(raw, '\n'), 0o644)
}

// unknownOverflowFields collects the keys inside the overflow object that are
// not threshold_bytes, so a save writes them back.
func unknownOverflowFields(rawOverflow json.RawMessage) map[string]json.RawMessage {
	var overflowFields map[string]json.RawMessage
	if err := json.Unmarshal(rawOverflow, &overflowFields); err != nil {
		return nil
	}
	var out map[string]json.RawMessage
	for k, v := range overflowFields {
		if strings.ToLower(k) == "threshold_bytes" {
			continue
		}
		if out == nil {
			out = map[string]json.RawMessage{}
		}
		out[k] = v
	}
	return out
}

// mergeOverflowExtras writes the unknown fields of the overflow object back
// into the object already in fields.
func mergeOverflowExtras(fields map[string]json.RawMessage, overflowExtra map[string]json.RawMessage) error {
	if len(overflowExtra) == 0 {
		return nil
	}
	overflow := map[string]json.RawMessage{}
	if raw, ok := fields["overflow"]; ok {
		if err := json.Unmarshal(raw, &overflow); err != nil {
			return err
		}
	}
	for k, v := range overflowExtra {
		if _, ok := overflow[k]; !ok {
			overflow[k] = v
		}
	}
	raw, err := json.Marshal(overflow)
	if err != nil {
		return err
	}
	fields["overflow"] = raw
	return nil
}

// mergeRoleExtras writes the unknown fields of each role back into the roles
// object already in fields.
func mergeRoleExtras(fields map[string]json.RawMessage, roleExtra map[string]map[string]json.RawMessage) error {
	if len(roleExtra) == 0 {
		return nil
	}
	roles := map[string]map[string]json.RawMessage{}
	if raw, ok := fields["roles"]; ok {
		if err := json.Unmarshal(raw, &roles); err != nil {
			return err
		}
	}
	for slot, extras := range roleExtra {
		if roles[slot] == nil {
			roles[slot] = map[string]json.RawMessage{}
		}
		for k, v := range extras {
			if _, ok := roles[slot][k]; !ok {
				roles[slot][k] = v
			}
		}
	}
	raw, err := json.Marshal(roles)
	if err != nil {
		return err
	}
	fields["roles"] = raw
	return nil
}

// mergeGraphExtras writes the unknown fields of the graph object back.
func mergeGraphExtras(fields map[string]json.RawMessage, graphExtra map[string]json.RawMessage) error {
	if len(graphExtra) == 0 {
		return nil
	}
	graph := map[string]json.RawMessage{}
	if raw, ok := fields["graph"]; ok {
		if err := json.Unmarshal(raw, &graph); err != nil {
			return err
		}
	}
	for k, v := range graphExtra {
		if _, ok := graph[k]; !ok {
			graph[k] = v
		}
	}
	raw, err := json.Marshal(graph)
	if err != nil {
		return err
	}
	fields["graph"] = raw
	return nil
}

// unknownEntryFields collects, per entry of a tools or mcp_servers object,
// the fields that are not in known, so a save writes them back.
func unknownEntryFields(rawEntries json.RawMessage, known []string) map[string]map[string]json.RawMessage {
	knownSet := map[string]bool{}
	for _, k := range known {
		knownSet[k] = true
	}
	var perEntry map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rawEntries, &perEntry); err != nil {
		return nil
	}
	var out map[string]map[string]json.RawMessage
	for entry, entryFields := range perEntry {
		for k, v := range entryFields {
			if knownSet[strings.ToLower(k)] {
				continue
			}
			if out == nil {
				out = map[string]map[string]json.RawMessage{}
			}
			if out[entry] == nil {
				out[entry] = map[string]json.RawMessage{}
			}
			out[entry][k] = v
		}
	}
	return out
}

// mergeEntryExtras writes the unknown fields of each tools or mcp_servers
// entry back into the object already in fields under key.
func mergeEntryExtras(fields map[string]json.RawMessage, key string, entryExtra map[string]map[string]json.RawMessage) error {
	if len(entryExtra) == 0 {
		return nil
	}
	entries := map[string]map[string]json.RawMessage{}
	if raw, ok := fields[key]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return err
		}
	}
	for entry, extras := range entryExtra {
		if entries[entry] == nil {
			entries[entry] = map[string]json.RawMessage{}
		}
		for k, v := range extras {
			if _, ok := entries[entry][k]; !ok {
				entries[entry][k] = v
			}
		}
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	fields[key] = raw
	return nil
}

// unknownOverrideFields collects, per entry and role of a tools or
// mcp_servers object, the override fields that are not enabled or placement,
// so a save writes them back.
func unknownOverrideFields(rawEntries json.RawMessage) map[string]map[string]map[string]json.RawMessage {
	var perEntry map[string]map[string]json.RawMessage
	if err := json.Unmarshal(rawEntries, &perEntry); err != nil {
		return nil
	}
	var out map[string]map[string]map[string]json.RawMessage
	for entry, entryFields := range perEntry {
		var rawRoles json.RawMessage
		for k, v := range entryFields {
			if strings.ToLower(k) == "roles" {
				rawRoles = v
			}
		}
		if rawRoles == nil {
			continue
		}
		var perRole map[string]map[string]json.RawMessage
		if err := json.Unmarshal(rawRoles, &perRole); err != nil {
			continue
		}
		for role, roleFields := range perRole {
			for k, v := range roleFields {
				switch strings.ToLower(k) {
				case "enabled", "placement":
				default:
					if out == nil {
						out = map[string]map[string]map[string]json.RawMessage{}
					}
					if out[entry] == nil {
						out[entry] = map[string]map[string]json.RawMessage{}
					}
					if out[entry][role] == nil {
						out[entry][role] = map[string]json.RawMessage{}
					}
					out[entry][role][k] = v
				}
			}
		}
	}
	return out
}

// mergeOverrideExtras writes the unknown fields of each per-role override
// back into the tools or mcp_servers object already in fields under key.
func mergeOverrideExtras(fields map[string]json.RawMessage, key string, overrideExtra map[string]map[string]map[string]json.RawMessage) error {
	if len(overrideExtra) == 0 {
		return nil
	}
	entries := map[string]map[string]json.RawMessage{}
	if raw, ok := fields[key]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return err
		}
	}
	for entry, perRole := range overrideExtra {
		if entries[entry] == nil {
			entries[entry] = map[string]json.RawMessage{}
		}
		roles := map[string]map[string]json.RawMessage{}
		if raw, ok := entries[entry]["roles"]; ok {
			if err := json.Unmarshal(raw, &roles); err != nil {
				return err
			}
		}
		for role, extras := range perRole {
			if roles[role] == nil {
				roles[role] = map[string]json.RawMessage{}
			}
			for k, v := range extras {
				if _, ok := roles[role][k]; !ok {
					roles[role][k] = v
				}
			}
		}
		raw, err := json.Marshal(roles)
		if err != nil {
			return err
		}
		entries[entry]["roles"] = raw
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	fields[key] = raw
	return nil
}

// ProjectSettingsPath is the per-project settings file under root.
// WICK_PROJECT_SETTINGS overrides it, so tests can point it at a temp file.
func ProjectSettingsPath(root string) string {
	if p := os.Getenv("WICK_PROJECT_SETTINGS"); p != "" {
		return p
	}
	return filepath.Join(root, filepath.FromSlash(ProjectSettingsRel))
}

// LoadProjectSettings reads the per-project file under root. A missing or
// unreadable file is empty settings: the project layer is optional.
func LoadProjectSettings(root string) Settings {
	raw, err := os.ReadFile(ProjectSettingsPath(root))
	if err != nil {
		return Settings{}
	}
	return decodeSettings(raw)
}

// EffectiveSettings merges the global file with the project file under root.
// The project wins field by field and maps merge by key; the process
// environment still wins over both. SaveSettings writes the global file only,
// so read the effective settings for serving and the global ones for editing.
func EffectiveSettings(root string) Settings {
	return MergeSettings(LoadSettings(), LoadProjectSettings(root))
}

// MergeSettings folds project over global: scalars and list fields win when
// set, maps merge by key with the project winning each field, and unknown keys
// merge by key too. A value the environment set (tracked in the source map)
// always wins over the project file.
func MergeSettings(global, project Settings) Settings {
	out := global
	out.src = cloneSources(global.src)
	out.extra = cloneRawMap(global.extra)
	out.overflowExtra = cloneRawMap(global.overflowExtra)
	out.contextExtra = cloneRawMap(global.contextExtra)
	out.roleExtra = cloneNestedRawMap(global.roleExtra)
	out.toolExtra = cloneNestedRawMap(global.toolExtra)
	out.mcpExtra = cloneNestedRawMap(global.mcpExtra)
	out.toolRoleExtra = cloneDeepNestedRawMap(global.toolRoleExtra)
	out.graphExtra = cloneRawMap(global.graphExtra)
	out.mcpRoleExtra = cloneDeepNestedRawMap(global.mcpRoleExtra)
	if out.src == nil {
		out.src = map[string]string{}
	}
	if project.DecisionModel != "" && out.src["decision_model"] != "env" {
		out.DecisionModel = project.DecisionModel
		out.src["decision_model"] = "project"
	}
	if project.Threshold != 0 && out.src["threshold"] != "env" {
		out.Threshold = project.Threshold
		out.src["threshold"] = "project"
	}
	if project.MaxWorkers != 0 && out.src["max_workers"] != "env" {
		out.MaxWorkers = project.MaxWorkers
		out.src["max_workers"] = "project"
	}
	if project.Overflow != nil && project.Overflow.ThresholdBytes != 0 && out.src["overflow.threshold_bytes"] != "env" {
		if out.Overflow == nil {
			// A copy, never the caller's struct: MergeSettings must not
			// mutate the global settings through the shared pointer.
			out.Overflow = &OverflowSettings{}
		} else {
			v := *out.Overflow
			out.Overflow = &v
		}
		out.Overflow.ThresholdBytes = project.Overflow.ThresholdBytes
		out.src["overflow.threshold_bytes"] = "project"
	}
	out.Context = mergeContext(global.Context, project.Context, out.src)
	out.Roles = mergeRoles(global.Roles, project.Roles, out.src)
	out.Tools = mergeTools(global.Tools, project.Tools, out.src)
	out.MCPServers = mergeMCPServers(global.MCPServers, project.MCPServers, out.src)
	out.Search = mergeSearch(global.Search, project.Search, out.src)
	out.searchExtra = cloneSearchExtra(global.searchExtra)
	for k, v := range project.searchExtra {
		out.searchExtra[k] = v
	}
	out.Graph = mergeGraph(global.Graph, project.Graph, out.src)
	for k, v := range project.extra {
		out.extra[k] = v
	}
	for k, v := range project.contextExtra {
		if out.contextExtra == nil {
			out.contextExtra = map[string]json.RawMessage{}
		}
		out.contextExtra[k] = v
	}
	for k, v := range project.overflowExtra {
		if out.overflowExtra == nil {
			out.overflowExtra = map[string]json.RawMessage{}
		}
		out.overflowExtra[k] = v
	}
	mergeNestedRaw(project.roleExtra, out.roleExtra)
	mergeNestedRaw(project.toolExtra, out.toolExtra)
	mergeNestedRaw(project.mcpExtra, out.mcpExtra)
	mergeDeepNestedRaw(project.toolRoleExtra, out.toolRoleExtra)
	for k, v := range project.graphExtra {
		out.graphExtra[k] = v
	}
	mergeDeepNestedRaw(project.mcpRoleExtra, out.mcpRoleExtra)
	return out
}

// mergeGraph folds project graph settings over global ones field by field.
func mergeGraph(global, project GraphSetting, src map[string]string) GraphSetting {
	out := global
	if project.Enabled != nil {
		v := *project.Enabled
		out.Enabled = &v
		src["graph.enabled"] = "project"
	}
	if project.Binary != "" {
		out.Binary = project.Binary
		src["graph.binary"] = "project"
	}
	if project.CacheDir != "" {
		out.CacheDir = project.CacheDir
		src["graph.cache_dir"] = "project"
	}
	return out
}

// mergeRoles folds project slots over global ones, field by field, recording
// a project source for every field the project set.
func mergeRoles(global, project map[string]Role, src map[string]string) map[string]Role {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}
	out := map[string]Role{}
	for slot, r := range global {
		out[slot] = r
	}
	for slot, p := range project {
		base := out[slot]
		if p.Harness != "" {
			base.Harness = p.Harness
			src["roles."+slot+".harness"] = "project"
		}
		if p.Model != "" {
			base.Model = p.Model
			src["roles."+slot+".model"] = "project"
		}
		if p.Effort != "" {
			base.Effort = p.Effort
			src["roles."+slot+".effort"] = "project"
		}
		out[slot] = base
	}
	return out
}

// mergeTools folds project tool entries over global ones by name, then field
// by field, recording a project source for every field the project set.
func mergeTools(global, project map[string]ToolSetting, src map[string]string) map[string]ToolSetting {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}
	out := map[string]ToolSetting{}
	for name, e := range global {
		out[name] = cloneToolSetting(e)
	}
	for name, p := range project {
		base := out[name]
		if base.Roles == nil && len(p.Roles) > 0 {
			base.Roles = map[string]ToolRoleOverride{}
		}
		if p.Enabled != nil {
			v := *p.Enabled
			base.Enabled = &v
			src["tools."+name+".enabled"] = "project"
		}
		if p.Placement != "" {
			base.Placement = p.Placement
			src["tools."+name+".placement"] = "project"
		}
		if p.Rename != "" {
			base.Rename = p.Rename
			src["tools."+name+".rename"] = "project"
		}
		for role, ov := range p.Roles {
			rbase := base.Roles[role]
			if ov.Enabled != nil {
				v := *ov.Enabled
				rbase.Enabled = &v
				src["tools."+name+".roles."+role+".enabled"] = "project"
			}
			if ov.Placement != "" {
				rbase.Placement = ov.Placement
				src["tools."+name+".roles."+role+".placement"] = "project"
			}
			base.Roles[role] = rbase
		}
		out[name] = base
	}
	if len(out) > 0 && len(project) > 0 {
		src["tools"] = "project"
	}
	return out
}

// mergeMCPServers folds project server entries over global ones by name.
// Scalars and lists win wholesale when set; env and rename maps merge by key,
// where a project entry with an empty value deletes the mapping.
func mergeMCPServers(global, project map[string]MCPServerSetting, src map[string]string) map[string]MCPServerSetting {
	if len(global) == 0 && len(project) == 0 {
		return nil
	}
	out := map[string]MCPServerSetting{}
	for name, e := range global {
		out[name] = cloneMCPServerSetting(e)
	}
	for name, p := range project {
		base := out[name]
		if p.Command != "" {
			base.Command = p.Command
			src["mcp_servers."+name+".command"] = "project"
		}
		if p.Args != nil {
			base.Args = append([]string(nil), p.Args...)
			src["mcp_servers."+name+".args"] = "project"
		}
		if p.Env != nil {
			if base.Env == nil {
				base.Env = map[string]string{}
			}
			for k, v := range p.Env {
				base.Env[k] = v
			}
			src["mcp_servers."+name+".env"] = "project"
		}
		if p.Summary != "" {
			base.Summary = p.Summary
			src["mcp_servers."+name+".summary"] = "project"
		}
		if p.Placement != "" {
			base.Placement = p.Placement
			src["mcp_servers."+name+".placement"] = "project"
		}
		if p.Include != nil {
			base.Include = append([]string(nil), p.Include...)
			src["mcp_servers."+name+".include"] = "project"
		}
		if p.Exclude != nil {
			base.Exclude = append([]string(nil), p.Exclude...)
			src["mcp_servers."+name+".exclude"] = "project"
		}
		if p.Rename != nil {
			if base.Rename == nil {
				base.Rename = map[string]string{}
			}
			for k, v := range p.Rename {
				if v == "" {
					delete(base.Rename, k)
				} else {
					base.Rename[k] = v
				}
			}
			src["mcp_servers."+name+".rename"] = "project"
		}
		if base.Roles == nil && len(p.Roles) > 0 {
			base.Roles = map[string]MCPServerRoleOverride{}
		}
		for role, ov := range p.Roles {
			rbase := base.Roles[role]
			if ov.Enabled != nil {
				v := *ov.Enabled
				rbase.Enabled = &v
				src["mcp_servers."+name+".roles."+role+".enabled"] = "project"
			}
			if ov.Placement != "" {
				rbase.Placement = ov.Placement
				src["mcp_servers."+name+".roles."+role+".placement"] = "project"
			}
			base.Roles[role] = rbase
		}
		out[name] = base
	}
	if len(out) > 0 && len(project) > 0 {
		src["mcp_servers"] = "project"
	}
	return out
}

func cloneToolSetting(e ToolSetting) ToolSetting {
	out := e
	if e.Enabled != nil {
		v := *e.Enabled
		out.Enabled = &v
	}
	if e.Roles != nil {
		out.Roles = map[string]ToolRoleOverride{}
		for role, ov := range e.Roles {
			c := ov
			if ov.Enabled != nil {
				v := *ov.Enabled
				c.Enabled = &v
			}
			out.Roles[role] = c
		}
	}
	return out
}

func cloneMCPServerSetting(e MCPServerSetting) MCPServerSetting {
	out := e
	out.Args = append([]string(nil), e.Args...)
	out.Include = append([]string(nil), e.Include...)
	out.Exclude = append([]string(nil), e.Exclude...)
	if e.Env != nil {
		out.Env = map[string]string{}
		for k, v := range e.Env {
			out.Env[k] = v
		}
	}
	if e.Rename != nil {
		out.Rename = map[string]string{}
		for k, v := range e.Rename {
			out.Rename[k] = v
		}
	}
	if e.Roles != nil {
		out.Roles = map[string]MCPServerRoleOverride{}
		for role, ov := range e.Roles {
			c := ov
			if ov.Enabled != nil {
				v := *ov.Enabled
				c.Enabled = &v
			}
			out.Roles[role] = c
		}
	}
	return out
}

func cloneSources(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneRawMap(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return map[string]json.RawMessage{}
	}
	out := map[string]json.RawMessage{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneNestedRawMap(m map[string]map[string]json.RawMessage) map[string]map[string]json.RawMessage {
	// Always non-nil: MergeSettings folds project extras into the result, so
	// a nil map here panics on the first project unknown field.
	out := map[string]map[string]json.RawMessage{}
	for k, inner := range m {
		c := map[string]json.RawMessage{}
		for f, v := range inner {
			c[f] = v
		}
		out[k] = c
	}
	return out
}

func mergeNestedRaw(from, into map[string]map[string]json.RawMessage) {
	for k, inner := range from {
		if into[k] == nil {
			into[k] = map[string]json.RawMessage{}
		}
		for f, v := range inner {
			into[k][f] = v
		}
	}
}

func cloneDeepNestedRawMap(m map[string]map[string]map[string]json.RawMessage) map[string]map[string]map[string]json.RawMessage {
	out := map[string]map[string]map[string]json.RawMessage{}
	for k, mid := range m {
		c := map[string]map[string]json.RawMessage{}
		for r, inner := range mid {
			cc := map[string]json.RawMessage{}
			for f, v := range inner {
				cc[f] = v
			}
			c[r] = cc
		}
		out[k] = c
	}
	return out
}

func mergeDeepNestedRaw(from, into map[string]map[string]map[string]json.RawMessage) {
	for k, mid := range from {
		if into[k] == nil {
			into[k] = map[string]map[string]json.RawMessage{}
		}
		for r, inner := range mid {
			if into[k][r] == nil {
				into[k][r] = map[string]json.RawMessage{}
			}
			for f, v := range inner {
				into[k][r][f] = v
			}
		}
	}
}
