package bm25

import (
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"hello world", []string{"hello", "world"}},
		{"search_tools", []string{"search", "tools"}},
		{"Execute a shell command", []string{"execute", "a", "shell", "command"}},
		{"memory_search - Search memory", []string{"memory", "search", "search", "memory"}},
		{"", nil},
		{"123", []string{"123"}},
		{"tool-name_test", []string{"tool", "name", "test"}},
	}

	for _, tc := range tests {
		got := Tokenize(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("Tokenize(%q) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Tokenize(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestBM25Search(t *testing.T) {
	docs := []Document{
		{ID: "web_search", Terms: Tokenize("Search the web using Google or Bing. Returns top results with snippets.")},
		{ID: "memory_search", Terms: Tokenize("Search your personal memory store for past notes and facts. Returns matching entries.")},
		{ID: "knowledge_search", Terms: Tokenize("Search the knowledge base for indexed files and documents.")},
		{ID: "exec", Terms: Tokenize("Execute a shell command and return stdout, stderr, and exit code.")},
		{ID: "file_read", Terms: Tokenize("Read contents of a file from the filesystem. Supports line ranges.")},
		{ID: "web_fetch", Terms: Tokenize("Fetch content from a URL and extract readable text.")},
		{ID: "agent_dispatch", Terms: Tokenize("Dispatch a task to a specific agent role for execution.")},
		{ID: "task_create", Terms: Tokenize("Create a new task in the task management system with title and description.")},
	}

	bm := New(docs, DefaultK1, DefaultB)

	tests := []struct {
		query    string
		wantTop  string // expected top result ID
		minScore float64
	}{
		{"web search", "web_search", 0.1},
		{"memory", "memory_search", 0.1},
		{"execute command", "exec", 0.1},
		{"read file", "file_read", 0.1},
		{"fetch url", "web_fetch", 0.1},
		{"dispatch agent task", "agent_dispatch", 0.1},
		{"search knowledge", "knowledge_search", 0.1},
	}

	for _, tc := range tests {
		terms := Tokenize(tc.query)
		results := bm.Search(terms, 3)
		if len(results) == 0 {
			t.Errorf("Search(%q): no results", tc.query)
			continue
		}
		if results[0].ID != tc.wantTop {
			t.Errorf("Search(%q): top result = %q, want %q (scores: %+v)", tc.query, results[0].ID, tc.wantTop, results)
		}
		if results[0].Score < tc.minScore {
			t.Errorf("Search(%q): score %f < minimum %f", tc.query, results[0].Score, tc.minScore)
		}
	}
}

func TestBM25NoResults(t *testing.T) {
	docs := []Document{
		{ID: "tool_a", Terms: Tokenize("Search web results")},
		{ID: "tool_b", Terms: Tokenize("Read files from disk")},
	}
	bm := New(docs, DefaultK1, DefaultB)

	results := bm.Search([]string{"nonexistent"}, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestBM25EmptyIndex(t *testing.T) {
	bm := New(nil, DefaultK1, DefaultB)
	results := bm.Search([]string{"anything"}, 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestBM25TopNLimit(t *testing.T) {
	docs := []Document{
		{ID: "a", Terms: Tokenize("search web")},
		{ID: "b", Terms: Tokenize("search memory")},
		{ID: "c", Terms: Tokenize("search knowledge")},
		{ID: "d", Terms: Tokenize("search files")},
		{ID: "e", Terms: Tokenize("search documents")},
	}
	bm := New(docs, DefaultK1, DefaultB)

	results := bm.Search([]string{"search"}, 2)
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestBM25ScoreOrdering(t *testing.T) {
	// BM25 favors documents where the query term is proportionally more important.
	// A short doc with the term once can beat a long doc with the term many times
	// due to length normalization. So test with clear frequency difference.
	docs := []Document{
		{ID: "once", Terms: Tokenize("web tool")},
		{ID: "twice", Terms: Tokenize("web search web results")},
	}
	bm := New(docs, DefaultK1, DefaultB)

	results := bm.Search([]string{"web"}, 2)
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// "twice" has higher TF for "web" (2 vs 1) and similar doc length, so it ranks higher.
	if results[0].ID != "twice" {
		t.Errorf("expected 'twice' first, got %q (scores: %+v)", results[0].ID, results)
	}
}
