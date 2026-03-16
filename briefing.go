package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tetora/internal/automation/briefing"
)

// --- P24.7: Morning Briefing & Evening Wrap ---

var globalBriefingService *BriefingService

// --- Tool Handlers ---

func toolBriefingMorning(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	app := appFromCtx(ctx)
	if app == nil || app.Briefing == nil {
		return "", fmt.Errorf("briefing service not initialized")
	}
	var args struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	date := time.Now()
	if args.Date != "" {
		parsed, err := time.Parse("2006-01-02", args.Date)
		if err != nil {
			return "", fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
		date = parsed
	}
	br, err := app.Briefing.GenerateMorning(date)
	if err != nil {
		return "", err
	}
	return briefing.FormatBriefing(br), nil
}

func toolBriefingEvening(ctx context.Context, cfg *Config, input json.RawMessage) (string, error) {
	app := appFromCtx(ctx)
	if app == nil || app.Briefing == nil {
		return "", fmt.Errorf("briefing service not initialized")
	}
	var args struct {
		Date string `json:"date"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	date := time.Now()
	if args.Date != "" {
		parsed, err := time.Parse("2006-01-02", args.Date)
		if err != nil {
			return "", fmt.Errorf("invalid date format (expected YYYY-MM-DD): %w", err)
		}
		date = parsed
	}
	br, err := app.Briefing.GenerateEvening(date)
	if err != nil {
		return "", err
	}
	return briefing.FormatBriefing(br), nil
}
