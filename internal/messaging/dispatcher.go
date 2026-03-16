// Package messaging defines shared interfaces for messaging platform integrations.
package messaging

import "context"

// TaskRequest represents a dispatch request from a messaging platform.
type TaskRequest struct {
	AgentRole string
	Content   string
	Meta      map[string]string
}

// TaskResult represents the result of a dispatched task.
type TaskResult struct {
	Output string
	Error  string
}

// Dispatcher abstracts the task dispatch mechanism from messaging integrations.
type Dispatcher interface {
	Submit(ctx context.Context, req TaskRequest) (TaskResult, error)
}
