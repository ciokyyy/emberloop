package tools

// Wick macros (input-queue ticket 03): the row's own commands. A macro is a
// name, a kind (a prompt template or a built-in action) and a template with
// one {{args}} slot. Typing a registered macro in the row expands and
// delivers it, or runs the action; /compress ships as the first built-in and
// calls the existing compress request. Text starting with a slash that names
// no macro is a normal message. A macro shadowing a built-in warns at
// settings load, and wick's macro wins.

import (
	"fmt"
	"strings"
)

// Macro kinds, v1: prompt template macros and built-in action macros.
// Sequences are out.
const (
	MacroKindPrompt  = "prompt"
	MacroKindBuiltin = "builtin"
)

// MacroBuiltinCompress is the first built-in action macro: it calls the
// supervisor's compress request for the wrapped session.
const MacroBuiltinCompress = "compress"

// MacroSetting is one entry of the settings file's macros list.
type MacroSetting struct {
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Template string `json:"template,omitempty"`
}

// NormalizeMacroName trims a macro name and strips one leading slash, so a
// settings entry written as "/review" still matches a /review row call.
func NormalizeMacroName(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "/")
}

// NormalizeMacroKind lowercases and trims a macro kind.
func NormalizeMacroKind(v string) string { return strings.ToLower(strings.TrimSpace(v)) }

// IsBuiltinMacro reports whether name calls a built-in action macro. The
// check is exact after normalization: wick's macros win on a collision.
// Built-ins take no arguments: trailing words on a /compress call are ignored.
func IsBuiltinMacro(name string) bool {
	return NormalizeMacroName(name) == MacroBuiltinCompress
}

// ParseMacroCall splits row text of the form "/name args..." into the macro
// name and its trailing args. Text without a leading slash, a bare slash,
// or a slash followed by blank is no call: the row sends it as a message.
func ParseMacroCall(text string) (name, args string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") || len(trimmed) < 2 {
		return "", "", false
	}
	rest := trimmed[1:]
	name = rest
	args = ""
	if i := strings.IndexAny(rest, " \t\n"); i >= 0 {
		name = rest[:i]
		args = strings.TrimSpace(rest[i+1:])
	}
	if name == "" {
		return "", "", false
	}
	return name, args, true
}

// ExpandMacro fills every {{args}} slot of a prompt template with args. A
// template without a slot stands as is.
func ExpandMacro(template, args string) string {
	out := strings.ReplaceAll(template, "{{args}}", args)
	return strings.ReplaceAll(out, "{{ args }}", args)
}

// validPromptMacros keeps the usable prompt entries of s.Macros in order:
// named, kind prompt, non-empty template, first of its name, and never a
// built-in shadow (wick's macro wins there).
func (s Settings) validPromptMacros() []MacroSetting {
	seen := map[string]bool{}
	var out []MacroSetting
	for _, m := range s.Macros {
		name := NormalizeMacroName(m.Name)
		if name == "" || seen[name] || IsBuiltinMacro(name) {
			continue
		}
		seen[name] = true
		if NormalizeMacroKind(m.Kind) != MacroKindPrompt {
			continue
		}
		if strings.TrimSpace(m.Template) == "" {
			continue
		}
		out = append(out, m)
	}
	return out
}

// LookupMacro returns the prompt macro for name, or false when no usable
// entry defines it. Built-ins never look up: the caller runs those.
func (s Settings) LookupMacro(name string) (MacroSetting, bool) {
	want := NormalizeMacroName(name)
	for _, m := range s.validPromptMacros() {
		if NormalizeMacroName(m.Name) == want {
			return m, true
		}
	}
	return MacroSetting{}, false
}

// ValidateMacros checks s.Macros at settings load. A duplicate name, an
// empty name, an unknown kind, an unknown built-in, or an empty prompt
// template is rejected (the first usable entry wins); a user entry naming a
// built-in warns and is dropped, since wick's macro wins the collision.
func (s Settings) ValidateMacros() (valid []MacroSetting, warnings []string, err error) {
	seen := map[string]bool{}
	var errs []string
	for _, m := range s.Macros {
		name := NormalizeMacroName(m.Name)
		kind := NormalizeMacroKind(m.Kind)
		switch {
		case name == "":
			errs = append(errs, "macro with no name: dropped")
		case seen[name]:
			errs = append(errs, fmt.Sprintf("duplicate macro %q: keeping the first entry", name))
		case IsBuiltinMacro(name):
			warnings = append(warnings, fmt.Sprintf("macro %q shadows the built-in: wick's macro wins", name))
			seen[name] = true
		case kind == MacroKindBuiltin:
			errs = append(errs, fmt.Sprintf("unknown built-in macro %q: dropped", name))
			seen[name] = true
		case kind != MacroKindPrompt:
			errs = append(errs, fmt.Sprintf("unknown macro kind %q for %q: want prompt or builtin", m.Kind, name))
			seen[name] = true
		case strings.TrimSpace(m.Template) == "":
			errs = append(errs, fmt.Sprintf("macro %q has no template: dropped", name))
			seen[name] = true
		default:
			seen[name] = true
			valid = append(valid, m)
		}
	}
	if len(errs) > 0 {
		err = fmt.Errorf("invalid macros: %s", strings.Join(errs, "; "))
	}
	return valid, warnings, err
}
