// Package cron provides a 5-field cron expression parser and scheduler.
package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Expr represents a parsed 5-field cron expression.
// Fields: minute(0-59) hour(0-23) dom(1-31) month(1-12) dow(0-6, 0=Sunday)
type Expr struct {
	Minutes []bool // [60]
	Hours   []bool // [24]
	DOMs    []bool // [32] (index 0 unused)
	Months  []bool // [13] (index 0 unused)
	DOWs    []bool // [7]
}

// Matches returns true if the given time matches this cron expression.
func (e Expr) Matches(t time.Time) bool {
	return e.Minutes[t.Minute()] &&
		e.Hours[t.Hour()] &&
		e.DOMs[t.Day()] &&
		e.Months[int(t.Month())] &&
		e.DOWs[int(t.Weekday())]
}

// Parse parses a 5-field cron expression string.
// Supports: *, N, N-M, */N, N-M/S, N,M,O
func Parse(s string) (Expr, error) {
	fields := strings.Fields(s)
	if len(fields) != 5 {
		return Expr{}, fmt.Errorf("expected 5 fields, got %d", len(fields))
	}

	minutes, err := parseField(fields[0], 0, 59)
	if err != nil {
		return Expr{}, fmt.Errorf("minute: %w", err)
	}
	hours, err := parseField(fields[1], 0, 23)
	if err != nil {
		return Expr{}, fmt.Errorf("hour: %w", err)
	}
	doms, err := parseField(fields[2], 1, 31)
	if err != nil {
		return Expr{}, fmt.Errorf("dom: %w", err)
	}
	months, err := parseField(fields[3], 1, 12)
	if err != nil {
		return Expr{}, fmt.Errorf("month: %w", err)
	}
	dows, err := parseField(fields[4], 0, 6)
	if err != nil {
		return Expr{}, fmt.Errorf("dow: %w", err)
	}

	e := Expr{
		Minutes: make([]bool, 60),
		Hours:   make([]bool, 24),
		DOMs:    make([]bool, 32),
		Months:  make([]bool, 13),
		DOWs:    make([]bool, 7),
	}
	for _, v := range minutes {
		e.Minutes[v] = true
	}
	for _, v := range hours {
		e.Hours[v] = true
	}
	for _, v := range doms {
		e.DOMs[v] = true
	}
	for _, v := range months {
		e.Months[v] = true
	}
	for _, v := range dows {
		e.DOWs[v] = true
	}
	return e, nil
}

// parseField parses a single cron field. Supports: *, N, N-M, */N, N-M/S, N,M,O
func parseField(field string, min, max int) ([]int, error) {
	var result []int
	for _, part := range strings.Split(field, ",") {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		result = append(result, vals...)
	}
	return result, nil
}

func parsePart(part string, min, max int) ([]int, error) {
	// Handle step: */N or N-M/S
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s <= 0 {
			return nil, fmt.Errorf("bad step in %q", part)
		}
		step = s
		part = part[:idx]
	}

	var lo, hi int

	switch {
	case part == "*":
		lo, hi = min, max

	case strings.Contains(part, "-"):
		parts := strings.SplitN(part, "-", 2)
		var err error
		lo, err = strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("bad range start in %q", part)
		}
		hi, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("bad range end in %q", part)
		}

	default:
		v, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("bad value %q", part)
		}
		if v < min || v > max {
			return nil, fmt.Errorf("value %d out of bounds [%d,%d]", v, min, max)
		}
		if step == 1 {
			return []int{v}, nil
		}
		lo, hi = v, max
	}

	if lo < min || hi > max || lo > hi {
		return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", lo, hi, min, max)
	}

	var vals []int
	for v := lo; v <= hi; v += step {
		vals = append(vals, v)
	}
	return vals, nil
}

// NextRunAfter finds the next time after `after` that matches the cron expression.
// Returns zero time if no match is found within 366 days.
func NextRunAfter(expr Expr, loc *time.Location, after time.Time) time.Time {
	// Start from the next minute.
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Search up to 366 days ahead.
	limit := t.Add(366 * 24 * time.Hour)
	for t.Before(limit) {
		if expr.Matches(t) {
			return t
		}
		// Skip ahead intelligently.
		if !expr.Months[int(t.Month())] {
			// Skip to next month.
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !expr.DOMs[t.Day()] || !expr.DOWs[int(t.Weekday())] {
			// Skip to next day.
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !expr.Hours[t.Hour()] {
			// Skip to next hour.
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}
		// Skip to next minute.
		t = t.Add(time.Minute)
	}
	return time.Time{} // no match found
}
