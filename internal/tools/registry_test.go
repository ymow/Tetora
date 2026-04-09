package tools

import (
	"testing"
)

func TestRegistrySearchBM25(t *testing.T) {
	r := NewRegistry()

	r.Register(&ToolDef{
		Name:        "web_search",
		Description: "Search the web using Google or Bing. Returns top results with snippets.",
	})
	r.Register(&ToolDef{
		Name:        "memory_search",
		Description: "Search your personal memory store for past notes and facts.",
	})
	r.Register(&ToolDef{
		Name:        "knowledge_search",
		Description: "Search the knowledge base for indexed files and documents.",
	})
	r.Register(&ToolDef{
		Name:        "exec",
		Description: "Execute a shell command and return stdout, stderr, and exit code.",
	})
	r.Register(&ToolDef{
		Name:        "file_read",
		Description: "Read contents of a file from the filesystem.",
	})
	r.Register(&ToolDef{
		Name:        "agent_dispatch",
		Description: "Dispatch a task to a specific agent role for execution.",
	})

	tests := []struct {
		query   string
		topN    int
		wantTop string
	}{
		{"web search", 3, "web_search"},
		{"memory", 3, "memory_search"},
		{"execute shell command", 3, "exec"},
		{"read file contents", 3, "file_read"},
		{"dispatch task to agent", 3, "agent_dispatch"},
		{"knowledge documents", 3, "knowledge_search"},
	}

	for _, tc := range tests {
		results := r.SearchBM25(tc.query, tc.topN)
		if len(results) == 0 {
			t.Errorf("SearchBM25(%q): no results", tc.query)
			continue
		}
		if results[0].Tool.Name != tc.wantTop {
			t.Errorf("SearchBM25(%q): top result = %q, want %q (scores: %+v)", tc.query, results[0].Tool.Name, tc.wantTop, results)
		}
	}
}

func TestRegistrySearchBM25EmptyQuery(t *testing.T) {
	r := NewRegistry()
	r.Register(&ToolDef{Name: "test", Description: "A test tool"})

	results := r.SearchBM25("", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty query, got %d", len(results))
	}
}

func TestRegistrySearchBM25NoTools(t *testing.T) {
	r := NewRegistry()
	results := r.SearchBM25("anything", 5)
	// Should not panic, returns nil or empty
	_ = results
}

func TestRegistryBM25IndexRebuildOnRegister(t *testing.T) {
	r := NewRegistry()

	// Register first tool, search should work
	r.Register(&ToolDef{Name: "tool_a", Description: "Search web results"})
	results := r.SearchBM25("web", 5)
	if len(results) != 1 || results[0].Tool.Name != "tool_a" {
		t.Errorf("expected tool_a, got %+v", results)
	}

	// Register second tool, search should include both
	r.Register(&ToolDef{Name: "tool_b", Description: "Search memory"})
	results = r.SearchBM25("search", 5)
	if len(results) != 2 {
		t.Errorf("expected 2 results after registering tool_b, got %d: %+v", len(results), results)
	}
}

func TestRegistrySearchBM25WithKeywords(t *testing.T) {
	r := NewRegistry()
	r.Register(&ToolDef{
		Name:        "msg_send",
		Description: "Send a message to a contact.",
		Keywords:    []string{"email", "notification", "communicate"},
	})

	// Searching for "email" should match via keywords even though description doesn't contain it
	results := r.SearchBM25("send email", 5)
	if len(results) == 0 {
		t.Errorf("expected results for 'send email' via keywords, got none")
	}
	if len(results) > 0 && results[0].Tool.Name != "msg_send" {
		t.Errorf("expected msg_send, got %q", results[0].Tool.Name)
	}
}
