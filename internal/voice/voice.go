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
	"strings"
	"time"

	tlog "tetora/internal/log"
)

// Shared HTTP client for voice operations to reduce connection overhead.
var voiceClient = &http.Client{
	Timeout: 30 * time.Second,
}

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
	Provider string `json:"provider,omitempty"` // "openai", "groq"
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	APIKey   string `json:"apiKey,omitempty"` // supports $ENV_VAR
	Language string `json:"language,omitempty"`
}

// TTSConfig configures text-to-speech.
type TTSConfig struct {
	Enabled  bool   `json:"enabled,omitempty"`
	Provider string `json:"provider,omitempty"` // "openai", "elevenlabs"
	Model    string `json:"model,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
	APIKey   string `json:"apiKey,omitempty"` // supports $ENV_VAR
	Voice    string `json:"voice,omitempty"`
	Format   string `json:"format,omitempty"` // "mp3", "opus"
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
	Model    string // default: "whisper-1"
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
		model = "whisper-1"
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	format := opts.Format
	if format == "" {
		format = "mp3"
	}
	fw, err := mw.CreateFormFile("file", "audio."+format)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return nil, fmt.Errorf("copy audio: %w", err)
	}

	if err := mw.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}
	if opts.Language != "" {
		if err := mw.WriteField("language", opts.Language); err != nil {
			return nil, fmt.Errorf("write language field: %w", err)
		}
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create stt request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := voiceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai stt error: %d %s", resp.StatusCode, string(body))
	}

	var result STTResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode stt result: %w", err)
	}
	return &result, nil
}

// --- Groq STT Provider ---

// GroqSTTProvider implements STT using Groq Whisper API.
type GroqSTTProvider struct {
	Endpoint string // default: https://api.groq.com/openai/v1/audio/transcriptions
	APIKey   string
	Model    string // default: "whisper-large-v3-turbo"
}

func (p *GroqSTTProvider) Name() string {
	return "groq-stt"
}

func (p *GroqSTTProvider) Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*STTResult, error) {
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://api.groq.com/openai/v1/audio/transcriptions"
	}
	model := p.Model
	if model == "" {
		model = "whisper-large-v3-turbo"
	}

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	format := opts.Format
	if format == "" {
		format = "mp3"
	}
	fw, err := mw.CreateFormFile("file", "audio."+format)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return nil, fmt.Errorf("copy audio: %w", err)
	}

	if err := mw.WriteField("model", model); err != nil {
		return nil, fmt.Errorf("write model field: %w", err)
	}
	if opts.Language != "" {
		if err := mw.WriteField("language", opts.Language); err != nil {
			return nil, fmt.Errorf("write language field: %w", err)
		}
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return nil, fmt.Errorf("write response_format field: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("create stt request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := voiceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("groq stt request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("groq stt error: %d %s", resp.StatusCode, string(body))
	}

	var result STTResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode stt result: %w", err)
	}
	return &result, nil
}

// --- OpenAI TTS Provider ---

// OpenAITTSProvider implements TTS using OpenAI TTS API.
type OpenAITTSProvider struct {
	Endpoint string // default: https://api.openai.com/v1/audio/speech
	APIKey   string
	Model    string // default: "tts-1"
	Voice    string // "alloy", "echo", "fable", "onyx", "nova", "shimmer"
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
	voice := p.Voice
	if voice == "" {
		voice = "alloy"
	}

	payload, err := json.Marshal(map[string]any{
		"model": model,
		"input": text,
		"voice": voice,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tts payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create tts request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := voiceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai tts request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai tts error: %d %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// --- ElevenLabs TTS Provider ---

// ElevenLabsTTSProvider implements TTS using ElevenLabs API.
type ElevenLabsTTSProvider struct {
	APIKey  string
	VoiceID string // default: "pNInz6obpg8nEmeWscic" (Adam)
	Model   string // default: "eleven_monolingual_v1"
}

func (p *ElevenLabsTTSProvider) Name() string {
	return "elevenlabs-tts"
}

func (p *ElevenLabsTTSProvider) Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error) {
	voiceID := p.VoiceID
	if voiceID == "" {
		voiceID = "pNInz6obpg8nEmeWscic"
	}
	model := p.Model
	if model == "" {
		model = "eleven_monolingual_v1"
	}
	endpoint := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", voiceID)

	payload, err := json.Marshal(map[string]any{
		"text":     text,
		"model_id": model,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal elevenlabs tts payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create elevenlabs request: %w", err)
	}

	req.Header.Set("xi-api-key", p.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := voiceClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("elevenlabs tts request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("elevenlabs tts error: %d %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// --- Voice Engine (Coordinator) ---

// VoiceEngine coordinates STT and TTS providers.
type VoiceEngine struct {
	STT STTProvider
	TTS TTSProvider
	Cfg VoiceConfig
}

// NewVoiceEngine initializes the voice engine from VoiceConfig.
func NewVoiceEngine(cfg VoiceConfig) *VoiceEngine {
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
				tlog.Warn("voice stt enabled (openai) but no apiKey configured")
			}
			ve.STT = &OpenAISTTProvider{
				Endpoint: cfg.STT.Endpoint,
				APIKey:   apiKey,
				Model:    cfg.STT.Model,
			}
			tlog.Info("voice stt initialized", "provider", provider, "model", cfg.STT.Model)
		case "groq":
			apiKey := cfg.STT.APIKey
			if apiKey == "" {
				tlog.Warn("voice stt enabled (groq) but no apiKey configured")
			}
			ve.STT = &GroqSTTProvider{
				Endpoint: cfg.STT.Endpoint,
				APIKey:   apiKey,
				Model:    cfg.STT.Model,
			}
			tlog.Info("voice stt initialized", "provider", provider, "model", cfg.STT.Model)
		default:
			tlog.Warn("unknown stt provider", "provider", provider)
		}
	}

	// Initialize TTS provider.
	if cfg.TTS.Enabled {
		provider := cfg.TTS.Provider
		if provider == "" {
			provider = "openai"
		}
		switch provider {
		case "openai":
			apiKey := cfg.TTS.APIKey
			if apiKey == "" {
				tlog.Warn("voice tts enabled (openai) but no apiKey configured")
			}
			ve.TTS = &OpenAITTSProvider{
				Endpoint: cfg.TTS.Endpoint,
				APIKey:   apiKey,
				Model:    cfg.TTS.Model,
				Voice:    cfg.TTS.Voice,
			}
			tlog.Info("voice tts initialized", "provider", provider, "model", cfg.TTS.Model, "voice", cfg.TTS.Voice)
		case "elevenlabs":
			apiKey := cfg.TTS.APIKey
			if apiKey == "" {
				tlog.Warn("voice tts enabled (elevenlabs) but no apiKey configured")
			}
			ve.TTS = &ElevenLabsTTSProvider{
				APIKey:  apiKey,
				VoiceID: cfg.TTS.Voice,
				Model:   cfg.TTS.Model,
			}
			tlog.Info("voice tts initialized", "provider", provider, "model", cfg.TTS.Model, "voice", cfg.TTS.Voice)
		default:
			tlog.Warn("unknown tts provider", "provider", provider)
		}
	}

	return ve
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

// --- TTS Options ---

// TTSOptions configures synthesis behavior.
type TTSOptions struct {
	Voice  string // override default voice
	Model  string // override default model
	Format string // "mp3", "opus", etc.
}
