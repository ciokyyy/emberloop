// Package systemone is the small client for OpenRouter's /v1/systemone
// endpoint, the one place a decisions model (Jev) can be asked a typed
// question. A chat-completions call to a decisions model returns 400.
//
// Ported from wick's apps/backend/systemone, keeping only the noul question
// the find cascade asks.
package systemone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	endpoint  = "https://openrouter.ai/api/v1/systemone"
	modelsURL = "https://openrouter.ai/api/v1/models?output_modalities=decisions"

	// JevModelPinned is the versioned Jev selector. Pinned, not floating: a
	// threshold measured against one release means nothing against the next.
	JevModelPinned = "typesafe/jev-1.13"
	// jevModelLatest is OpenRouter's rolling alias. It is resolved to the pin,
	// because a stored alias that follows whatever ships next changes behaviour
	// without anyone changing the setting.
	jevModelLatest = "~typesafe/jev-latest"
)

// CanonicalModel rewrites a rolling alias onto the pin and passes any other
// id through: a decisions model released after this build is a legitimate pick.
func CanonicalModel(selector string) string {
	switch strings.TrimSpace(selector) {
	case jevModelLatest, "typesafe/jev-latest":
		return JevModelPinned
	}
	return selector
}

type QuestionType string

// TypeNoul is a calibrated probability judgment, 0.0 to 1.0.
const TypeNoul QuestionType = "noul"

type Question struct {
	Type         QuestionType `json:"type"`
	Instructions string       `json:"instructions"`
	Criteria     any          `json:"criteria,omitempty"`
}

type Answer struct {
	Type QuestionType `json:"type"`
	Noul *float64     `json:"noul,omitempty"`
}

type Request struct {
	Model     string              `json:"model"`
	State     any                 `json:"state"`
	Questions map[string]Question `json:"questions"`
}

type Response struct {
	Answers map[string]Answer `json:"answers"`
}

// Provider answers a Request. The find cascade takes this, so a test can
// stand in for the network.
type Provider interface {
	Evaluate(ctx context.Context, req Request) (Response, error)
}

// OpenRouter is the hosted Provider.
type OpenRouter struct {
	APIKey string
	client http.Client
	sem    chan struct{}
}

// NewOpenRouter builds a provider that runs at most 16 requests at once.
func NewOpenRouter(apiKey string) *OpenRouter {
	return &OpenRouter{APIKey: apiKey, client: http.Client{Timeout: 30 * time.Second}, sem: make(chan struct{}, 16)}
}

func (o *OpenRouter) Evaluate(ctx context.Context, req Request) (Response, error) {
	select {
	case o.sem <- struct{}{}:
		defer func() { <-o.sem }()
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("marshal systemone request: %w", err)
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+o.APIKey)
	resp, err := o.client.Do(hr)
	if err != nil {
		return Response{}, fmt.Errorf("openrouter systemone: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return Response{}, fmt.Errorf("openrouter systemone (status %d): %s", resp.StatusCode, msg)
	}
	var out Response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Response{}, fmt.Errorf("decode systemone response: %w", err)
	}
	return out, nil
}

// APIKey finds the OpenRouter key: OPENROUTER_API_KEY first, then the one pig
// already holds. Empty means none, and find falls back to lexical search.
func APIKey(ctx context.Context) string {
	if k := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")); k != "" {
		return k
	}
	out, err := exec.CommandContext(ctx, "pig", "auth", "print-api-key", "--provider", "openrouter").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ListModels returns the ids of OpenRouter's decisions-modality models. The
// listing is public, so no key is needed.
func ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("openrouter models: status %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}
