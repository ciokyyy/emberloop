package el

// Shared theme (ticket 06): one palette definition drives the strip, the
// generated claude and pig themes, and the pane colors (OSC). Catppuccin
// Mocha for dark terminals, Latte for light ones. ElPalette is the single
// source: the strip reads its roles through the headerColor vars in
// header.go, and the generators and the OSC setter assemble from the same
// struct, so no consumer keeps an independent hex table. Generated files
// live under el's state dir and reach the child only through per-run
// launch flags (and, for claude, two el-named theme files in its config dir), so the
// user's saved theme choice is never changed. Mocha is live-verified in
// herdr; Latte is generated from the same source and is not verified.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ElHue is one agent identity hue: the Mocha hex for dark terminals, the
// Latte hex for light ones. Green, yellow and red stay out, so identity
// never reads as status.
type ElHue struct {
	Dark  string
	Light string
}

// claudeThemeOverrides is one variant's claude token table, in the probe's
// shape. Borders stay at the surface tone (the borderless look); themed
// borders are a later ticket.
type claudeThemeOverrides struct {
	Claude                string `json:"claude"`
	Text                  string `json:"text"`
	InverseText           string `json:"inverseText"`
	Inactive              string `json:"inactive"`
	Subtle                string `json:"subtle"`
	Suggestion            string `json:"suggestion"`
	Permission            string `json:"permission"`
	Remember              string `json:"remember"`
	Success               string `json:"success"`
	Error                 string `json:"error"`
	Warning               string `json:"warning"`
	Merged                string `json:"merged"`
	PromptBorder          string `json:"promptBorder"`
	PlanMode              string `json:"planMode"`
	AutoAccept            string `json:"autoAccept"`
	BashBorder            string `json:"bashBorder"`
	Ide                   string `json:"ide"`
	DiffAdded             string `json:"diffAdded"`
	DiffRemoved           string `json:"diffRemoved"`
	DiffAddedWord         string `json:"diffAddedWord"`
	DiffRemovedWord       string `json:"diffRemovedWord"`
	UserMessageBackground string `json:"userMessageBackground"`
	BashMessageBackground string `json:"bashMessageBackgroundColor"`
	MemoryBackground      string `json:"memoryBackgroundColor"`
	UserMessageHover      string `json:"userMessageBackgroundHover"`
	SelectionBg           string `json:"selectionBg"`
}

// pigThemeVars is one variant's pig var table, in the probe's shape.
type pigThemeVars struct {
	Text     string `json:"text"`
	Muted    string `json:"muted"`
	Dim      string `json:"dim"`
	Mauve    string `json:"mauve"`
	Blue     string `json:"blue"`
	Green    string `json:"green"`
	Red      string `json:"red"`
	Yellow   string `json:"yellow"`
	Peach    string `json:"peach"`
	Teal     string `json:"teal"`
	Surface0 string `json:"surface0"`
	Surface1 string `json:"surface1"`
	Mantle   string `json:"mantle"`
}

// pigThemeColors is one variant's pig color table, in the probe's shape.
// Borders stay at the surface tone (the borderless look).
type pigThemeColors struct {
	Accent          string `json:"accent"`
	Border          string `json:"border"`
	BorderAccent    string `json:"borderAccent"`
	BorderMuted     string `json:"borderMuted"`
	Success         string `json:"success"`
	Error           string `json:"error"`
	Warning         string `json:"warning"`
	Muted           string `json:"muted"`
	Dim             string `json:"dim"`
	Text            string `json:"text"`
	ThinkingText    string `json:"thinkingText"`
	SelectedBg      string `json:"selectedBg"`
	ScrollbarTrack  string `json:"scrollbarTrack"`
	ScrollbarThumb  string `json:"scrollbarThumb"`
	SearchMatchBg   string `json:"searchMatchBg"`
	SearchMatchText string `json:"searchMatchText"`
	UserMessageBg   string `json:"userMessageBg"`
	UserMessageText string `json:"userMessageText"`
	CustomMessageBg string `json:"customMessageBg"`
	CustomMessageTx string `json:"customMessageText"`
	CustomMessageLb string `json:"customMessageLabel"`
	ToolPendingBg   string `json:"toolPendingBg"`
	ToolSuccessBg   string `json:"toolSuccessBg"`
	ToolErrorBg     string `json:"toolErrorBg"`
	ToolTitle       string `json:"toolTitle"`
	ToolOutput      string `json:"toolOutput"`
	MdHeading       string `json:"mdHeading"`
	MdLink          string `json:"mdLink"`
	MdLinkURL       string `json:"mdLinkUrl"`
	MdCode          string `json:"mdCode"`
	MdCodeBlock     string `json:"mdCodeBlock"`
	MdCodeBlockBrd  string `json:"mdCodeBlockBorder"`
	MdQuote         string `json:"mdQuote"`
	MdQuoteBorder   string `json:"mdQuoteBorder"`
	MdHr            string `json:"mdHr"`
	MdListBullet    string `json:"mdListBullet"`
	ToolDiffAdded   string `json:"toolDiffAdded"`
	ToolDiffRemoved string `json:"toolDiffRemoved"`
	ToolDiffContext string `json:"toolDiffContext"`
	SyntaxComment   string `json:"syntaxComment"`
	SyntaxKeyword   string `json:"syntaxKeyword"`
	SyntaxFunction  string `json:"syntaxFunction"`
	SyntaxVariable  string `json:"syntaxVariable"`
	SyntaxString    string `json:"syntaxString"`
	SyntaxNumber    string `json:"syntaxNumber"`
	SyntaxType      string `json:"syntaxType"`
	SyntaxOperator  string `json:"syntaxOperator"`
	SyntaxPunct     string `json:"syntaxPunctuation"`
	ThinkingOff     string `json:"thinkingOff"`
	ThinkingMinimal string `json:"thinkingMinimal"`
	ThinkingLow     string `json:"thinkingLow"`
	ThinkingMedium  string `json:"thinkingMedium"`
	ThinkingHigh    string `json:"thinkingHigh"`
	ThinkingXhigh   string `json:"thinkingXhigh"`
	ThinkingMax     string `json:"thinkingMax"`
	BashMode        string `json:"bashMode"`
}

// ElPalette is the full shared palette for one variant: every token the
// strip, the generators and the OSC setter need, from one Catppuccin source.
type ElPalette struct {
	StripBg string
	Text    string
	Dim     string
	Accent  string
	Calm    string
	Warm    string
	Hot     string
	Cost    string
	Tool    string
	Idle    string
	BarDim  string
	Hues    [7]ElHue
	// Generator and OSC tokens: the claude and pig theme tables and the
	// pane colors for this variant.
	ClaudeName    string
	ClaudeBase    string
	Claude        claudeThemeOverrides
	PigName       string
	PigAppearance string
	PigVars       pigThemeVars
	PigColors     pigThemeColors
	PaneFG        string
	PaneBG        string
	PaneCursor    string
	PaneANSI      [16]string
}

// ElMocha is the dark variant: Catppuccin Mocha.
func ElMocha() ElPalette {
	return ElPalette{
		StripBg: "#181825",
		Text:    "#cdd6f4",
		Dim:     "#6c7086",
		Accent:  "#cba6f7",
		Calm:    "#a6e3a1",
		Warm:    "#f9e2af",
		Hot:     "#f38ba8",
		Cost:    "#fab387",
		Tool:    "#89b4fa",
		Idle:    "#94e2d5",
		BarDim:  "#313244",
		Hues: [7]ElHue{
			{"#cba6f7", "#8839ef"}, // mauve: the orchestrator
			{"#f5c2e7", "#ea76cb"}, // pink
			{"#b4befe", "#7287fd"}, // lavender
			{"#74c7ec", "#209fb5"}, // sapphire
			{"#89dceb", "#04a5e5"}, // sky
			{"#f2cdcd", "#dd7878"}, // flamingo
			{"#eba0ac", "#e64553"}, // maroon
		},
		ClaudeName: "El (Catppuccin Mocha)",
		ClaudeBase: "dark",
		Claude: claudeThemeOverrides{
			Claude: "#cba6f7", Text: "#cdd6f4", InverseText: "#181825",
			Inactive: "#7f849c", Subtle: "#313244",
			Suggestion: "#f5c2e7", Permission: "#f5c2e7", Remember: "#94e2d5",
			Success: "#a6e3a1", Error: "#f38ba8", Warning: "#fab387",
			Merged: "#cba6f7", PromptBorder: "#313244", PlanMode: "#89dceb",
			AutoAccept: "#a6e3a1", BashBorder: "#313244", Ide: "#89b4fa",
			DiffAdded: "#2a3b2f", DiffRemoved: "#43262f",
			DiffAddedWord: "#40604a", DiffRemovedWord: "#6b3a48",
			UserMessageBackground: "#3b3b57",
			BashMessageBackground: "#2a2a3c", MemoryBackground: "#2a2a3c",
			UserMessageHover: "#45475a", SelectionBg: "#585b70",
		},
		PigName: "el-mocha", PigAppearance: "dark",
		PigVars: pigThemeVars{
			Text: "#cdd6f4", Muted: "#a6adc8", Dim: "#6c7086",
			Mauve: "#cba6f7", Blue: "#89b4fa", Green: "#a6e3a1",
			Red: "#f38ba8", Yellow: "#f9e2af", Peach: "#fab387",
			Teal:     "#94e2d5",
			Surface0: "#313244", Surface1: "#45475a", Mantle: "#181825",
		},
		PigColors: pigThemeColors{
			Accent: "#f5c2e7",
			Border: "surface0", BorderAccent: "surface0", BorderMuted: "surface0",
			Success: "green", Error: "red", Warning: "peach",
			Muted: "muted", Dim: "#7f849c", Text: "text",
			ThinkingText: "dim", SelectedBg: "#45475a",
			ScrollbarTrack: "surface0", ScrollbarThumb: "dim",
			SearchMatchBg: "surface1", SearchMatchText: "text",
			UserMessageBg: "#3b3b57", UserMessageText: "text",
			CustomMessageBg: "mantle", CustomMessageTx: "muted",
			CustomMessageLb: "mauve",
			ToolPendingBg:   "#252536", ToolSuccessBg: "#1f2f2c", ToolErrorBg: "#35212b",
			ToolTitle: "text", ToolOutput: "muted",
			MdHeading: "#f5c2e7", MdLink: "blue", MdLinkURL: "dim",
			MdCode: "peach", MdCodeBlock: "green", MdCodeBlockBrd: "surface0",
			MdQuote: "muted", MdQuoteBorder: "surface0", MdHr: "surface0",
			MdListBullet:  "mauve",
			ToolDiffAdded: "green", ToolDiffRemoved: "red", ToolDiffContext: "dim",
			SyntaxComment: "dim", SyntaxKeyword: "mauve", SyntaxFunction: "blue",
			SyntaxVariable: "text", SyntaxString: "green", SyntaxNumber: "peach",
			SyntaxType: "yellow", SyntaxOperator: "teal", SyntaxPunct: "muted",
			ThinkingOff: "dim", ThinkingMinimal: "surface1", ThinkingLow: "blue",
			ThinkingMedium: "teal", ThinkingHigh: "mauve", ThinkingXhigh: "peach",
			ThinkingMax: "red", BashMode: "peach",
		},
		PaneFG: "#cdd6f4", PaneBG: "#1e1e2e", PaneCursor: "#f5e0dc",
		PaneANSI: [16]string{
			"#45475a", "#f38ba8", "#a6e3a1", "#f9e2af",
			"#89b4fa", "#f5c2e7", "#94e2d5", "#bac2de",
			"#585b70", "#f38ba8", "#a6e3a1", "#f9e2af",
			"#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8",
		},
	}
}

// ElLatte is the light variant: Catppuccin Latte, generated from the same
// source as Mocha. Not live-verified (spec: Out of Scope).
func ElLatte() ElPalette {
	m := ElMocha()
	return ElPalette{
		StripBg:    "#e6e9ef",
		Text:       "#4c4f69",
		Dim:        "#9ca0b0",
		Accent:     "#8839ef",
		Calm:       "#40a02b",
		Warm:       "#df8e1d",
		Hot:        "#d20f39",
		Cost:       "#fe640b",
		Tool:       "#1e66f5",
		Idle:       "#179299",
		BarDim:     "#ccd0da",
		Hues:       m.Hues,
		ClaudeName: "El (Catppuccin Latte)",
		ClaudeBase: "light",
		Claude: claudeThemeOverrides{
			Claude: "#8839ef", Text: "#4c4f69", InverseText: "#e6e9ef",
			Inactive: "#8c8f9e", Subtle: "#ccd0da",
			Suggestion: "#ea76cb", Permission: "#ea76cb", Remember: "#179299",
			Success: "#40a02b", Error: "#d20f39", Warning: "#fe640b",
			Merged: "#8839ef", PromptBorder: "#ccd0da", PlanMode: "#04a5e5",
			AutoAccept: "#40a02b", BashBorder: "#ccd0da", Ide: "#1e66f5",
			DiffAdded: "#d9e8dc", DiffRemoved: "#f3d7dd",
			DiffAddedWord: "#b3d4ba", DiffRemovedWord: "#e6aeb9",
			UserMessageBackground: "#dde1ed",
			BashMessageBackground: "#e4e7f0", MemoryBackground: "#e4e7f0",
			UserMessageHover: "#bcc0cc", SelectionBg: "#acb0be",
		},
		PigName: "el-latte", PigAppearance: "light",
		PigVars: pigThemeVars{
			Text: "#4c4f69", Muted: "#6c6f85", Dim: "#9ca0b0",
			Mauve: "#8839ef", Blue: "#1e66f5", Green: "#40a02b",
			Red: "#d20f39", Yellow: "#df8e1d", Peach: "#fe640b",
			Teal:     "#179299",
			Surface0: "#ccd0da", Surface1: "#bcc0cc", Mantle: "#e6e9ef",
		},
		PigColors: pigThemeColors{
			Accent: "#ea76cb",
			Border: "surface0", BorderAccent: "surface0", BorderMuted: "surface0",
			Success: "green", Error: "red", Warning: "peach",
			Muted: "muted", Dim: "#8c8f9e", Text: "text",
			ThinkingText: "dim", SelectedBg: "#bcc0cc",
			ScrollbarTrack: "surface0", ScrollbarThumb: "dim",
			SearchMatchBg: "surface1", SearchMatchText: "text",
			UserMessageBg: "#dde1ed", UserMessageText: "text",
			CustomMessageBg: "mantle", CustomMessageTx: "muted",
			CustomMessageLb: "mauve",
			ToolPendingBg:   "#e2e4f0", ToolSuccessBg: "#d8e8db", ToolErrorBg: "#f3d8de",
			ToolTitle: "text", ToolOutput: "muted",
			MdHeading: "#ea76cb", MdLink: "blue", MdLinkURL: "dim",
			MdCode: "peach", MdCodeBlock: "green", MdCodeBlockBrd: "surface0",
			MdQuote: "muted", MdQuoteBorder: "surface0", MdHr: "surface0",
			MdListBullet:  "mauve",
			ToolDiffAdded: "green", ToolDiffRemoved: "red", ToolDiffContext: "dim",
			SyntaxComment: "dim", SyntaxKeyword: "mauve", SyntaxFunction: "blue",
			SyntaxVariable: "text", SyntaxString: "green", SyntaxNumber: "peach",
			SyntaxType: "yellow", SyntaxOperator: "teal", SyntaxPunct: "muted",
			ThinkingOff: "dim", ThinkingMinimal: "surface1", ThinkingLow: "blue",
			ThinkingMedium: "teal", ThinkingHigh: "mauve", ThinkingXhigh: "peach",
			ThinkingMax: "red", BashMode: "peach",
		},
		PaneFG: "#4c4f69", PaneBG: "#eff1f5", PaneCursor: "#dc8a78",
		PaneANSI: [16]string{
			"#bcc0cc", "#d20f39", "#40a02b", "#df8e1d",
			"#1e66f5", "#ea76cb", "#179299", "#5c5f77",
			"#acb0be", "#d20f39", "#40a02b", "#df8e1d",
			"#1e66f5", "#ea76cb", "#179299", "#6c6f85",
		},
	}
}

// elVariant selects the palette variant: Mocha for dark, Latte for light.
func elVariant(dark bool) ElPalette {
	if dark {
		return ElMocha()
	}
	return ElLatte()
}

// ThemeVariant names a generated theme: mocha for dark, latte for light.
func ThemeVariant(dark bool) string {
	if dark {
		return "mocha"
	}
	return "latte"
}

// ThemeName is the theme name inside a generated file: el-mocha or
// el-latte, so the strip, claude, pig and the pane cannot drift.
func ThemeName(dark bool) string { return "el-" + ThemeVariant(dark) }

// ClaudeThemeRef is the theme reference passed to claude through the launch
// settings JSON: custom:<name>, as in the theme probe.
func ClaudeThemeRef(dark bool) string { return "custom:" + ThemeName(dark) }

// claudeTheme is the generated claude custom theme: name, base appearance
// and token overrides, in the probe's shape.
type claudeTheme struct {
	Name      string               `json:"name"`
	Base      string               `json:"base"`
	Overrides claudeThemeOverrides `json:"overrides"`
}

// ClaudeTheme builds the generated claude theme for the variant, from the
// shared palette.
func ClaudeTheme(dark bool) claudeTheme {
	v := elVariant(dark)
	return claudeTheme{Name: v.ClaudeName, Base: v.ClaudeBase, Overrides: v.Claude}
}

// pigTheme is the generated pig theme: vars plus color refs, in the probe's
// shape.
type pigTheme struct {
	Name       string         `json:"name"`
	Appearance string         `json:"appearance"`
	Vars       pigThemeVars   `json:"vars"`
	Colors     pigThemeColors `json:"colors"`
}

// PigTheme builds the generated pig theme for the variant, from the shared
// palette.
func PigTheme(dark bool) pigTheme {
	v := elVariant(dark)
	return pigTheme{Name: v.PigName, Appearance: v.PigAppearance, Vars: v.PigVars, Colors: v.PigColors}
}

// Marshal helpers keep the golden fixtures byte-stable: two-space indent
// with a trailing newline, as the probe assets.
func marshalTheme(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// ClaudeThemeJSON renders the generated claude theme file for the variant.
func ClaudeThemeJSON(dark bool) (string, error) { return marshalTheme(ClaudeTheme(dark)) }

// PigThemeJSON renders the generated pig theme file for the variant.
func PigThemeJSON(dark bool) (string, error) { return marshalTheme(PigTheme(dark)) }

// Theme file names under the theme dir.
func claudeThemeFile(dark bool) string { return "claude-" + ThemeVariant(dark) + ".json" }
func pigThemeFile(dark bool) string    { return "pig-" + ThemeVariant(dark) + ".json" }

// DefaultThemeDir is where generated theme files live: el's state dir.
func DefaultThemeDir() string { return filepath.Join(filepath.Dir(statePath), "themes") }

// EnsureThemeFiles writes the generated claude and pig themes (both
// variants, one source) under dir and returns the file paths by variant.
func EnsureThemeFiles(dir string) (map[string]string, error) {
	if dir == "" {
		dir = DefaultThemeDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, dark := range []bool{true, false} {
		cj, err := ClaudeThemeJSON(dark)
		if err != nil {
			return nil, err
		}
		pj, err := PigThemeJSON(dark)
		if err != nil {
			return nil, err
		}
		cp, pp := filepath.Join(dir, claudeThemeFile(dark)), filepath.Join(dir, pigThemeFile(dark))
		if err := os.WriteFile(cp, []byte(cj), 0o600); err != nil {
			return nil, err
		}
		if err := os.WriteFile(pp, []byte(pj), 0o600); err != nil {
			return nil, err
		}
		out[claudeThemeFile(dark)] = cp
		out[pigThemeFile(dark)] = pp
	}
	return out, nil
}

// ClaudeConfigDir installs the generated claude themes where Claude Code
// resolves custom:<slug>: <dir>/themes/<slug>.json, and returns dir for a
// per-run CLAUDE_CONFIG_DIR. The state-dir files stay the generated
// artefacts (spec line 79); these slug-named copies are what the launch
// points at, so the user's ~/.claude is never touched.
func ClaudeConfigDir(themeDir string) (string, error) {
	if themeDir == "" {
		themeDir = DefaultThemeDir()
	}
	dir := filepath.Join(themeDir, "claude-config", "themes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, dark := range []bool{true, false} {
		cj, err := ClaudeThemeJSON(dark)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, ThemeName(dark)+".json"), []byte(cj), 0o600); err != nil {
			return "", err
		}
	}
	return filepath.Join(themeDir, "claude-config"), nil
}

// ThemePaneColors is the pane's own palette: default foreground,
// background, cursor and the 16 ANSI colors, from the same source.
type ThemePaneColors struct {
	FG     string
	BG     string
	Cursor string
	ANSI   [16]string
}

// PaneColors returns the OSC palette for the variant, from the shared
// palette. The Mocha values are the approved probe (run-themed.sh); Latte
// mirrors its structure with the Latte source.
func PaneColors(dark bool) ThemePaneColors {
	v := elVariant(dark)
	return ThemePaneColors{FG: v.PaneFG, BG: v.PaneBG, Cursor: v.PaneCursor, ANSI: v.PaneANSI}
}

// ThemeSetSequence is the exact bytes the wrapper writes to the terminal at
// start: OSC 11 (background), 10 (foreground), 12 (cursor) and 4 (16 ANSI),
// in the probe's order.
func ThemeSetSequence(dark bool) string {
	c := PaneColors(dark)
	var b strings.Builder
	fmt.Fprintf(&b, "\x1b]11;#%s\x07\x1b]10;#%s\x07\x1b]12;#%s\x07",
		strings.TrimPrefix(c.BG, "#"), strings.TrimPrefix(c.FG, "#"), strings.TrimPrefix(c.Cursor, "#"))
	for i, hex := range c.ANSI {
		fmt.Fprintf(&b, "\x1b]4;%d;#%s\x07", i, strings.TrimPrefix(hex, "#"))
	}
	return b.String()
}

// ThemeResetSequence is the exact bytes the wrapper writes on exit: OSC
// 104, 110, 111 and 112 restore the palette, foreground, background and
// cursor, in the spec's order.
func ThemeResetSequence() string {
	return "\x1b]104\x07\x1b]110\x07\x1b]111\x07\x1b]112\x07"
}

// ThemeLaunchArgs returns the child args with the per-run theme flags for
// the harness: pig gets --theme <generated file> --use-theme <name>, claude
// gets its theme reference merged into the --settings JSON it already
// launches with. Saved claude and pig settings are untouched: only the
// launch changes. An --theme already on the pig command wins, and a theme
// already in the claude settings wins.
func ThemeLaunchArgs(harness string, args []string, dark bool, themeDir string) ([]string, error) {
	files, err := EnsureThemeFiles(themeDir)
	if err != nil {
		return args, err
	}
	switch harness {
	case "pig":
		for i, a := range args {
			if a == "--theme" && i+1 < len(args) {
				return args, nil
			}
			if strings.HasPrefix(a, "--theme=") {
				return args, nil
			}
		}
		file := files[pigThemeFile(dark)]
		// Flags go before a "--": everything after it is the prompt, and pig
		// would take the theme flags as user messages.
		flags := []string{"--theme", file, "--use-theme", ThemeName(dark)}
		at := len(args)
		for i, a := range args {
			if a == "--" {
				at = i
				break
			}
		}
		out := append([]string{}, args[:at]...)
		out = append(out, flags...)
		return append(out, args[at:]...), nil
	case "claude":
		ref := ClaudeThemeRef(dark)
		for i, a := range args {
			if a == "--settings" && i+1 < len(args) {
				merged, err := mergeClaudeTheme(args[i+1], ref)
				if err != nil {
					return args, err
				}
				out := append([]string{}, args...)
				out[i+1] = merged
				return out, nil
			}
		}
		return append(append([]string{}, args...), "--settings", `{"theme":`+jsonQuote(ref)+`}`), nil
	default:
		return args, fmt.Errorf("unknown harness %q", harness)
	}
}

// claudeConfigRoot is the config directory the user's claude reads: the
// CLAUDE_CONFIG_DIR already in the environment, else ~/.claude.
func claudeConfigRoot() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), ".claude")
}

// InstallClaudeThemes writes the generated claude themes into <root>/themes
// under el's own slug names, where Claude Code resolves custom:<slug>.
// This adds two files and changes nothing else: the theme choice travels in
// the launch settings, and the user's saved selection stays untouched. A
// per-run CLAUDE_CONFIG_DIR is not an option: a fresh config directory has
// no login and no onboarding state, so every claude agent would stop at the
// first-run dialogs instead of working.
func InstallClaudeThemes(root string) error {
	dir := filepath.Join(root, "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, dark := range []bool{true, false} {
		cj, err := ClaudeThemeJSON(dark)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, ThemeName(dark)+".json"), []byte(cj), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ThemeLaunchEnv installs the claude theme files and returns the extra
// child environment for the harness theme: none. Claude resolves the theme
// reference from the user's own config directory, so the child keeps its
// login and onboarding state. Pig needs none either: its theme travels in
// the args.
func ThemeLaunchEnv(harness string, dark bool, themeDir string) ([]string, error) {
	_, _ = dark, themeDir
	if harness != "claude" {
		return nil, nil
	}
	return nil, InstallClaudeThemes(claudeConfigRoot())
}

// mergeClaudeTheme sets the theme reference in a claude --settings JSON
// document, leaving every other key (the stop gate, hooks) intact. A theme
// already there wins.
func mergeClaudeTheme(settingsJSON, ref string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &m); err != nil {
		return "", fmt.Errorf("claude --settings is not JSON: %w", err)
	}
	if _, ok := m["theme"]; !ok {
		m["theme"] = ref
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// jsonQuote renders a JSON string literal.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
