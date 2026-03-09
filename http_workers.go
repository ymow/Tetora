package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerWorkersRoutes(mux *http.ServeMux) {
	// GET /api/workers — list all active hook-based workers.
	mux.HandleFunc("/api/workers", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		type workerInfo struct {
			SessionId string `json:"sessionId"`
			Name      string `json:"name"`
			State     string `json:"state"`
			Workdir   string `json:"workdir"`
			Uptime    string `json:"uptime"`
			ToolCount int    `json:"toolCount"`
			LastTool  string `json:"lastTool,omitempty"`
			Source    string `json:"source"`
		}
		var out []workerInfo

		if s.hookReceiver != nil {
			hookWorkers := s.hookReceiver.ListHookWorkers()
			for _, hw := range hookWorkers {
				// Skip "done" workers older than 2 minutes.
				if hw.State == "done" && time.Since(hw.LastSeen) > 2*time.Minute {
					continue
				}
				sessionShort := hw.SessionID
				if len(sessionShort) > 12 {
					sessionShort = sessionShort[:12]
				}
				out = append(out, workerInfo{
					SessionId: sessionShort,
					Name:      "hook-" + sessionShort,
					State:     hw.State,
					Workdir:   hw.Cwd,
					Uptime:    time.Since(hw.FirstSeen).Round(time.Second).String(),
					ToolCount: hw.ToolCount,
					LastTool:  hw.LastTool,
					Source:    "hooks",
				})
			}
		}

		if out == nil {
			out = []workerInfo{}
		}
		json.NewEncoder(w).Encode(map[string]any{"workers": out, "count": len(out)})
	})

	// GET /api/workers/{id}/events — event log for a specific worker.
	mux.HandleFunc("/api/workers/events/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// Path: /api/workers/events/{sessionIdPrefix}
		idPrefix := strings.TrimPrefix(r.URL.Path, "/api/workers/events/")
		if idPrefix == "" {
			http.NotFound(w, r)
			return
		}

		if s.hookReceiver == nil {
			json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
			return
		}

		worker, events := s.hookReceiver.FindHookWorkerByPrefix(idPrefix)
		if worker == nil {
			json.NewEncoder(w).Encode(map[string]any{"events": []any{}})
			return
		}

		json.NewEncoder(w).Encode(map[string]any{
			"sessionId": idPrefix,
			"state":     worker.State,
			"workdir":   worker.Cwd,
			"toolCount": worker.ToolCount,
			"lastTool":  worker.LastTool,
			"uptime":    time.Since(worker.FirstSeen).Round(time.Second).String(),
			"events":    events,
		})
	})

	// GET /api/workers/agents — list agents with their provider info.
	mux.HandleFunc("/api/workers/agents", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		type agentInfo struct {
			Name     string `json:"name"`
			Provider string `json:"provider"`
			Model    string `json:"model"`
		}
		cfg := s.cfg
		agents := make([]agentInfo, 0, len(cfg.Agents))
		for name, rc := range cfg.Agents {
			p := rc.Provider
			if p == "" {
				p = cfg.DefaultProvider
			}
			if p == "" {
				p = "claude"
			}
			agents = append(agents, agentInfo{
				Name:     name,
				Provider: p,
				Model:    rc.Model,
			})
		}
		json.NewEncoder(w).Encode(map[string]any{"agents": agents})
	})

	// GET/PATCH /api/settings/discord — Discord display settings.
	mux.HandleFunc("/api/settings/discord", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			showProgress := s.cfg.Discord.ShowProgress == nil || *s.cfg.Discord.ShowProgress
			json.NewEncoder(w).Encode(map[string]any{
				"showProgress": showProgress,
			})

		case http.MethodPatch:
			var body struct {
				ShowProgress *bool `json:"showProgress"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "invalid body"})
				return
			}
			if body.ShowProgress != nil {
				s.cfg.Discord.ShowProgress = body.ShowProgress
				configPath := findConfigPath()
				if configPath != "" {
					updateConfigField(configPath, func(raw map[string]any) {
						disc, _ := raw["discord"].(map[string]any)
						if disc == nil {
							disc = map[string]any{}
							raw["discord"] = disc
						}
						disc["showProgress"] = *body.ShowProgress
					})
				}
			}
			showProgress := s.cfg.Discord.ShowProgress == nil || *s.cfg.Discord.ShowProgress
			json.NewEncoder(w).Encode(map[string]any{"showProgress": showProgress})

		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// POST /api/iterm2/keystroke — send a keystroke to iTerm2.
	mux.HandleFunc("/api/iterm2/keystroke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			Key string `json:"key"` // ArrowUp, ArrowDown, Return, Escape
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Key == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "key is required"})
			return
		}

		if err := sendITerm2Keystroke(body.Key); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ok": "true", "key": body.Key})
	})
}
