package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- Web Fetch Tests ---

func TestWebFetch_HTML(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><h1>Test Page</h1><p>This is a test.</p></body></html>"))
	}))
	defer mockServer.Close()

	cfg := &Config{}
	ctx := context.Background()
	input := json.RawMessage(`{"url": "` + mockServer.URL + `"}`)

	result, err := toolWebFetch(ctx, cfg, input)
	if err != nil {
		t.Fatalf("toolWebFetch failed: %v", err)
	}

	if !strings.Contains(result, "Test Page") {
		t.Errorf("expected 'Test Page' in result, got: %s", result)
	}
	if !strings.Contains(result, "This is a test") {
		t.Errorf("expected 'This is a test' in result, got: %s", result)
	}
	if strings.Contains(result, "<html>") || strings.Contains(result, "<body>") {
		t.Errorf("expected HTML tags to be stripped, got: %s", result)
	}
}

func TestWebFetch_PlainText(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Plain text content"))
	}))
	defer mockServer.Close()

	cfg := &Config{}
	ctx := context.Background()
	input := json.RawMessage(`{"url": "` + mockServer.URL + `"}`)

	result, err := toolWebFetch(ctx, cfg, input)
	if err != nil {
		t.Fatalf("toolWebFetch failed: %v", err)
	}

	if result != "Plain text content" {
		t.Errorf("expected 'Plain text content', got: %s", result)
	}
}

func TestWebFetch_MaxLength(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return a long HTML page.
		longContent := strings.Repeat("<p>Lorem ipsum dolor sit amet. </p>", 1000)
		w.Write([]byte("<html><body>" + longContent + "</body></html>"))
	}))
	defer mockServer.Close()

	cfg := &Config{}
	ctx := context.Background()
	input := json.RawMessage(`{"url": "` + mockServer.URL + `", "maxLength": 100}`)

	result, err := toolWebFetch(ctx, cfg, input)
	if err != nil {
		t.Fatalf("toolWebFetch failed: %v", err)
	}

	if len(result) > 100 {
		t.Errorf("expected result length <= 100, got %d", len(result))
	}
}

func TestWebFetch_Timeout(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than timeout.
		ctx := r.Context()
		select {
		case <-ctx.Done():
			return
		}
	}))
	defer mockServer.Close()

	cfg := &Config{}
	ctx := context.Background()
	input := json.RawMessage(`{"url": "` + mockServer.URL + `"}`)

	_, err := toolWebFetch(ctx, cfg, input)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestWebFetch_InvalidURL(t *testing.T) {
	cfg := &Config{}
	ctx := context.Background()
	input := json.RawMessage(`{"url": "not-a-valid-url"}`)

	_, err := toolWebFetch(ctx, cfg, input)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}
