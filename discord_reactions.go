package main

// discord_reactions.go — thin wrapper around internal/discord.ReactionManager.
// The discordRequest method stays here since it's on *DiscordBot.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"tetora/internal/discord"
	"tetora/internal/log"
)

// Type aliases for backward compat.
type discordReactionManager = discord.ReactionManager

// Constant aliases.
const (
	reactionPhaseQueued   = discord.ReactionPhaseQueued
	reactionPhaseThinking = discord.ReactionPhaseThinking
	reactionPhaseTool     = discord.ReactionPhaseTool
	reactionPhaseDone     = discord.ReactionPhaseDone
	reactionPhaseError    = discord.ReactionPhaseError
)

var defaultReactionEmojis = discord.DefaultReactionEmojis
var validReactionPhases = discord.ValidReactionPhases

func newDiscordReactionManager(bot *DiscordBot, overrides map[string]string) *discordReactionManager {
	return discord.NewReactionManager(bot.api, overrides)
}

// --- Generic Discord API Request Helper ---

// discordRequest performs a generic HTTP request to the Discord API.
// Supports PUT, DELETE, PATCH, GET, POST methods.
func (db *DiscordBot) discordRequest(method, path string, payload any) (int, []byte) {
	if db == nil || db.api == nil {
		return 0, nil
	}
	var bodyStr string
	if payload != nil {
		body, _ := json.Marshal(payload)
		bodyStr = string(body)
	}

	reqBody := strings.NewReader(bodyStr)
	req, err := http.NewRequest(method, discordAPIBase+path, reqBody)
	if err != nil {
		log.Error("discord api request error", "method", method, "path", path, "error", err)
		return 0, nil
	}
	if bodyStr != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bot "+db.cfg.Discord.BotToken)

	resp, err := db.api.HTTPClient.Do(req)
	if err != nil {
		log.Error("discord api send failed", "method", method, "path", path, "error", err)
		return 0, nil
	}
	defer resp.Body.Close()

	var respBody []byte
	if resp.Body != nil {
		respBody, _ = io.ReadAll(io.LimitReader(resp.Body, 8192))
	}

	if resp.StatusCode >= 400 {
		log.Warn("discord api error", "method", method, "path", path,
			"status", resp.StatusCode, "body", string(respBody))
	}

	return resp.StatusCode, respBody
}
