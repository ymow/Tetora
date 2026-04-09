// Package tools provides the tool registry and handler types for Tetora agents.
package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"tetora/internal/bm25"
	"tetora/internal/classify"
	"tetora/internal/config"
	"tetora/internal/provider"
)

// ToolDef defines a tool that can be called by agents.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Keywords    []string        `json:"-"` // Extra searchable keywords for BM25
	Handler     Handler         `json:"-"`
	Builtin     bool            `json:"-"`
	RequireAuth bool            `json:"requireAuth,omitempty"`
}

// ToolCall is an alias for provider.ToolCall.
type ToolCall = provider.ToolCall

// Result represents the result of a tool execution.
type Result struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// Handler is a function that executes a tool.
type Handler func(ctx context.Context, cfg *config.Config, input json.RawMessage) (string, error)

// Registry manages available tools.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]*ToolDef
	bm25Index  *bm25.BM25
}

// NewRegistry creates a new empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*ToolDef),
	}
}

// Register adds a tool to the registry and rebuilds the BM25 index.
func (r *Registry) Register(tool *ToolDef) {
	r.mu.Lock()
	r.tools[tool.Name] = tool
	r.rebuildBM25IndexLocked()
	r.mu.Unlock()
}

// rebuildBM25IndexLocked rebuilds the BM25 index from all registered tools.
// Must be called with r.mu held (write lock).
func (r *Registry) rebuildBM25IndexLocked() {
	docs := make([]bm25.Document, 0, len(r.tools))
	for _, t := range r.tools {
		// Build searchable text: name + description + keywords
		var parts []string
		// Split underscored names into separate terms for better matching
		nameTerms := strings.ReplaceAll(t.Name, "_", " ")
		parts = append(parts, nameTerms)
		parts = append(parts, t.Description)
		parts = append(parts, strings.Join(t.Keywords, " "))
		docs = append(docs, bm25.Document{
			ID:    t.Name,
			Terms: bm25.Tokenize(strings.Join(parts, " ")),
		})
	}
	r.bm25Index = bm25.New(docs, bm25.DefaultK1, bm25.DefaultB)
}

// Get retrieves a tool by name.
func (r *Registry) Get(name string) (*ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools.
func (r *Registry) List() []*ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ToolDef, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

// ListFiltered returns tools whose Name is in the allowed map.
// If allowed is nil or empty, returns all tools (backward compat).
func (r *Registry) ListFiltered(allowed map[string]bool) []*ToolDef {
	if len(allowed) == 0 {
		return r.List()
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ToolDef, 0, len(allowed))
	for _, t := range r.tools {
		if allowed[t.Name] {
			result = append(result, t)
		}
	}
	return result
}

// SearchResult holds a tool search result with its BM25 score.
type SearchResult struct {
	Tool  *ToolDef
	Score float64
}

// SearchBM25 searches tools using BM25 ranking. Returns results sorted by
// relevance score. If topN <= 0, returns all matching results.
func (r *Registry) SearchBM25(query string, topN int) []SearchResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.bm25Index == nil {
		return nil
	}

	terms := bm25.Tokenize(query)
	results := r.bm25Index.Search(terms, topN)

	out := make([]SearchResult, 0, len(results))
	for _, res := range results {
		if t, ok := r.tools[res.ID]; ok {
			out = append(out, SearchResult{Tool: t, Score: res.Score})
		}
	}
	return out
}

// Range calls fn for each tool in the registry. If fn returns false, iteration stops.
func (r *Registry) Range(fn func(*ToolDef) bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.tools {
		if !fn(t) {
			return
		}
	}
}

// ListForProvider serializes tools for API calls (no Handler field).
func (r *Registry) ListForProvider() []map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]map[string]any, 0, len(r.tools))
	for _, t := range r.tools {
		var schema map[string]any
		if len(t.InputSchema) > 0 {
			json.Unmarshal(t.InputSchema, &schema)
		}
		result = append(result, map[string]any{
			"name":         t.Name,
			"description":  t.Description,
			"input_schema": schema,
		})
	}
	return result
}

// --- Tool Profiles ---

// ProfileSets defines which tools are included in each profile.
var ProfileSets = map[string][]string{
	"minimal": {
		"memory_get", "memory_search", "knowledge_search",
		"web_search", "agent_dispatch",
	},
	"standard": {
		"memory_get", "memory_search", "memory_store", "memory_recall",
		"memory_um_search", "memory_forget",
		"knowledge_search", "web_search", "web_fetch",
		"agent_dispatch", "lesson_record",
		"task_create", "task_list", "task_update",
		"file_read", "file_write",
		"taskboard_list", "taskboard_get", "taskboard_create",
		"taskboard_move", "taskboard_comment", "taskboard_decompose",
	},
	// "full" = all tools (no filtering)
}

// ForProfile returns the allowed tool set for a given profile name.
// Returns nil for "full" or unknown profiles (which means all tools).
func ForProfile(profile string) map[string]bool {
	tools, ok := ProfileSets[profile]
	if !ok || profile == "full" {
		return nil // nil = all tools
	}
	allowed := make(map[string]bool, len(tools))
	for _, t := range tools {
		allowed[t] = true
	}
	return allowed
}

// ForComplexity returns the tool profile name appropriate for the given request complexity.
func ForComplexity(c classify.Complexity) string {
	switch c {
	case classify.Simple:
		return "none"
	case classify.Standard:
		return "standard"
	default:
		return "full"
	}
}
