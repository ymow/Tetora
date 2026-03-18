package main

import (
	"context"
	"io"

	"tetora/internal/voice"
)

// --- Type aliases ---

type STTProvider = voice.STTProvider
type STTOptions = voice.STTOptions
type STTResult = voice.STTResult

type TTSProvider = voice.TTSProvider
type TTSOptions = voice.TTSOptions

type OpenAISTTProvider = voice.OpenAISTTProvider
type OpenAITTSProvider = voice.OpenAITTSProvider
type ElevenLabsTTSProvider = voice.ElevenLabsTTSProvider

// VoiceEngine coordinates STT and TTS providers.
type VoiceEngine = voice.VoiceEngine

// newVoiceEngine initializes the voice engine from config.
func newVoiceEngine(cfg *Config) *VoiceEngine {
	return voice.NewVoiceEngine(voice.VoiceConfig{
		STT: voice.STTConfig{
			Enabled:  cfg.Voice.STT.Enabled,
			Provider: cfg.Voice.STT.Provider,
			Model:    cfg.Voice.STT.Model,
			Endpoint: cfg.Voice.STT.Endpoint,
			APIKey:   cfg.Voice.STT.APIKey,
			Language: cfg.Voice.STT.Language,
		},
		TTS: voice.TTSConfig{
			Enabled:  cfg.Voice.TTS.Enabled,
			Provider: cfg.Voice.TTS.Provider,
			Model:    cfg.Voice.TTS.Model,
			Endpoint: cfg.Voice.TTS.Endpoint,
			APIKey:   cfg.Voice.TTS.APIKey,
			Voice:    cfg.Voice.TTS.Voice,
			Format:   cfg.Voice.TTS.Format,
		},
		Wake: voice.VoiceWakeConfig{
			Enabled:   cfg.Voice.Wake.Enabled,
			WakeWords: cfg.Voice.Wake.WakeWords,
			Threshold: cfg.Voice.Wake.Threshold,
		},
		Realtime: voice.VoiceRealtimeConfig{
			Enabled:  cfg.Voice.Realtime.Enabled,
			Provider: cfg.Voice.Realtime.Provider,
			Model:    cfg.Voice.Realtime.Model,
			APIKey:   cfg.Voice.Realtime.APIKey,
			Voice:    cfg.Voice.Realtime.Voice,
		},
	})
}

// Compile-time check: VoiceEngine satisfies the method set used by http.go.
var _ interface {
	Transcribe(ctx context.Context, audio io.Reader, opts STTOptions) (*STTResult, error)
	Synthesize(ctx context.Context, text string, opts TTSOptions) (io.ReadCloser, error)
} = (*VoiceEngine)(nil)
