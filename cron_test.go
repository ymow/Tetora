package main

import (
	"encoding/json"
	"testing"
)

// Expression parser tests are in internal/cron/expr_test.go.
// This file tests cron engine types that remain in package main.

// --- truncate tests ---

func TestTruncate_ShortString(t *testing.T) {
	s := "hello"
	result := truncate(s, 10)
	if result != "hello" {
		t.Errorf("got %q, want %q", result, "hello")
	}
}

func TestTruncate_LongString(t *testing.T) {
	s := "hello world, this is a long string"
	result := truncate(s, 10)
	want := "hello worl..."
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestTruncate_ExactLength(t *testing.T) {
	s := "hello"
	result := truncate(s, 5)
	if result != "hello" {
		t.Errorf("got %q, want %q", result, "hello")
	}
}

func TestTruncate_OneOver(t *testing.T) {
	s := "abcdef"
	result := truncate(s, 5)
	want := "abcde..."
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	result := truncate("", 10)
	if result != "" {
		t.Errorf("got %q, want %q", result, "")
	}
}

func TestTruncate_ZeroMaxLen(t *testing.T) {
	result := truncate("hello", 0)
	want := "..."
	if result != want {
		t.Errorf("got %q, want %q", result, want)
	}
}

// --- maxChainDepth constant ---

func TestMaxChainDepth(t *testing.T) {
	if maxChainDepth != 5 {
		t.Errorf("maxChainDepth = %d, want 5", maxChainDepth)
	}
}

// --- truncate table-driven ---

func TestTruncate_Table(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hi", 10, "hi"},
		{"exact", "hello", 5, "hello"},
		{"over by one", "abcdef", 5, "abcde..."},
		{"way over", "the quick brown fox jumps over", 10, "the quick ..."},
		{"empty", "", 5, ""},
		{"zero len", "abc", 0, "..."},
		{"one char max", "abc", 1, "a..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

// --- Per-job concurrency tests ---

func TestEffectiveMaxConcurrentRuns_Default(t *testing.T) {
	// MaxConcurrentRuns == 0 (unset) should default to 1.
	j := &cronJob{CronJobConfig: CronJobConfig{ID: "test", MaxConcurrentRuns: 0}}
	if got := j.effectiveMaxConcurrentRuns(); got != 1 {
		t.Errorf("expected 1 for unset MaxConcurrentRuns, got %d", got)
	}
}

func TestEffectiveMaxConcurrentRuns_Explicit(t *testing.T) {
	// Explicitly set value should be returned as-is.
	for _, want := range []int{1, 2, 5, 10} {
		j := &cronJob{CronJobConfig: CronJobConfig{ID: "test", MaxConcurrentRuns: want}}
		if got := j.effectiveMaxConcurrentRuns(); got != want {
			t.Errorf("MaxConcurrentRuns=%d: expected %d, got %d", want, want, got)
		}
	}
}

func TestEffectiveMaxConcurrentRuns_Negative(t *testing.T) {
	// Negative value (invalid) falls back to default 1.
	j := &cronJob{CronJobConfig: CronJobConfig{ID: "test", MaxConcurrentRuns: -1}}
	if got := j.effectiveMaxConcurrentRuns(); got != 1 {
		t.Errorf("expected 1 for negative MaxConcurrentRuns, got %d", got)
	}
}

func TestCronJobConfig_MaxConcurrentRuns_JSONRoundtrip(t *testing.T) {
	// Field absent → deserialises to 0; effectiveMaxConcurrentRuns() → 1 (default).
	var cfgAbsent CronJobConfig
	if err := json.Unmarshal([]byte(`{"id":"j1","name":"Job","enabled":true,"schedule":"* * * * *","task":{"prompt":"hi"}}`), &cfgAbsent); err != nil {
		t.Fatalf("unmarshal without maxConcurrentRuns: %v", err)
	}
	if cfgAbsent.MaxConcurrentRuns != 0 {
		t.Errorf("expected MaxConcurrentRuns=0 when field absent, got %d", cfgAbsent.MaxConcurrentRuns)
	}
	jAbsent := &cronJob{CronJobConfig: cfgAbsent}
	if jAbsent.effectiveMaxConcurrentRuns() != 1 {
		t.Errorf("expected effectiveMaxConcurrentRuns()=1 for absent field, got %d", jAbsent.effectiveMaxConcurrentRuns())
	}

	// Field present with explicit value → returned as-is.
	var cfgPresent CronJobConfig
	if err := json.Unmarshal([]byte(`{"id":"j2","name":"Job","enabled":true,"schedule":"* * * * *","task":{"prompt":"hi"},"maxConcurrentRuns":3}`), &cfgPresent); err != nil {
		t.Fatalf("unmarshal with maxConcurrentRuns: %v", err)
	}
	if cfgPresent.MaxConcurrentRuns != 3 {
		t.Errorf("expected MaxConcurrentRuns=3, got %d", cfgPresent.MaxConcurrentRuns)
	}
	jPresent := &cronJob{CronJobConfig: cfgPresent}
	if jPresent.effectiveMaxConcurrentRuns() != 3 {
		t.Errorf("expected effectiveMaxConcurrentRuns()=3, got %d", jPresent.effectiveMaxConcurrentRuns())
	}
}
