// Models: configurable catalog served on every discovery endpoint.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// Model is one fake catalog entry.
type Model struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	PromptPrice   string `json:"prompt_price,omitempty"`
	CompPrice     string `json:"comp_price,omitempty"`
}

// defaultModels is the built-in catalog when --models is unset.
func defaultModels() []Model {
	return []Model{
		{ID: "fake-small", Name: "Fake Small", ContextLength: 32000, PromptPrice: "0", CompPrice: "0"},
		{ID: "fake-tools", Name: "Fake Tools", ContextLength: 64000, PromptPrice: "0", CompPrice: "0"},
		{ID: "openai/gpt-fake-1", Name: "GPT Fake 1", ContextLength: 128000, PromptPrice: "0", CompPrice: "0"},
		{ID: "anthropic/claude-fake-1", Name: "Claude Fake 1", ContextLength: 200000, PromptPrice: "0", CompPrice: "0"},
		{ID: "google/gemini-fake-1", Name: "Gemini Fake 1", ContextLength: 1000000, PromptPrice: "0", CompPrice: "0"},
		{ID: "openrouter/fake-large", Name: "Fake Large", ContextLength: 256000, PromptPrice: "0", CompPrice: "0"},
	}
}

// LoadModels reads a JSON catalog file. Accepted shapes:
//
//	["id-a", "id-b"]
//	[{"id": "...", "name": "...", "context_length": N,
//	  "pricing": {"prompt": "0", "completion": "0"}}]
//	{"data": [...]}  (OpenAI list wrapper)
//	{"models": [...]} (pi.dev wrapper)
func LoadModels(path string) ([]Model, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var asStrings []string
	if err := json.Unmarshal(b, &asStrings); err == nil {
		out := make([]Model, 0, len(asStrings))
		for _, id := range asStrings {
			if id != "" {
				out = append(out, Model{ID: id, Name: id, ContextLength: 128000, PromptPrice: "0", CompPrice: "0"})
			}
		}
		return out, nil
	}
	var wrapped struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && (wrapped.Data != nil || wrapped.Models != nil) {
		raw := wrapped.Data
		if raw == nil {
			raw = wrapped.Models
		}
		return decodeModelEntries(raw), nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, fmt.Errorf("models file: expected array, object with data/models, or string array")
	}
	return decodeModelEntries(arr), nil
}

func decodeModelEntries(raw []json.RawMessage) []Model {
	out := make([]Model, 0, len(raw))
	for _, e := range raw {
		var s string
		if err := json.Unmarshal(e, &s); err == nil {
			if s != "" {
				out = append(out, Model{ID: s, Name: s, ContextLength: 128000, PromptPrice: "0", CompPrice: "0"})
			}
			continue
		}
		var o struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			MaxTokens     int    `json:"max_tokens"`
			Pricing       *struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
			Cost *struct {
				Input  float64 `json:"input"`
				Output float64 `json:"output"`
			} `json:"cost"`
		}
		if err := json.Unmarshal(e, &o); err != nil || o.ID == "" {
			continue
		}
		m := Model{ID: o.ID, Name: o.Name, ContextLength: o.ContextLength, PromptPrice: "0", CompPrice: "0"}
		if m.Name == "" {
			m.Name = m.ID
		}
		if m.ContextLength == 0 {
			m.ContextLength = o.MaxTokens
		}
		if m.ContextLength == 0 {
			m.ContextLength = 128000
		}
		if o.Pricing != nil {
			m.PromptPrice = o.Pricing.Prompt
			m.CompPrice = o.Pricing.Completion
		}
		out = append(out, m)
	}
	return out
}

func (s *Server) modelIDs() []string {
	ids := make([]string, 0, len(s.models))
	for _, m := range s.models {
		ids = append(ids, m.ID)
	}
	return ids
}

// bumpCatalogGet records a catalog fetch for /__stats.
func (s *Server) bumpCatalogGet() {
	s.mu.Lock()
	s.catalogGets++
	s.mu.Unlock()
}

func (s *Server) getModels() []Model {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Model(nil), s.models...)
}

// handleOpenAIModels serves GET /v1/models (OpenAI-compatible).
func (s *Server) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	s.record(r, "", nil, false, "")
	s.bumpCatalogGet()
	data := make([]any, 0, len(s.getModels()))
	for _, m := range s.getModels() {
		data = append(data, map[string]any{
			"id":       m.ID,
			"object":   "model",
			"created":  1700000000,
			"owned_by": "fakellm",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

// handleOpenRouterModels serves GET /api/v1/models (OpenRouter-style with
// context_length and pricing so discovery harnesses work).
func (s *Server) handleOpenRouterModels(w http.ResponseWriter, r *http.Request) {
	s.record(r, "", nil, false, "")
	s.bumpCatalogGet()
	data := make([]any, 0, len(s.getModels()))
	for _, m := range s.getModels() {
		pp := m.PromptPrice
		cp := m.CompPrice
		if pp == "" {
			pp = "0"
		}
		if cp == "" {
			cp = "0"
		}
		cl := m.ContextLength
		if cl == 0 {
			cl = 128000
		}
		nm := m.Name
		if nm == "" {
			nm = m.ID
		}
		data = append(data, map[string]any{
			"id":             m.ID,
			"name":           nm,
			"context_length": cl,
			"pricing":        map[string]any{"prompt": pp, "completion": cp},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// providerOf guesses a models.dev provider key from a model id.
func providerOf(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	if i := strings.Index(id, "-"); i > 0 {
		return id[:i]
	}
	return "fake"
}

// handleModelsDev serves GET /api.json in a models.dev-style shape:
// {provider: {models: {id: {id, name, cost, limit}}}}.
func (s *Server) handleModelsDev(w http.ResponseWriter, r *http.Request) {
	s.record(r, "", nil, false, "")
	s.bumpCatalogGet()
	out := map[string]any{}
	for _, m := range s.getModels() {
		prov := providerOf(m.ID)
		nm := m.Name
		if nm == "" {
			nm = m.ID
		}
		cl := m.ContextLength
		if cl == 0 {
			cl = 128000
		}
		entry := map[string]any{
			"id":   m.ID,
			"name": nm,
			"cost": map[string]any{"input": 0, "output": 0},
			"limit": map[string]any{
				"context": cl,
				"output":  4096,
			},
			"tool_call": true,
		}
		pm, ok := out[prov].(map[string]any)
		if !ok {
			pm = map[string]any{"models": map[string]any{}}
			out[prov] = pm
		}
		pm["models"].(map[string]any)[m.ID] = entry
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePiProvider serves GET /api/models/providers/<provider>?types=...
// in the pi.dev shape catalogsync.go reads: either a bare JSON array of
// entries carrying "id", or {"models": [...]}. Entries are tagged with the
// requested provider.
func (s *Server) handlePiProvider(w http.ResponseWriter, r *http.Request) {
	const prefix = "/api/models/providers/"
	prov := strings.TrimPrefix(r.URL.Path, prefix)
	if i := strings.Index(prov, "/"); i >= 0 {
		prov = prov[:i]
	}
	prov, _ = url.PathUnescape(prov)
	s.record(r, "", nil, false, "")
	s.bumpCatalogGet()
	if prov == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing provider"})
		return
	}
	models := s.getModels()
	var matched []Model
	for _, m := range models {
		if providerOf(m.ID) == prov || m.ID == prov ||
			strings.HasPrefix(m.ID, prov+"/") || strings.HasPrefix(m.ID, prov+"-") {
			matched = append(matched, m)
		}
	}
	if len(matched) == 0 {
		// Tag everything with the requested provider so a benchmark
		// catalog refresh never blanks the store.
		matched = append([]Model(nil), models...)
	}
	entries := make([]any, 0, len(matched))
	for _, m := range matched {
		nm := m.Name
		if nm == "" {
			nm = m.ID
		}
		entries = append(entries, map[string]any{
			"id":       m.ID,
			"name":     nm,
			"provider": prov,
		})
	}
	writeJSON(w, http.StatusOK, entries)
}

// handleCatalog serves the mutable catalog: GET /__catalog reads the live
// list, PUT /__catalog replaces it. PUT accepts the same shapes as the
// --models file: a string array, an object array, or {"data":[...]} /
// {"models":[...]} wrappers. Every catalog GET bumps catalog_fetches in
// /__stats so catalog-refresh behaviour of agents can be measured.
func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.bumpCatalogGet()
		data := make([]any, 0, len(s.getModels()))
		for _, m := range s.getModels() {
			nm := m.Name
			if nm == "" {
				nm = m.ID
			}
			cl := m.ContextLength
			if cl == 0 {
				cl = 128000
			}
			data = append(data, map[string]any{
				"id":             m.ID,
				"name":           nm,
				"context_length": cl,
				"pricing":        map[string]any{"prompt": m.PromptPrice, "completion": m.CompPrice},
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": data})
	case http.MethodPut:
		b, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body"})
			return
		}
		models, err := decodeCatalogBody(b)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.mu.Lock()
		s.models = models
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"data": len(models)})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// decodeCatalogBody parses a PUT /__catalog body with LoadModels shapes.
func decodeCatalogBody(b []byte) ([]Model, error) {
	var asStrings []string
	if err := json.Unmarshal(b, &asStrings); err == nil {
		out := make([]Model, 0, len(asStrings))
		for _, id := range asStrings {
			if id != "" {
				out = append(out, Model{ID: id, Name: id, ContextLength: 128000, PromptPrice: "0", CompPrice: "0"})
			}
		}
		return out, nil
	}
	var wrapped struct {
		Data   []json.RawMessage `json:"data"`
		Models []json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && (wrapped.Data != nil || wrapped.Models != nil) {
		raw := wrapped.Data
		if raw == nil {
			raw = wrapped.Models
		}
		return decodeModelEntries(raw), nil
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil {
		return nil, fmt.Errorf("catalog: expected array or object with data/models")
	}
	return decodeModelEntries(arr), nil
}
