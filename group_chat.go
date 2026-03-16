package main

import (
	"tetora/internal/messaging/groupchat"
)

// GroupChatEngine is an alias for the internal groupchat.Engine.
type GroupChatEngine = groupchat.Engine

// GroupMessage is an alias for the internal groupchat.Message.
type GroupMessage = groupchat.Message

// GroupChatStatus is an alias for the internal groupchat.Status.
type GroupChatStatus = groupchat.Status

// newGroupChatEngine creates a new GroupChatEngine from the root Config.
func newGroupChatEngine(cfg *Config) *GroupChatEngine {
	if cfg == nil {
		return nil
	}

	// Build agent names from cfg.Agents map.
	var agentNames []string
	for name := range cfg.Agents {
		agentNames = append(agentNames, name)
	}

	return groupchat.New(&groupchat.Config{
		Activation:    cfg.GroupChat.Activation,
		Keywords:      cfg.GroupChat.Keywords,
		ContextWindow: cfg.GroupChat.ContextWindow,
		RateLimit: groupchat.RateLimitConfig{
			MaxPerMin: cfg.GroupChat.RateLimit.MaxPerMin,
			PerGroup:  cfg.GroupChat.RateLimit.PerGroup,
		},
		AllowedGroups: cfg.GroupChat.AllowedGroups,
		ThreadReply:   cfg.GroupChat.ThreadReply,
		MentionNames:  cfg.GroupChat.MentionNames,
		AgentNames:    agentNames,
	})
}
