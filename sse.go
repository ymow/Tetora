package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	dtypes "tetora/internal/dispatch"
)

// --- SSE Event Types (aliases to internal/dispatch) ---

type SSEEvent = dtypes.SSEEvent

const (
	SSEStarted           = dtypes.SSEStarted
	SSEProgress          = dtypes.SSEProgress
	SSEOutputChunk       = dtypes.SSEOutputChunk
	SSECompleted         = dtypes.SSECompleted
	SSEError             = dtypes.SSEError
	SSEHeartbeat         = dtypes.SSEHeartbeat
	SSEQueued            = dtypes.SSEQueued
	SSETaskReceived      = dtypes.SSETaskReceived
	SSETaskRouting       = dtypes.SSETaskRouting
	SSEDiscordProcessing = dtypes.SSEDiscordProcessing
	SSEDiscordReplying   = dtypes.SSEDiscordReplying
	SSEDashboardKey      = dtypes.SSEDashboardKey
	SSEToolCall          = dtypes.SSEToolCall
	SSEToolResult        = dtypes.SSEToolResult
	SSESessionMessage    = dtypes.SSESessionMessage
	SSEAgentState        = dtypes.SSEAgentState
	SSEHeartbeatAlert    = dtypes.SSEHeartbeatAlert
	SSETaskStalled       = dtypes.SSETaskStalled
	SSETaskRecovered     = dtypes.SSETaskRecovered
	SSEWorkerUpdate      = dtypes.SSEWorkerUpdate
	SSEHookEvent         = dtypes.SSEHookEvent
	SSEPlanReview        = dtypes.SSEPlanReview
)

// sseBroker is an alias for the canonical broker in internal/dispatch.
type sseBroker = dtypes.Broker

func newSSEBroker() *sseBroker {
	return dtypes.NewBroker()
}

// --- SSE HTTP Helpers ---

func serveSSE(w http.ResponseWriter, r *http.Request, broker *sseBroker, key string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := broker.Subscribe(key)
	defer unsub()

	var eventID atomic.Int64

	fmt.Fprintf(w, ": connected to %s\n\n", key)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-ch:
			if !ok {
				return
			}
			id := eventID.Add(1)
			writeSSEEvent(w, id, event)
			flusher.Flush()

			if event.Type == SSECompleted || event.Type == SSEError {
				return
			}

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

func serveDashboardSSE(w http.ResponseWriter, r *http.Request, broker *sseBroker) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := broker.Subscribe(SSEDashboardKey)
	defer unsub()

	var eventID atomic.Int64

	fmt.Fprintf(w, ": connected to dashboard\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-ch:
			if !ok {
				return
			}
			id := eventID.Add(1)
			writeSSEEvent(w, id, event)
			flusher.Flush()

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

// serveSSEPersistent handles a persistent SSE connection for a custom key.
// Unlike serveSSE, it does NOT close on completed/error events — it stays open
// across multiple task cycles (e.g. a Discord channel conversation).
func serveSSEPersistent(w http.ResponseWriter, r *http.Request, broker *sseBroker, key string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error":"streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, unsub := broker.Subscribe(key)
	defer unsub()

	var eventID atomic.Int64

	fmt.Fprintf(w, ": connected to %s\n\n", key)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			id := eventID.Add(1)
			writeSSEEvent(w, id, event)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat %s\n\n", time.Now().Format(time.RFC3339))
			flusher.Flush()
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, id int64, event SSEEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, event.Type, string(data))
}
