package web

import (
	"slices"
	"testing"

	"github.com/MichaelKinsy/PiG/ai"
)

func TestPigModelEffortOptionsUseSupportedLevelsAndMaxThinking(t *testing.T) {
	str := func(value string) *string { return &value }
	model := ai.GeneratedModel{
		Reasoning: true,
		ThinkingLevelMap: ai.ThinkingLevelMap{
			ai.ThinkingOff:    nil,
			ai.ThinkingLow:    nil,
			ai.ThinkingMedium: str("medium"),
			ai.ThinkingHigh:   str("high"),
			ai.ThinkingXHigh:  str("xhigh"),
			ai.ThinkingMax:    str("max"),
		},
	}
	got := pigModelEffortOptions(model)
	want := []string{"minimal", "medium", "high", "xhigh", "max"}
	if !slices.Equal(got, want) {
		t.Fatalf("mapped pig efforts = %v, want %v", got, want)
	}

	// Models with a reasoning capability but no explicit level map use PiG's
	// MaxThinking default rather than appearing to support no efforts.
	reasoning := ai.GeneratedModel{Reasoning: true}
	if max := reasoning.ToCapabilities().MaxThinking; max != ai.ThinkingHigh {
		t.Fatalf("reasoning model MaxThinking = %q, want %q", max, ai.ThinkingHigh)
	}
	if got, want := pigModelEffortOptions(reasoning), []string{"off", "minimal", "low", "medium", "high"}; !slices.Equal(got, want) {
		t.Fatalf("unmapped reasoning pig efforts = %v, want %v", got, want)
	}
}

func TestModelEffortOptionsPerHarness(t *testing.T) {
	offeredClaude := []string{"opus", "sonnet", "haiku"}
	for _, model := range []string{"opus", "sonnet"} {
		if got := modelEffortOptions("claude", model, offeredClaude); !slices.Equal(got, claudeEfforts) {
			t.Errorf("Claude %s efforts = %v, want %v", model, got, claudeEfforts)
		}
	}
	for _, model := range []string{"haiku", "unknown"} {
		if got := modelEffortOptions("claude", model, offeredClaude); len(got) != 0 {
			t.Errorf("Claude %s efforts = %v, want none", model, got)
		}
	}
	offeredGrok := []string{grokDefaultModel}
	if got := modelEffortOptions("grok", grokDefaultModel, offeredGrok); !slices.Equal(got, grokEfforts) {
		t.Errorf("Grok efforts = %v, want %v", got, grokEfforts)
	}
	if got := modelEffortOptions("grok", "unknown", offeredGrok); len(got) != 0 {
		t.Errorf("unknown Grok model efforts = %v, want none", got)
	}
	if got := modelEffortOptions("pig", "unknown/model", nil); len(got) != 0 {
		t.Errorf("unknown pig model efforts = %v, want none", got)
	}
}

func TestModelEffortOptionsPigCatalog(t *testing.T) {
	catalog := ai.ListModels("")
	var nilOff, reasoningWithoutMap *ai.GeneratedModel
	for i := range catalog {
		model := &catalog[i]
		if mapped, ok := model.ThinkingLevelMap[ai.ThinkingOff]; ok && mapped == nil && nilOff == nil {
			nilOff = model
		}
		if len(model.ThinkingLevelMap) == 0 && model.ToCapabilities().MaxThinking != "" && reasoningWithoutMap == nil {
			reasoningWithoutMap = model
		}
	}
	if nilOff == nil {
		t.Fatal("PiG catalog has no model that explicitly rejects ThinkingOff")
	}
	pigName := func(model *ai.GeneratedModel) string { return model.Provider + "/" + model.ID }
	nilOffName := pigName(nilOff)
	if got := modelEffortOptions("pig", nilOffName, []string{nilOffName}); slices.Contains(got, string(ai.ThinkingOff)) {
		t.Errorf("%s offers unsupported effort off: %v", nilOffName, got)
	}
	if reasoningWithoutMap == nil {
		t.Fatal("PiG catalog has no reasoning model without an explicit effort map")
	}
	reasoningName := pigName(reasoningWithoutMap)
	got := modelEffortOptions("pig", reasoningName, []string{reasoningName})
	want := pigModelEffortOptions(*reasoningWithoutMap)
	if !slices.Equal(got, want) || len(got) == 0 {
		t.Errorf("%s efforts = %v, want supported reasoning levels %v", reasoningName, got, want)
	}
	max := reasoningWithoutMap.ToCapabilities().MaxThinking
	for _, effort := range got {
		if ai.CompareThinkingLevels(ai.ModelThinkingLevel(effort), max) > 0 {
			t.Errorf("%s offers %q above MaxThinking %q", reasoningName, effort, max)
		}
	}
}
