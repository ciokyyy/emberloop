package main

import (
	"strings"
	"time"
)

// ScenarioResult is one scenario's row in the JSON report.
type ScenarioResult struct {
	Name      string         `json:"name"`
	Verdict   string         `json:"verdict"`
	LatencyMs int64          `json:"latencyMs"`
	Details   map[string]any `json:"details,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// MemoryReport is the RAM section of the JSON report.
type MemoryReport struct {
	SampleIntervalMs        int64                `json:"sampleIntervalMs"`
	Samples                 int                  `json:"samples"`
	Phases                  map[string]phaseStat `json:"phases,omitempty"`
	PerSessionMarginalBytes *int64               `json:"perSessionMarginalBytes,omitempty"`
	Note                    string               `json:"note,omitempty"`
}

// BenchReport is the top-level JSON document acpbench emits.
// SchemaVersion is the stable machine-readable schema marker (compare
// refuses files whose schemaVersion it does not know).
type BenchReport struct {
	Tool          string           `json:"tool"`
	Version       string           `json:"version"`
	SchemaVersion int              `json:"schemaVersion"`
	StartedAt     time.Time        `json:"startedAt"`
	DurationMs    int64            `json:"durationMs"`
	Agent         map[string]any   `json:"agent"`
	Scenarios     []ScenarioResult `json:"scenarios"`
	Memory        MemoryReport     `json:"memory"`
	Summary       map[string]int   `json:"summary"`
}

func summarize(results []ScenarioResult) map[string]int {
	sum := map[string]int{"total": len(results)}
	for _, r := range results {
		sum[r.Verdict]++
	}
	return sum
}

// turnOutcome is the classifier input: what a prompt turn produced.
type turnOutcome struct {
	err         error
	timedOut    bool // client-side timeout fired
	crashed     bool // agent process is gone
	stopReason  string
	updates     int
	sawToolCall bool
	sawChunk    bool
	expectTool  bool // the scripted prompt should call the echo tool
	expectText  string
	gotText     string
}

// classifyTurn maps a finished turn to exactly one verdict.
func classifyTurn(o turnOutcome) string {
	if o.crashed {
		return VerdictCrash
	}
	if o.timedOut {
		return VerdictTimeoutHang
	}
	if o.err != nil {
		if isMethodNotFound(o.err) {
			return VerdictProtocolError
		}
		s := o.err.Error()
		if strings.Contains(s, "adapter closed") {
			return VerdictCrash
		}
		if strings.Contains(s, "timed out after") {
			return VerdictTimeoutHang
		}
		if strings.Contains(s, "no sessionId") || strings.Contains(s, "unexpected") {
			return VerdictProtocolError
		}
		return VerdictProtocolError
	}
	if o.updates == 0 || (!o.sawToolCall && !o.sawChunk) {
		return VerdictSilentEmptyTurn
	}
	if o.expectTool && !o.sawToolCall {
		return VerdictWrongResult
	}
	if o.expectText != "" && !strings.Contains(o.gotText, o.expectText) {
		return VerdictWrongResult
	}
	return VerdictOK
}
