// Package voice implements STT/TTS provider types and the VoiceEngine coordinator.
package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	tlog "tetora/internal/log"
)

// --- Config Types ---

// VoiceConfig aggregates all voice-related configuration.
type VoiceConfig struct {
	STT      STTConfig           `json:"stt,omitempty"`
	TTS      TTSConfig           `json:"tts,omitempty"`
	Wake     VoiceWakeConfig     `json:"wake,omitempty"`
	Realtime VoiceRealtimeConfig `json:"realtime,omitempty"`
}

// STTConfig configures speech-to-text.
type STTConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"` // "openai"
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	APIKey   string `json:"apiKey,omitempty"` // supports $ENV_VAR
	Language string `json:"language,omitempty"`
}

// TTSConfig configures text-to-speech.
type TTSConfig struct {
	Enabled   bool     `json:"enabled,omitempty"`
	Provider  string   `json:"provider,omitempty"`  // legacy single provider: "openai", "elevenlabs"
	Providers []string `json:"providers,omitempty"` // fallback chain: ["vibevoice-local", "fal", "openai"]
	Model     string   `json:"model,omitempty"`
	Endpoint  string   `json:"endpoint,omitempty"`
	APIKey    string   `json:"apiKey,omitempty"`    // supports $ENV_VAR
	FalAPIKey string   `json:"falApiKey,omitempty"` // $FAL_KEY
	Voice     string   `json:"voice,omitempty"`
	Format    string   `json:"format,omitempty"` // "mp3", "opus"
	VibeVoice VibeVoiceConfig `json:"vibevoice,omitempty"`
}

// VibeVoiceConfig configures the local VibeVoice TTS endpoint.
type VibeVoiceConfig struct {
	Endpoint string `json:"endpoint,omitempty"` // default: http://localhost:8880
}

// VoiceWakeConfig configures wake word detection.
type VoiceWakeConfig struct {
	Enabled   bool     `json:"enabled,omitempty"`
	WakeWords []string `json:"wakeWords,omitempty"` // ["テトラ", "tetora", "hey tetora"]
	Threshold float64  `json:"threshold,omitempty"` // VAD sensitivity (0.0-1.0), default 0.6
}

// VoiceRealtimeConfig configures the OpenAI Realtime API relay.
type VoiceRealtimeConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"` // "openai"
	Model    string `json:"model,omitempty"`    // "gpt-4o-realtime-preview"
	APIKey   string `json:"apiKey,omitempty"`   // $ENV_VAR supported
	Voice    string `json:"voice,omitempty"`    // "alloy", "shimmer", etc.
}

// --- STT (Speech-to-Text) Types ---

// STTProvider defines the interface for speech-to-text providers.
type STTProvider interface {
	Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*STTResult, error)
	Name() string
}

// STTOptions configures transcription behavior.
type STTOptions struct {
	Language string // ISO 639-1 code, "" = auto-detect
	Format   string // "ogg", "wav", "mp3", "webm", etc.
}

// STTResult holds transcription output.
type STTResult struct {
	Text       string  `json:"text"`
	Language   string  `json:"language"`
	Duration   float64 `json:"durationSec"`
	Confidence float64 `json:"confidence,omitempty"`
}

// --- OpenAI STT Provider ---

// OpenAISTTProvider implements STT using OpenAI Whisper API.
type OpenAISTTProvider struct {
	Endpoint string // default: https://api.openai.com/v1/audio/transcriptions
	APIKey   string
	Model    string // default: "gpt-4o-mini-transcribe"
}

func (p *OpenAISTTProvider) Name() string {
	return "openai-stt"
}

func (p *OpenAISTTProvider) Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*STTResult, error) {
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/audio/transcriptions"
	}
	model := p.Model
	if model == "" {
		model = "gpt-4o-mini-transcribe"
	}

	// Build multipart form data.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	// Add file field.
	format := opts.Format
	if format == "" {
		format = "mp3"
	}
	filename := "audio." + format
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return nil, fmt.Errorf("copy audio: %w", err)
	}

	// Add model field.
	if err := mw.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}

	// Add language field if specified.
	if opts.Language != "" {
		if err := mw.WriteField("language", opts.Language); err != nil {
			return nil, fmt.Errorf("write language field: %w", err)
		}
	}

	// Add response_format field (default: json).
	if err := mw.WriteField("response_format", "json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	// Create request.
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	// Execute request.
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai stt api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Parse response: {"text": "transcribed text"}
	var result struct {
		Text     string  `json:"text"`
		Language string  `json:"language,omitempty"`
		Duration float64 `json:"duration,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &STTResult{
		Text:     result.Text,
		Language: result.Language,
		Duration: result.Duration,
	}, nil
}

// --- TTS (Text-to-Speech) Types ---

// TTSProvider defines the interface for text-to-speech providers.
type TTSProvider interface {
	Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error)
	Name() string
}

// TTSOptions configures synthesis behavior.
type TTSOptions struct {
	Voice  string  // provider-specific voice ID
	Speed  float64 // default 1.0
	Format string  // "mp3", "opus", "wav"
}

// --- OpenAI TTS Provider ---

// OpenAITTSProvider implements TTS using OpenAI TTS API.
type OpenAITTSProvider struct {
	Endpoint string // default: https://api.openai.com/v1/audio/speech
	APIKey   string
	Model    string // default: "tts-1"
	Voice    string // default: "alloy"
}

func (p *OpenAITTSProvider) Name() string {
	return "openai-tts"
}

func (p *OpenAITTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/audio/speech"
	}
	model := p.Model
	if model == "" {
		model = "tts-1"
	}
	voice := opts.Voice
	if voice == "" {
		voice = p.Voice
	}
	if voice == "" {
		voice = "alloy"
	}
	format := opts.Format
	if format == "" {
		format = "mp3"
	}
	speed := opts.Speed
	if speed <= 0 {
		speed = 1.0
	}

	// Build request body.
	reqBody := map[string]any{
		"model":           model,
		"input":           text,
		"voice":           voice,
		"response_format": format,
		"speed":           speed,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create request.
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request.
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai tts api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Return audio stream (caller must close).
	return resp.Body, nil
}

// --- ElevenLabs TTS Provider ---

// ElevenLabsTTSProvider implements TTS using ElevenLabs API.
type ElevenLabsTTSProvider struct {
	APIKey  string
	VoiceID string // default: "Rachel"
	Model   string // default: "eleven_flash_v2_5"
}

func (p *ElevenLabsTTSProvider) Name() string {
	return "elevenlabs-tts"
}

func (p *ElevenLabsTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	voiceID := opts.Voice
	if voiceID == "" {
		voiceID = p.VoiceID
	}
	if voiceID == "" {
		voiceID = "Rachel"
	}
	model := p.Model
	if model == "" {
		model = "eleven_flash_v2_5"
	}

	endpoint := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	// Build request body.
	reqBody := map[string]any{
		"text":     text,
		"model_id": model,
	}
	// Add voice settings if speed is specified.
	if opts.Speed > 0 && opts.Speed != 1.0 {
		reqBody["voice_settings"] = map[string]any{
			"stability":        0.5,
			"similarity_boost": 0.75,
			"speed":            opts.Speed,
		}
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Create request.
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("xi-api-key", p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")

	// Execute request.
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Return audio stream (caller must close).
	return resp.Body, nil
}

// --- VibeVoice Local TTS Provider ---

// VibeVoiceLocalTTSProvider implements TTS via a local VibeVoice server
// exposing an OpenAI-compatible /v1/audio/speech endpoint.
type VibeVoiceLocalTTSProvider struct {
	Endpoint string // default: http://localhost:8880
}

func (p *VibeVoiceLocalTTSProvider) Name() string {
	return "vibevoice-local"
}

func (p *VibeVoiceLocalTTSProvider) endpoint() string {
	if p.Endpoint != "" {
		return p.Endpoint
	}
	return "http://localhost:8880"
}

func (p *VibeVoiceLocalTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	voice := opts.Voice
	if voice == "" {
		voice = "alloy"
	}
	format := opts.Format
	if format == "" {
		format = "mp3"
	}
	speed := opts.Speed
	if speed <= 0 {
		speed = 1.0
	}

	reqBody := map[string]any{
		"model":           "vibevoice",
		"input":           text,
		"voice":           voice,
		"response_format": format,
		"speed":           speed,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := p.endpoint() + "/v1/audio/speech"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vibevoice-local http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("vibevoice-local api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// Healthy checks if the local VibeVoice server is reachable.
func (p *VibeVoiceLocalTTSProvider) Healthy(ctx context.Context) bool {
	ctx2, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx2, "GET", p.endpoint()+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// --- Fal TTS Provider ---

// FalTTSProvider implements TTS using fal.ai's vibevoice model.
type FalTTSProvider struct {
	APIKey  string
	AuditFn func(action, source, detail string) // injected audit callback (avoids import cycle)
}

func (p *FalTTSProvider) Name() string {
	return "fal-tts"
}

func (p *FalTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("fal tts: apiKey not configured")
	}

	voice := opts.Voice
	if voice == "" {
		voice = "alloy"
	}

	reqBody := map[string]any{
		"text":  text,
		"voice": voice,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	endpoint := "https://fal.run/fal-ai/vibevoice"
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Key "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fal tts http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("fal tts api error: status=%d body=%s", resp.StatusCode, string(body))
	}

	// Log estimated cost: ~$0.04/min, estimate 150 chars/sec speech rate.
	if p.AuditFn != nil {
		charCount := len(text)
		estDurationSec := float64(charCount) / 15.0 // ~15 chars per second of speech
		costUSD := (estDurationSec / 60.0) * 0.04
		detail := fmt.Sprintf(`{"provider":"fal","text_chars":%d,"est_duration_sec":%.1f,"cost_usd":%.4f}`, charCount, estDurationSec, costUSD)
		p.AuditFn("voice.tts.cost", "fal-tts", detail)
	}

	return resp.Body, nil
}

// --- Fallback TTS Provider ---

// FallbackTTSProvider tries providers in order, returning the first success.
type FallbackTTSProvider struct {
	Providers []TTSProvider
}

func (f *FallbackTTSProvider) Name() string {
	if len(f.Providers) > 0 {
		return "fallback(" + f.Providers[0].Name() + "+...)"
	}
	return "fallback(empty)"
}

func (f *FallbackTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	var lastErr error
	for _, p := range f.Providers {
		rc, err := p.Synthesize(ctx, text, opts)
		if err == nil {
			return rc, nil
		}
		tlog.Warn("tts provider failed, trying next", "provider", p.Name(), "error", err)
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("all tts providers failed, last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no tts providers configured")
}

// --- Voice Engine (Coordinator) ---

// VoiceEngine coordinates STT and TTS providers.
type VoiceEngine struct {
	STT STTProvider
	TTS TTSProvider
	Cfg VoiceConfig
}

// NewVoiceEngine initializes the voice engine from VoiceConfig.
func NewVoiceEngine(cfg VoiceConfig, auditFn func(action, source, detail string)) *VoiceEngine {
	ve := &VoiceEngine{Cfg: cfg}

	// Initialize STT provider.
	if cfg.STT.Enabled {
		provider := cfg.STT.Provider
		if provider == "" {
			provider = "openai"
		}
		switch provider {
		case "openai":
			apiKey := cfg.STT.APIKey
			if apiKey == "" {
				tlog.Warn("voice stt enabled but no apiKey configured")
			}
			ve.STT = &OpenAISTTProvider{
				Endpoint: cfg.STT.Endpoint,
				APIKey:   apiKey,
				Model:    cfg.STT.Model,
			}
			tlog.Info("voice stt initialized", "provider", provider, "model", cfg.STT.Model)
		default:
			tlog.Warn("unknown stt provider", "provider", provider)
		}
	}

	// Initialize TTS provider(s).
	if cfg.TTS.Enabled {
		providers := cfg.TTS.Providers
		// Backward compat: if Providers is empty, use legacy single Provider field.
		if len(providers) == 0 && cfg.TTS.Provider != "" {
			providers = []string{cfg.TTS.Provider}
		}
		if len(providers) == 0 {
			providers = []string{"openai"}
		}

		var chain []TTSProvider
		for _, name := range providers {
			p := buildTTSProvider(name, cfg, auditFn)
			if p == nil {
				tlog.Warn("unknown tts provider, skipping", "provider", name)
				continue
			}
			// Health-check local providers asynchronously.
			if lp, ok := p.(*VibeVoiceLocalTTSProvider); ok {
				if !lp.Healthy(context.Background()) {
					tlog.Warn("vibevoice-local not reachable, removed from chain", "endpoint", lp.endpoint())
					continue
				}
			}
			chain = append(chain, p)
			tlog.Info("voice tts provider added", "provider", name)
		}

		if len(chain) == 1 {
			ve.TTS = chain[0]
		} else if len(chain) > 1 {
			ve.TTS = &FallbackTTSProvider{Providers: chain}
		} else {
			tlog.Warn("voice tts enabled but no providers available")
		}
	}

	return ve
}

// buildTTSProvider creates a single TTS provider by name.
func buildTTSProvider(name string, cfg VoiceConfig, auditFn func(action, source, detail string)) TTSProvider {
	switch name {
	case "vibevoice-local":
		return &VibeVoiceLocalTTSProvider{
			Endpoint: cfg.TTS.VibeVoice.Endpoint,
		}
	case "fal":
		if cfg.TTS.FalAPIKey == "" {
			tlog.Warn("fal tts provider requested but falApiKey not configured")
			return nil
		}
		return &FalTTSProvider{
			APIKey:  cfg.TTS.FalAPIKey,
			AuditFn: auditFn,
		}
	case "openai":
		apiKey := cfg.TTS.APIKey
		if apiKey == "" {
			tlog.Warn("voice tts enabled but no apiKey configured")
		}
		return &OpenAITTSProvider{
			Endpoint: cfg.TTS.Endpoint,
			APIKey:   apiKey,
			Model:    cfg.TTS.Model,
			Voice:    cfg.TTS.Voice,
		}
	case "elevenlabs":
		apiKey := cfg.TTS.APIKey
		if apiKey == "" {
			tlog.Warn("voice tts enabled but no apiKey configured")
		}
		return &ElevenLabsTTSProvider{
			APIKey:  apiKey,
			VoiceID: cfg.TTS.Voice,
			Model:   cfg.TTS.Model,
		}
	default:
		return nil
	}
}

// Transcribe delegates to the configured STT provider.
func (v *VoiceEngine) Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*STTResult, error) {
	if v.STT == nil {
		return nil, fmt.Errorf("stt not enabled")
	}
	return v.STT.Transcribe(ctx, audio, opts)
}

// Synthesize delegates to the configured TTS provider.
func (v *VoiceEngine) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	if v.TTS == nil {
		return nil, fmt.Errorf("tts not enabled")
	}
	return v.TTS.Synthesize(ctx, text, opts)
}
