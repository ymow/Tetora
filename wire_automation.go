package main

// wire_automation.go wires the automation internal packages to the root package.

import (
	"time"

	"tetora/internal/automation/briefing"
	"tetora/internal/automation/insights"
)

// --- Insights type aliases ---

type InsightsEngine = insights.Engine
type LifeInsight = insights.LifeInsight
type LifeReport = insights.LifeReport
type SpendingReport = insights.SpendingReport
type TasksReport = insights.TasksReport
type MoodReport = insights.MoodReport
type SocialReport = insights.SocialReport
type HabitsReport = insights.HabitsReport

// --- Briefing type aliases ---

type BriefingService = briefing.Service
type BriefingSection = briefing.BriefingSection
type Briefing = briefing.Briefing

// --- Constructors ---

func newInsightsEngine(cfg *Config) *InsightsEngine {
	deps := insights.Deps{
		Query:   queryDB,
		Escape:  escapeSQLite,
		LogWarn: logWarn,
		UUID:    newUUID,
	}

	// Populate per-service DB paths based on available globals.
	if globalFinanceService != nil {
		deps.FinanceDBPath = globalFinanceService.DBPath()
	}
	if globalTaskManager != nil {
		deps.TasksDBPath = globalTaskManager.DBPath()
	}
	if globalUserProfileService != nil {
		deps.ProfileDBPath = globalUserProfileService.DBPath()
	}
	if globalContactsService != nil {
		deps.ContactsDBPath = globalContactsService.DBPath()
	}
	if globalHabitsService != nil {
		deps.HabitsDBPath = globalHabitsService.DBPath()
		deps.GetHabitStreak = globalHabitsService.GetStreak
	}

	return insights.New(cfg.HistoryDB, deps)
}

func initInsightsDB(dbPath string) error {
	return insights.InitDB(dbPath)
}

func newBriefingService(cfg *Config) *BriefingService {
	deps := briefing.Deps{
		Query:  queryDB,
		Escape: escapeSQLite,
	}

	// Inject optional service methods.
	if globalSchedulingService != nil {
		svc := globalSchedulingService
		deps.ViewSchedule = func(dateStr string, days int) ([]briefing.ScheduleDay, error) {
			schedules, err := svc.ViewSchedule(dateStr, days)
			if err != nil {
				return nil, err
			}
			result := make([]briefing.ScheduleDay, len(schedules))
			for i, s := range schedules {
				events := make([]briefing.ScheduleEvent, len(s.Events))
				for j, ev := range s.Events {
					events[j] = briefing.ScheduleEvent{
						Start: ev.Start,
						Title: ev.Title,
					}
				}
				result[i] = briefing.ScheduleDay{Events: events}
			}
			return result, nil
		}
	}
	if globalContactsService != nil {
		svc := globalContactsService
		deps.GetUpcomingEvents = func(days int) ([]map[string]any, error) {
			return svc.GetUpcomingEvents(days)
		}
	}

	// Service availability flags.
	deps.TasksAvailable = globalTaskManager != nil
	deps.HabitsAvailable = globalHabitsService != nil
	deps.GoalsAvailable = globalGoalsService != nil
	deps.FinanceAvailable = globalFinanceService != nil

	return briefing.New(cfg.HistoryDB, deps)
}

// --- Forwarding helpers ---

func periodDateRange(period string, anchor time.Time) (time.Time, time.Time) {
	return insights.PeriodDateRange(period, anchor)
}

func prevPeriodRange(period string, currentStart time.Time) (time.Time, time.Time) {
	return insights.PrevPeriodRange(period, currentStart)
}

func insightFromRow(row map[string]any) LifeInsight {
	return insights.InsightFromRow(row)
}

func capitalizeFirst(s string) string {
	return briefing.CapitalizeFirst(s)
}

// FormatBriefing formats a Briefing into a readable text string.
var FormatBriefing = briefing.FormatBriefing
