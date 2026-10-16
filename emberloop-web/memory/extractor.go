package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	ExtractorPromptVersion = "x1"
	DefaultExtractorModel  = "inclusionai/ling-3.0-flash"
	OpenRouterChatEndpoint = "https://openrouter.ai/api/v1/chat/completions"
)

const ExtractorPromptX1 = `You extract long-term memories about a user from messages they sent to a coding agent.
Extract ONLY persistent, non-transient items: standing preferences, rules for future work,
decisions with their choice, and durable project or environment facts.
Ignore one-off task requests, bug reports, questions, status, and chat.
Write each item as one self-contained sentence (max 200 chars) that keeps the user's meaning
exactly, never inverted. Quote the user's own words that support it, verbatim, in "quote".
If nothing qualifies return {"items":[]}.
Return only JSON: {"items":[{"kind":"preference|instruction|decision|fact","text":"...","quote":"...","event":N,"stated":true}]}`

type ExtractEvent struct {
	Number int    `json:"number"`
	ID     string `json:"id"`
	Text   string `json:"text"`
}
type ExtractRequest struct {
	Events           []ExtractEvent
	AssistantContext string
}
type ExtractedItem struct {
	Kind   string `json:"kind"`
	Text   string `json:"text"`
	Quote  string `json:"quote"`
	Event  int    `json:"event"`
	Stated bool   `json:"stated"`
}
type Extractor interface {
	Extract(context.Context, ExtractRequest) ([]ExtractedItem, error)
}

type ExtractorError struct {
	StatusCode int
	Err        error
}

func (e *ExtractorError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("extractor HTTP %d", e.StatusCode)
}
func (e *ExtractorError) Unwrap() error { return e.Err }

type OpenRouterExtractor struct {
	APIKey    string
	Model     string
	Effort    string
	Endpoint  string
	Client    *http.Client
	Sleep     func(context.Context, time.Duration) error
	costMu    sync.RWMutex
	costUSD   float64
	costKnown bool
}

// CostUSD reports the provider-reported cost for the last successful API response.
func (o *OpenRouterExtractor) CostUSD() (float64, bool) {
	o.costMu.RLock()
	defer o.costMu.RUnlock()
	return o.costUSD, o.costKnown
}

func NewOpenRouterExtractor(key, model, effort string) *OpenRouterExtractor {
	if strings.TrimSpace(model) == "" {
		model = DefaultExtractorModel
	}
	return &OpenRouterExtractor{APIKey: key, Model: model, Effort: effort, Endpoint: OpenRouterChatEndpoint, Client: &http.Client{Timeout: 30 * time.Second}, Sleep: sleepContext}
}
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Reasoning   struct {
		Effort string `json:"effort"`
	} `json:"reasoning"`
	Usage struct {
		Include bool `json:"include"`
	} `json:"usage"`
}
type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		Cost *float64 `json:"cost"`
	} `json:"usage"`
}
type extractorJSON struct {
	Items []ExtractedItem `json:"items"`
}

func (o *OpenRouterExtractor) Extract(ctx context.Context, input ExtractRequest) ([]ExtractedItem, error) {
	if strings.TrimSpace(o.APIKey) == "" {
		return nil, errors.New("memory extractor: OpenRouter API key is empty")
	}
	if strings.TrimSpace(o.Model) == "" {
		return nil, errors.New("memory extractor: model is empty")
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	endpoint := o.Endpoint
	if endpoint == "" {
		endpoint = OpenRouterChatEndpoint
	}
	payload := chatRequest{Model: o.Model, Temperature: 0, MaxTokens: 4000, Messages: []chatMessage{{Role: "system", Content: ExtractorPromptX1}, {Role: "user", Content: ForDistill(formatExtractInput(input))}}}
	payload.Reasoning.Effort = o.Effort
	payload.Usage.Include = true
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+o.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			if attempt == 0 && isTimeout(err) {
				if err = o.pause(ctx); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			if attempt == 0 && isTimeout(readErr) {
				if err = o.pause(ctx); err != nil {
					return nil, err
				}
				continue
			}
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt == 0 {
				if err = o.pause(ctx); err != nil {
					return nil, err
				}
				continue
			}
			return nil, &ExtractorError{StatusCode: resp.StatusCode, Err: fmt.Errorf("OpenRouter extractor: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &ExtractorError{StatusCode: resp.StatusCode, Err: fmt.Errorf("OpenRouter extractor: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))}
		}
		var result chatResponse
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("decode OpenRouter response: %w", err)
		}
		if result.Usage.Cost != nil {
			o.costMu.Lock()
			o.costUSD = *result.Usage.Cost
			o.costKnown = true
			o.costMu.Unlock()
		}
		if len(result.Choices) == 0 {
			return nil, errors.New("OpenRouter extractor: no choices")
		}
		var output extractorJSON
		if err := json.Unmarshal([]byte(result.Choices[0].Message.Content), &output); err != nil {
			return nil, fmt.Errorf("decode extractor JSON: %w", err)
		}
		if len(output.Items) > 5 {
			output.Items = output.Items[:5]
		}
		return output.Items, nil
	}
	return nil, errors.New("OpenRouter extractor: retry exhausted")
}
func (o *OpenRouterExtractor) pause(ctx context.Context) error {
	if o.Sleep != nil {
		return o.Sleep(ctx, 2*time.Second)
	}
	return sleepContext(ctx, 2*time.Second)
}

type netError interface {
	error
	Timeout() bool
}

func isTimeout(err error) bool { var n netError; return errors.As(err, &n) && n.Timeout() }
func formatExtractInput(in ExtractRequest) string {
	var b strings.Builder
	for _, ev := range in.Events {
		fmt.Fprintf(&b, "Event %d (user): %s\n", ev.Number, ForDistill(ev.Text))
	}
	if in.AssistantContext != "" {
		fmt.Fprintf(&b, "\ncontext only, never quote it: %s", ForDistill(in.AssistantContext))
	}
	return b.String()
}

// ScriptedExtractor returns queued answers/errors and records calls for deterministic tests.
type ScriptedExtractor struct {
	Results [][]ExtractedItem
	Errors  []error
	Calls   int
}

func (s *ScriptedExtractor) Extract(_ context.Context, _ ExtractRequest) ([]ExtractedItem, error) {
	i := s.Calls
	s.Calls++
	if i < len(s.Errors) && s.Errors[i] != nil {
		return nil, s.Errors[i]
	}
	if i < len(s.Results) {
		return append([]ExtractedItem(nil), s.Results[i]...), nil
	}
	return nil, nil
}

// ValidateExtractedItems drops invalid candidates independently; validation never calls the model.
func ValidateExtractedItems(items []ExtractedItem, events []ExtractEvent) (valid, dropped []ExtractedItem) {
	byNumber := make(map[int]ExtractEvent, len(events))
	numberCounts := make(map[int]int, len(events))
	idCounts := make(map[string]int, len(events))
	for _, ev := range events {
		numberCounts[ev.Number]++
		idCounts[ev.ID]++
	}
	for _, ev := range events {
		if ev.Number > 0 && ev.ID != "" && numberCounts[ev.Number] == 1 && idCounts[ev.ID] == 1 {
			byNumber[ev.Number] = ev
		}
	}
	for _, item := range items {
		ev, ok := byNumber[item.Event]
		if !ok || !validMemoryKind(item.Kind) || strings.TrimSpace(item.Text) == "" || utf8.RuneCountInString(item.Text) > 300 || item.Quote == "" || !strings.Contains(ev.Text, item.Quote) || Redact(item.Text) != item.Text {
			dropped = append(dropped, item)
			continue
		}
		valid = append(valid, item)
	}
	return
}

func uniqueExtractEvents(events []ExtractEvent) []ExtractEvent {
	numbers, ids := make(map[int]int, len(events)), make(map[string]int, len(events))
	for _, ev := range events {
		numbers[ev.Number]++
		ids[ev.ID]++
	}
	out := make([]ExtractEvent, 0, len(events))
	for _, ev := range events {
		if ev.Number > 0 && ev.ID != "" && numbers[ev.Number] == 1 && ids[ev.ID] == 1 {
			out = append(out, ev)
		}
	}
	return out
}

// ExtractWithFallback uses the configured extractor when enabled; on an error or an empty setting it
// falls through to the legacy heuristic, whose results are inferred and deliberately low-importance.
func ExtractWithFallback(ctx context.Context, extractor Extractor, model string, input ExtractRequest) ([]MemoryItem, error) {
	// Validation and grounding use the exact redacted text sent to the model.
	safe := input
	safe.Events = uniqueExtractEvents(input.Events)
	for i := range safe.Events {
		safe.Events[i].Text = ForDistill(safe.Events[i].Text)
	}
	safe.AssistantContext = ForDistill(safe.AssistantContext)
	if strings.TrimSpace(model) != "" && extractor != nil {
		items, err := extractor.Extract(ctx, safe)
		if err == nil {
			valid, _ := ValidateExtractedItems(items, safe.Events)
			out := make([]MemoryItem, 0, len(valid))
			for _, it := range valid {
				if len(out) == 5 {
					break
				}
				v := VeracityStated
				if !it.Stated {
					v = VeracityInferred
				}
				out = append(out, MemoryItem{Kind: it.Kind, Text: it.Text, Quote: it.Quote, EventID: eventID(safe.Events, it.Event), Veracity: v, Importance: MemoryImportance(it.Text, it.Stated)})
			}
			return out, nil
		}
	}
	var fallback []MemoryItem
	for _, ev := range safe.Events {
		if _, ok := ClassifyHeuristic(ev.Text); !ok {
			continue
		}
		text := strings.TrimSpace(ev.Text)
		if text == "" || utf8.RuneCountInString(text) > 300 {
			continue
		}
		fallback = append(fallback, MemoryItem{Kind: KindPreference, Text: text, Quote: text, EventID: ev.ID, Veracity: VeracityInferred, Importance: 0.4})
		if len(fallback) == 5 {
			break
		}
	}
	return fallback, nil
}
func eventID(events []ExtractEvent, n int) string {
	for _, ev := range events {
		if ev.Number == n {
			return ev.ID
		}
	}
	return ""
}
func MemoryImportance(text string, stated bool) float64 {
	if !stated {
		return 0.5
	}
	lower := strings.ToLower(text)
	for _, phrase := range []string{"always", "never", "from now on", "in this repo", "we decided"} {
		if strings.Contains(lower, phrase) {
			return 0.85
		}
	}
	return 0.7
}
