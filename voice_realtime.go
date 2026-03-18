package main

import (
	"context"
	"encoding/json"
	"net/http"

	"tetora/internal/voice"
)

// --- WebSocket opcode constants ---

const (
	wsText   = voice.WSText
	wsBinary = voice.WSBinary
	wsClose  = voice.WSClose
	wsPing   = voice.WSPing
	wsPong   = voice.WSPong
)

// VoiceRealtimeEngine manages wake word detection and OpenAI Realtime API relay.
type VoiceRealtimeEngine = voice.VoiceRealtimeEngine

// newVoiceRealtimeEngine initializes the voice realtime engine.
func newVoiceRealtimeEngine(cfg *Config, ve *VoiceEngine) *VoiceRealtimeEngine {
	vcfg := voice.VoiceConfig{
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
	}
	return voice.NewVoiceRealtimeEngine(vcfg, ve)
}

// toolRegistryAdapter bridges *ToolRegistry to voice.ToolRegistryIface.
type toolRegistryAdapter struct {
	cfg *Config
	reg *ToolRegistry
}

func (a *toolRegistryAdapter) GetTool(name string) *voice.ToolEntry {
	tool, ok := a.reg.Get(name)
	if !ok {
		return nil
	}
	cfg := a.cfg
	return &voice.ToolEntry{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
		Handler: func(ctx context.Context, argsJSON json.RawMessage) (string, error) {
			return tool.Handler(ctx, cfg, argsJSON)
		},
	}
}

func (a *toolRegistryAdapter) ListTools() []*voice.ToolEntry {
	defs := a.reg.List()
	entries := make([]*voice.ToolEntry, 0, len(defs))
	cfg := a.cfg
	for _, tool := range defs {
		t := tool
		entries = append(entries, &voice.ToolEntry{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Handler: func(ctx context.Context, argsJSON json.RawMessage) (string, error) {
				return t.Handler(ctx, cfg, argsJSON)
			},
		})
	}
	return entries
}

// wsUpgrade upgrades an HTTP connection to WebSocket (server-side).
func wsUpgrade(w http.ResponseWriter, r *http.Request) (voice.WSConn, error) {
	return voice.WSUpgrade(w, r)
}

// generateSessionID returns a random 32-character hex session ID.
func generateSessionID() string {
	return voice.GenerateSessionID()
}

// handleWakeWebSocket is the root-level handler for /ws/voice/wake.
// Called by http.go via s.voiceRealtimeEngine.handleWakeWebSocket.
// NOTE: the VoiceRealtimeEngine type alias exposes HandleWakeWebSocket (exported).
// http.go calls s.voiceRealtimeEngine.handleWakeWebSocket which is routed below.

// handleRealtimeWebSocket is the root-level handler for /ws/voice/realtime.
// http.go calls s.voiceRealtimeEngine.handleRealtimeWebSocket which is routed below.
