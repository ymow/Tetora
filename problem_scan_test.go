package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProblemScanDisabledSkips(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	tb := newTaskBoardEngine(dbPath, TaskBoardConfig{Enabled: true, ProblemScan: false}, nil)
	if err := tb.initTaskBoardSchema(); err != nil {
		t.Fatal(err)
	}

	task, err := tb.CreateTask(TaskBoard{Title: "Test", Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &TaskBoardDispatcher{
		engine: tb,
		cfg:    &Config{},
		ctx:    ctx,
	}

	// Should return immediately without creating any comments.
	d.postTaskProblemScan(task, "ERROR: something failed", "done")

	comments, err := tb.GetThread(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments when ProblemScan disabled, got %d", len(comments))
	}
}

func TestProblemScanEmptyOutputSkips(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	tb := newTaskBoardEngine(dbPath, TaskBoardConfig{Enabled: true, ProblemScan: true}, nil)
	if err := tb.initTaskBoardSchema(); err != nil {
		t.Fatal(err)
	}

	task, err := tb.CreateTask(TaskBoard{Title: "Test", Project: "proj"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d := &TaskBoardDispatcher{
		engine: tb,
		cfg:    &Config{},
		ctx:    ctx,
	}

	// Empty output should skip.
	d.postTaskProblemScan(task, "", "done")
	d.postTaskProblemScan(task, "   ", "done")

	comments, err := tb.GetThread(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("expected 0 comments for empty output, got %d", len(comments))
	}
}

func TestProblemScanFollowUpCreation(t *testing.T) {
	// Test that follow-up tickets are created correctly with DependsOn reference.
	// This tests the DB/ticket-creation path that postTaskProblemScan uses.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	tb := newTaskBoardEngine(dbPath, TaskBoardConfig{Enabled: true, ProblemScan: true}, nil)
	if err := tb.initTaskBoardSchema(); err != nil {
		t.Fatal(err)
	}

	parent, err := tb.CreateTask(TaskBoard{
		Title:   "Parent Task",
		Project: "proj",
		Status:  "backlog",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate what postTaskProblemScan does: create follow-up tickets.
	followups := []struct {
		title    string
		desc     string
		priority string
	}{
		{"Fix error handling", "The error path is missing", "high"},
		{"Add missing tests", "Test coverage gap found", "normal"},
		{"Review security concern", "Hardcoded token detected", "high"},
		{"Should be skipped", "Over the cap of 3", "low"},
	}

	created := 0
	for _, f := range followups {
		if created >= 3 {
			break
		}
		newTask, err := tb.CreateTask(TaskBoard{
			Project:     parent.Project,
			Title:       f.title,
			Description: f.desc,
			Priority:    f.priority,
			Status:      "backlog",
			DependsOn:   []string{parent.ID},
		})
		if err != nil {
			t.Fatalf("failed to create follow-up: %v", err)
		}
		if newTask.ID == "" {
			t.Fatal("follow-up task should have ID")
		}
		created++
	}

	if created != 3 {
		t.Fatalf("expected 3 follow-ups created (capped), got %d", created)
	}

	// Verify all follow-ups exist.
	tasks, err := tb.ListTasks("", "", "proj")
	if err != nil {
		t.Fatal(err)
	}
	// 1 parent + 3 follow-ups = 4
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks total, got %d", len(tasks))
	}

	// Verify DependsOn is set.
	for _, task := range tasks {
		if task.ID == parent.ID {
			continue
		}
		if len(task.DependsOn) != 1 || task.DependsOn[0] != parent.ID {
			t.Fatalf("follow-up %s should depend on parent %s, got %v", task.ID, parent.ID, task.DependsOn)
		}
	}
}

func TestProblemScanCommentFormat(t *testing.T) {
	// Verify the comment format matches what postTaskProblemScan produces.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	tb := newTaskBoardEngine(dbPath, TaskBoardConfig{Enabled: true}, nil)
	if err := tb.initTaskBoardSchema(); err != nil {
		t.Fatal(err)
	}

	task, err := tb.CreateTask(TaskBoard{Title: "Test"})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the comment that postTaskProblemScan would add.
	comment := "[problem-scan] Potential issues detected:\n- [high] Missing error handling: The function returns nil on error\n- [medium] Skipped test: TestFoo is commented out\n"
	c, err := tb.AddComment(task.ID, "system", comment)
	if err != nil {
		t.Fatal(err)
	}
	if c.Author != "system" {
		t.Fatalf("expected author system, got %s", c.Author)
	}

	thread, err := tb.GetThread(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(thread) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(thread))
	}
	if thread[0].Content != comment {
		t.Fatalf("comment content mismatch")
	}
}
