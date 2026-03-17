// Package dispatch provides shared types for task dispatch, routing, and SSE streaming.
// These types are used across multiple domain packages (discord, telegram, workflow, etc.)
// and must live in internal/ to break the root-only dependency.
package dispatch

// SSEEvent represents a Server-Sent Event for task/session streaming.
type SSEEvent struct {
	Type          string `json:"type"`                   // "started", "progress", "output_chunk", "completed", "error", "heartbeat"
	TaskID        string `json:"taskId,omitempty"`        // which task produced this event
	SessionID     string `json:"sessionId,omitempty"`     // which session this belongs to
	WorkflowRunID string `json:"workflowRunId,omitempty"` // workflow run ID for routing
	Data          any    `json:"data,omitempty"`          // event-specific payload
	Timestamp     string `json:"timestamp"`               // RFC3339
}

// SSE event type constants.
const (
	SSEStarted           = "started"
	SSEProgress          = "progress"
	SSEOutputChunk       = "output_chunk"
	SSECompleted         = "completed"
	SSEError             = "error"
	SSEHeartbeat         = "heartbeat"
	SSEQueued            = "task_queued"
	SSETaskReceived      = "task_received"
	SSETaskRouting       = "task_routing"
	SSEDiscordProcessing = "discord_processing"
	SSEDiscordReplying   = "discord_replying"
	SSEDashboardKey      = "__dashboard__"
	SSEToolCall          = "tool_call"
	SSEToolResult        = "tool_result"
	SSESessionMessage    = "session_message"
	SSEAgentState        = "agent_state"
	SSEHeartbeatAlert    = "heartbeat_alert"
	SSETaskStalled       = "task_stalled"
	SSETaskRecovered     = "task_recovered"
	SSEWorkerUpdate      = "worker_update"
	SSEHookEvent         = "hook_event"
	SSEPlanReview        = "plan_review"
)

// SSEBrokerPublisher is the interface for publishing SSE events.
// Root sseBroker satisfies this; domain packages use this interface.
type SSEBrokerPublisher interface {
	Publish(key string, event SSEEvent)
	PublishMulti(keys []string, event SSEEvent)
	HasSubscribers(key string) bool
}
