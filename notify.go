package main

import (
	"time"

	"tetora/internal/notify"
)

// Type aliases and shims — root callers continue to compile unchanged.

type Notifier = notify.Notifier

type SlackNotifier = notify.SlackNotifier
type DiscordNotifier = notify.DiscordNotifier
type MultiNotifier = notify.MultiNotifier
type WhatsAppNotifier = notify.WhatsAppNotifier

// NotifyMessage is the root-level alias for notify.Message.
type NotifyMessage = notify.Message

// NotificationEngine is the root-level alias for notify.Engine.
type NotificationEngine = notify.Engine

const (
	PriorityCritical = notify.PriorityCritical
	PriorityHigh     = notify.PriorityHigh
	PriorityNormal   = notify.PriorityNormal
	PriorityLow      = notify.PriorityLow
)

func buildNotifiers(cfg *Config) []Notifier {
	return notify.BuildNotifiers(cfg)
}

func buildDiscordNotifierByName(cfg *Config, name string) *DiscordNotifier {
	return notify.BuildDiscordNotifierByName(cfg, name)
}

func NewNotificationEngine(cfg *Config, notifiers []Notifier, fallbackFn func(string)) *NotificationEngine {
	return notify.NewEngine(cfg, notifiers, fallbackFn)
}

func wrapNotifyFn(ne *NotificationEngine, defaultPriority string) func(string) {
	return notify.WrapNotifyFn(ne, defaultPriority)
}

func priorityRank(p string) int {
	return notify.PriorityRank(p)
}

func priorityFromRank(rank int) string {
	return notify.PriorityFromRank(rank)
}

func isValidPriority(p string) bool {
	return notify.IsValidPriority(p)
}

// newDiscordNotifier is a local helper used by tool_core.go for ad-hoc webhook sends.
func newDiscordNotifier(webhookURL string, timeout time.Duration) *DiscordNotifier {
	return notify.NewDiscordNotifier(webhookURL, timeout)
}
