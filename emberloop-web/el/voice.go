package el

import (
	_ "embed"
	"encoding/json"
	"io"
	"regexp"
	"strings"
)

// The register every el session writes its own words in is the caveman
// plugin's, adopted as the plugin loads it, at its ultra level, always:
//
//   - the SessionStart text: a banner and caveman/SKILL.md, a byte-for-byte
//     copy of the plugin's skill, with the frontmatter stripped and only the
//     ultra row of the intensity table and the ultra examples kept (the
//     plugin's loadFilteredRuleset), so the model is never handed a second
//     level to choose between. It is a layer of the system prompt, so every
//     role and agent type reads it and no skill body can outrank it.
//   - the UserPromptSubmit text: the plugin's one-line per-turn reminder,
//     delivered on every user message by the claude harness's UserPromptSubmit
//     hook and by the pig extension, because the rules read far back in the
//     system prompt lose to whatever the latest message says.
//
// Source: JuliusBrussee/caveman, plugin skill caveman at commit
// b39c90862855ad2f0813ce775b8bf07a9d6d2a50. To update, copy SKILL.md again;
// nothing else here restates it.
//
//go:embed caveman/SKILL.md
var cavemanSkill string

// voiceLevel is the one intensity level el runs. It has no switch.
const voiceLevel = "ultra"

// voiceBanner is the plugin's ruleset banner for that level.
const voiceBanner = "CAVEMAN MODE ACTIVE — level: " + voiceLevel

// voiceReminder is the plugin's ultra per-turn reminder, as it emits it.
const voiceReminder = "CAVEMAN MODE ACTIVE (" + voiceLevel + "). Enforce this reply: " +
	"Drop articles, filler, pleasantries, hedging, and excess conjunctions. Prefer fragments over full natural-prose sentences. State each fact once. No preamble or recap." +
	" Technical terms, code, commands, paths, and errors stay exact."

// voiceEl is what el adds after the plugin's text: where a spawned agent's
// result goes.
const voiceEl = "El. Your report goes in the submit tool when you have one: call it last, " +
	"with the full result inside it, in this register, and end the turn with no text after it. " +
	"Without submit, the final answer is the result, not a recap of the steps."

var (
	skillFrontmatter = regexp.MustCompile(`^---[\s\S]*?---\s*`)
	skillTableRow    = regexp.MustCompile(`^\|\s*\*\*(\S+?)\*\*\s*\|`)
	skillExample     = regexp.MustCompile(`^- (\S+?):\s`)
)

// filterRuleset is the plugin's loadFilteredRuleset: the skill without its
// frontmatter, keeping only level's row of the intensity table and level's
// example lines.
func filterRuleset(skill, level string) string {
	body := skillFrontmatter.ReplaceAllString(skill, "")
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if m := skillTableRow.FindStringSubmatch(line); m != nil {
			if m[1] == level {
				out = append(out, line)
			}
			continue
		}
		if m := skillExample.FindStringSubmatch(line); m != nil {
			if m[1] == level {
				out = append(out, line)
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// voiceRegister is the register as the system prompt carries it.
var voiceRegister = voiceBanner + "\n\n" + filterRuleset(cavemanSkill, voiceLevel) + "\n\n" + voiceEl

// VoiceReminder is the per-turn reminder, for the extension that cannot import
// this package.
func VoiceReminder() string { return voiceReminder }

// RunVoiceReminder serves `el voice-reminder`. Plain, it prints the
// reminder text, which the pig extension reads. With hook it prints claude's
// UserPromptSubmit hook output, which adds the reminder to the user's message.
func RunVoiceReminder(args []string, out io.Writer) error {
	if len(args) == 1 && args[0] == "hook" {
		return json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName":     "UserPromptSubmit",
			"additionalContext": voiceReminder,
		}})
	}
	_, err := io.WriteString(out, voiceReminder)
	return err
}
