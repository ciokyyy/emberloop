package el

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Model is one model a harness offers, and the effort levels that model takes.
// Label is what a picker shows; it is empty when the harness has no name beyond
// the id.
type Model struct {
	ID      string   `json:"id"`
	Label   string   `json:"label,omitempty"`
	Efforts []string `json:"efforts,omitempty"`
}

// pigEffortLevels are the thinking levels pig takes. A pig model that thinks
// gets all of them.

// pigBin is the pig CLI, overridable for tests.
func pigBin() string {
	if p := os.Getenv("EL_PIG"); p != "" {
		return p
	}
	return "pig"
}

var pigEffortLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// Models lists a harness's models through the Exec seam, so a test can feed it
// captured CLI output.
func (pigHarness) Models(run Exec) ([]Model, error) {
	res, err := run(pigBin(), []string{"--list-models"}, ExecOpts{Timeout: time.Minute})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("pig --list-models: %s", strings.TrimSpace(res.Stderr))
	}
	return parsePigModels(res.Stdout), nil
}

// parsePigModels reads the fixed-width table `pig --list-models` prints. The
// header names the columns and fixes their offsets, so a blank cell does not
// shift the thinking column into the images one. A model's id keeps its
// provider: pig routes a bare id to openai, so `deepseek-flash` cannot be sent
// without the `deepseek/` in front of it. A thinking model takes pig's levels;
// the others take none.
func parsePigModels(out string) []Model {
	var cols []int
	var models []Model
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "provider") && strings.Contains(line, "thinking") {
			cols = columnStarts(line)
			continue
		}
		if cols == nil || strings.TrimSpace(line) == "" {
			continue
		}
		cell := func(j int) string {
			if j >= len(cols) || cols[j] >= len(line) {
				return ""
			}
			end := len(line)
			if j+1 < len(cols) && cols[j+1] < end {
				end = cols[j+1]
			}
			return strings.TrimSpace(line[cols[j]:end])
		}
		provider, model := cell(0), cell(1)
		if provider == "" || model == "" {
			continue
		}
		m := Model{ID: provider + "/" + model}
		if cell(4) == "yes" {
			m.Efforts = pigEffortLevels
		}
		models = append(models, m)
	}
	return models
}

// columnStarts is where each space-separated column of a fixed-width table
// header begins.
func columnStarts(header string) []int {
	var starts []int
	inColumn := false
	for i, r := range header {
		switch {
		case r != ' ' && !inColumn:
			starts = append(starts, i)
			inColumn = true
		case r == ' ':
			inColumn = false
		}
	}
	return starts
}

// claudeInitializeRequest is the control request that makes claude report what
// it can run. It is not a user message, so the answer costs nothing.
const claudeInitializeRequest = `{"type":"control_request","request_id":"el-models","request":{"subtype":"initialize"}}` + "\n"

// Models asks claude for its model list over stream-json. No prompt is sent and
// stdin closes, so the process answers and exits without a turn.
func (claudeHarness) Models(run Exec) ([]Model, error) {
	args := []string{
		"-p", "--input-format", "stream-json", "--output-format", "stream-json",
		"--setting-sources", "", "--no-session-persistence", "--verbose",
	}
	res, err := run("claude", args, ExecOpts{Timeout: time.Minute, Stdin: claudeInitializeRequest})
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("claude initialize: %s", strings.TrimSpace(res.Stderr))
	}
	return parseClaudeModels(res.Stdout)
}

// parseClaudeModels reads the models[] of an initialize control response.
func parseClaudeModels(out string) ([]Model, error) {
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			Type     string `json:"type"`
			Response struct {
				Subtype  string `json:"subtype"`
				Error    string `json:"error"`
				Response struct {
					Models []struct {
						Value                 string   `json:"value"`
						DisplayName           string   `json:"displayName"`
						SupportedEffortLevels []string `json:"supportedEffortLevels"`
					} `json:"models"`
				} `json:"response"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil || rec.Type != "control_response" {
			continue
		}
		if rec.Response.Subtype != "success" {
			return nil, fmt.Errorf("claude initialize failed: %s", rec.Response.Error)
		}
		models := make([]Model, 0, len(rec.Response.Response.Models))
		for _, m := range rec.Response.Response.Models {
			models = append(models, Model{ID: m.Value, Label: m.DisplayName, Efforts: m.SupportedEffortLevels})
		}
		return models, nil
	}
	return nil, fmt.Errorf("claude sent no initialize response")
}

// modelCache is one harness's model list and the time it was read.
type modelCache struct {
	Harness string    `json:"harness"`
	At      time.Time `json:"at"`
	Models  []Model   `json:"models"`
}

// modelCachePath is where a harness's list is cached, named for the harness.
func modelCachePath(harness string) string {
	return filepath.Join(filepath.Dir(statePath), "models", harness+".json")
}

// CachedModels is a harness's model list: the cache when there is one and
// refresh is false, a fresh reading otherwise. A rebuild that fails answers
// from the cache if there is one, so a broken CLI leaves pickers working.
func CachedModels(harness string, run Exec, refresh bool) ([]Model, error) {
	h, ok := registry[harness]
	if !ok {
		return nil, fmt.Errorf("unknown harness %q (have %v)", harness, HarnessNames())
	}
	cached, cachedErr := readModelCache(harness)
	if !refresh && cachedErr == nil {
		return cached, nil
	}
	models, err := h.Models(run)
	if err != nil {
		if cachedErr == nil {
			return cached, nil
		}
		return nil, err
	}
	writeModelCache(harness, models)
	return models, nil
}

func readModelCache(harness string) ([]Model, error) {
	raw, err := os.ReadFile(modelCachePath(harness))
	if err != nil {
		return nil, err
	}
	var c modelCache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return c.Models, nil
}

// writeModelCache is best effort: a list that could not be cached is still a
// list, and the next reader just rebuilds it.
func writeModelCache(harness string, models []Model) {
	raw, err := json.MarshalIndent(modelCache{Harness: harness, At: time.Now(), Models: models}, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(modelCachePath(harness))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	_ = os.WriteFile(modelCachePath(harness), append(raw, '\n'), 0o644)
}
