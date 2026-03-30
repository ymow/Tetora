package voice

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// stubTTSProvider is a configurable TTSProvider for testing.
type stubTTSProvider struct {
	name    string
	err     error
	calls   atomic.Int32
	body    string
}

func (s *stubTTSProvider) Name() string { return s.name }

func (s *stubTTSProvider) Synthesize(_ context.Context, _ string, _ TTSOptions) (io.ReadCloser, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	return io.NopCloser(strings.NewReader(s.body)), nil
}

func TestDisabledVoice_NoInit(t *testing.T) {
	ve := NewVoiceEngine(VoiceConfig{
		TTS: TTSConfig{Enabled: false},
	}, nil)

	if ve.TTS != nil {
		t.Fatal("expected TTS to be nil when disabled")
	}
	if ve.STT != nil {
		t.Fatal("expected STT to be nil when disabled")
	}
}

func TestHealthCheck_LocalFail_FallbackToNext(t *testing.T) {
	// Start a mock server that returns 503 on /health (simulating unhealthy).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("audio"))
	}))
	defer srv.Close()

	ve := NewVoiceEngine(VoiceConfig{
		TTS: TTSConfig{
			Enabled:   true,
			Providers: []string{"vibevoice-local", "openai"},
			APIKey:    "test-key",
			VibeVoice: VibeVoiceConfig{Endpoint: srv.URL},
		},
	}, nil)

	if ve.TTS == nil {
		t.Fatal("expected TTS to be initialized with fallback provider")
	}

	// vibevoice-local should have been removed (health check failed),
	// leaving only openai as a single provider (no FallbackTTSProvider wrapper).
	if _, ok := ve.TTS.(*OpenAITTSProvider); !ok {
		t.Fatalf("expected OpenAITTSProvider after health check failure, got %T", ve.TTS)
	}
}

func TestAllProvidersFail_VoiceNil(t *testing.T) {
	// vibevoice-local with unreachable endpoint, fal with no key.
	ve := NewVoiceEngine(VoiceConfig{
		TTS: TTSConfig{
			Enabled:   true,
			Providers: []string{"vibevoice-local", "fal"},
			VibeVoice: VibeVoiceConfig{Endpoint: "http://127.0.0.1:1"}, // unreachable
			// FalAPIKey intentionally empty → fal provider skipped
		},
	}, nil)

	if ve.TTS != nil {
		t.Fatalf("expected TTS to be nil when all providers fail init, got %T", ve.TTS)
	}
}

func TestFallbackChain_Order(t *testing.T) {
	p1 := &stubTTSProvider{name: "first", err: fmt.Errorf("fail1")}
	p2 := &stubTTSProvider{name: "second", err: fmt.Errorf("fail2")}
	p3 := &stubTTSProvider{name: "third", body: "ok"}

	fb := &FallbackTTSProvider{Providers: []TTSProvider{p1, p2, p3}}
	rc, err := fb.Synthesize(context.Background(), "hello", TTSOptions{})
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != "ok" {
		t.Fatalf("expected 'ok', got %q", string(data))
	}

	if p1.calls.Load() != 1 {
		t.Errorf("expected first provider called once, got %d", p1.calls.Load())
	}
	if p2.calls.Load() != 1 {
		t.Errorf("expected second provider called once, got %d", p2.calls.Load())
	}
	if p3.calls.Load() != 1 {
		t.Errorf("expected third provider called once, got %d", p3.calls.Load())
	}
}
