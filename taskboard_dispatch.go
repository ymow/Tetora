package main

// taskboard_dtypes.go — thin shim: wires root-package dependencies into internal/taskboard.Dispatcher.
//
// TaskBoardDispatcher is a type alias for taskboard.Dispatcher (defined in taskboard.go).
// newTaskBoardDispatcher builds DispatcherDeps from root-package functions and calls
// taskboard.NewDispatcher.

import (
	"context"
	"path/filepath"

	"tetora/internal/config"
	"tetora/internal/discord"
	dtypes "tetora/internal/dispatch"
	"tetora/internal/taskboard"
)

func newTaskBoardDispatcher(engine *TaskBoardEngine, cfg *Config, sem, childSem chan struct{}, state *dispatchState) *TaskBoardDispatcher {
	wtBaseDir := filepath.Join(cfg.RuntimeDir, "worktrees")
	wtMgr := NewWorktreeManager(wtBaseDir)

	deps := taskboard.DispatcherDeps{
		Executor: dtypes.TaskExecutorFunc(func(ctx context.Context, task dtypes.Task, agentName string) dtypes.TaskResult {
			// Acquire semaphore slot for this task.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return dtypes.TaskResult{Status: "cancelled", Error: ctx.Err().Error()}
			}
			// Convert internal dtypes.Task to root Task and run.
			rootTask := Task(task)
			result := runSingleTask(ctx, cfg, rootTask, sem, childSem, agentName)
			return dtypes.TaskResult(result)
		}),
		ChildExecutor: dtypes.TaskExecutorFunc(func(ctx context.Context, task dtypes.Task, agentName string) dtypes.TaskResult {
			select {
			case childSem <- struct{}{}:
				defer func() { <-childSem }()
			case <-ctx.Done():
				return dtypes.TaskResult{Status: "cancelled", Error: ctx.Err().Error()}
			}
			rootTask := Task(task)
			result := runSingleTask(ctx, cfg, rootTask, childSem, nil, agentName)
			return dtypes.TaskResult(result)
		}),

		FillDefaults: func(c *config.Config, t *dtypes.Task) {
			rootTask := Task(*t)
			fillDefaults(c, &rootTask)
			*t = dtypes.Task(rootTask)
		},

		RecordHistory: func(dbPath, jobID, name, source, role string, task dtypes.Task, result dtypes.TaskResult, startedAt, finishedAt, outputFile string) {
			recordHistory(dbPath, jobID, name, source, role, Task(task), TaskResult(result), startedAt, finishedAt, outputFile)
		},

		LoadAgentPrompt: func(c *config.Config, agentName string) (string, error) {
			return loadAgentPrompt(c, agentName)
		},

		Workflows: &rootWorkflowRunner{cfg: cfg, state: state, sem: sem, childSem: childSem},

		GetProject: func(historyDB, id string) *taskboard.ProjectInfo {
			p, err := getProject(historyDB, id)
			if err != nil || p == nil {
				return nil
			}
			return &taskboard.ProjectInfo{Name: p.Name, Workdir: p.Workdir}
		},

		Skills: &rootSkillsProvider{cfg: cfg},

		Worktrees: wtMgr,

		Delegations: &rootDelegationProcessor{cfg: cfg, state: state, sem: sem, childSem: childSem},

		BuildBranch: func(gitCfg config.GitWorkflowConfig, t taskboard.TaskBoard) string {
			return buildBranchName(gitCfg, t)
		},

		NewID: newUUID,

		Truncate: func(s string, maxLen int) string {
			return truncate(s, maxLen)
		},

		TruncateToChars: func(s string, maxChars int) string {
			return truncateToChars(s, maxChars)
		},

		ExtractJSON: func(s string) string {
			return extractJSON(s)
		},

		Discord:                discordSender(state),
		DiscordNotifyChannelID: cfg.Discord.NotifyChannelID,
	}

	return taskboard.NewDispatcher(engine, cfg, deps)
}

// --- rootWorkflowRunner implements taskboard.WorkflowRunner ---

type rootWorkflowRunner struct {
	cfg      *Config
	state    *dispatchState
	sem      chan struct{}
	childSem chan struct{}
}

func (r *rootWorkflowRunner) Execute(ctx context.Context, workflowName string, vars map[string]string) (taskboard.WorkflowRunResult, error) {
	w, err := loadWorkflowByName(r.cfg, workflowName)
	if err != nil {
		return taskboard.WorkflowRunResult{}, err
	}
	run := executeWorkflow(ctx, r.cfg, w, vars, r.state, r.sem, r.childSem)
	return convertWorkflowRun(run, w), nil
}

func (r *rootWorkflowRunner) Resume(ctx context.Context, runID string) (taskboard.WorkflowRunResult, error) {
	run, err := resumeWorkflow(ctx, r.cfg, runID, r.state, r.sem, r.childSem)
	if err != nil {
		return taskboard.WorkflowRunResult{}, err
	}
	// Load the workflow definition to get step order.
	prevRun, qErr := queryWorkflowRunByID(r.cfg.HistoryDB, runID)
	if qErr != nil || prevRun == nil {
		return convertWorkflowRunNoSteps(run), nil
	}
	w, wErr := loadWorkflowByName(r.cfg, prevRun.WorkflowName)
	if wErr != nil {
		return convertWorkflowRunNoSteps(run), nil
	}
	return convertWorkflowRun(run, w), nil
}

func (r *rootWorkflowRunner) QueryRun(dbPath, id string) (taskboard.WorkflowRunInfo, error) {
	run, err := queryWorkflowRunByID(dbPath, id)
	if err != nil || run == nil {
		return taskboard.WorkflowRunInfo{}, err
	}
	return taskboard.WorkflowRunInfo{
		ID:           run.ID,
		WorkflowName: run.WorkflowName,
		Status:       run.Status,
	}, nil
}

// convertWorkflowRun maps a root WorkflowRun to the internal WorkflowRunResult.
func convertWorkflowRun(run *WorkflowRun, w *Workflow) taskboard.WorkflowRunResult {
	r := taskboard.WorkflowRunResult{
		ID:          run.ID,
		Status:      run.Status,
		TotalCost:   run.TotalCost,
		DurationMs:  run.DurationMs,
		Error:       run.Error,
		StepOutputs: make(map[string]string),
		StepErrors:  make(map[string]string),
		StepSessions: make(map[string]string),
	}
	// Build ordered step list from Workflow definition.
	for _, step := range w.Steps {
		r.StepOrder = append(r.StepOrder, step.ID)
		if sr, ok := run.StepResults[step.ID]; ok {
			if sr.Output != "" {
				r.StepOutputs[step.ID] = sr.Output
			}
			if sr.Error != "" {
				r.StepErrors[step.ID] = sr.Error
			}
			if sr.SessionID != "" {
				r.StepSessions[step.ID] = sr.SessionID
			}
		}
	}
	return r
}

// convertWorkflowRunNoSteps maps a WorkflowRun without workflow step info.
func convertWorkflowRunNoSteps(run *WorkflowRun) taskboard.WorkflowRunResult {
	r := taskboard.WorkflowRunResult{
		ID:           run.ID,
		Status:       run.Status,
		TotalCost:    run.TotalCost,
		DurationMs:   run.DurationMs,
		Error:        run.Error,
		StepOutputs:  make(map[string]string),
		StepErrors:   make(map[string]string),
		StepSessions: make(map[string]string),
	}
	for id, sr := range run.StepResults {
		r.StepOrder = append(r.StepOrder, id)
		if sr.Output != "" {
			r.StepOutputs[id] = sr.Output
		}
		if sr.Error != "" {
			r.StepErrors[id] = sr.Error
		}
		if sr.SessionID != "" {
			r.StepSessions[id] = sr.SessionID
		}
	}
	return r
}

// --- rootSkillsProvider implements taskboard.SkillsProvider ---

type rootSkillsProvider struct {
	cfg *Config
}

func (s *rootSkillsProvider) SelectSkills(task dtypes.Task) []config.SkillConfig {
	rootTask := Task(task)
	skills := selectSkills(s.cfg, rootTask)
	out := make([]config.SkillConfig, len(skills))
	for i, sk := range skills {
		out[i] = config.SkillConfig(sk)
	}
	return out
}

func (s *rootSkillsProvider) LoadFailuresByName(skillName string) string {
	return loadSkillFailuresByName(s.cfg, skillName)
}

func (s *rootSkillsProvider) AppendFailure(skillName, taskTitle, agentName, errMsg string) {
	appendSkillFailure(s.cfg, skillName, taskTitle, agentName, errMsg)
}

func (s *rootSkillsProvider) MaxInject() int {
	return skillFailuresMaxInject
}

// --- rootDelegationProcessor implements taskboard.DelegationProcessor ---

type rootDelegationProcessor struct {
	cfg      *Config
	state    *dispatchState
	sem      chan struct{}
	childSem chan struct{}
}

func (p *rootDelegationProcessor) Parse(output string) []taskboard.AutoDelegation {
	raw := parseAutoDelegate(output)
	out := make([]taskboard.AutoDelegation, len(raw))
	for i, d := range raw {
		out[i] = taskboard.AutoDelegation{Agent: d.Agent, Task: d.Task, Reason: d.Reason}
	}
	return out
}

func (p *rootDelegationProcessor) Process(ctx context.Context, delegations []taskboard.AutoDelegation, output, fromAgent string) {
	raw := make([]AutoDelegation, len(delegations))
	for i, d := range delegations {
		raw[i] = AutoDelegation{Agent: d.Agent, Task: d.Task, Reason: d.Reason}
	}
	processAutoDelegations(ctx, p.cfg, raw, output, "", fromAgent, "", p.state, p.sem, p.childSem, nil)
}

// --- discordSender extracts the DiscordEmbedSender from dispatchState ---

// discordBotAdapter wraps *DiscordBot to satisfy taskboard.DiscordEmbedSender
// (exported method SendEmbed delegates to unexported sendEmbed).
type discordBotAdapter struct {
	bot *DiscordBot
}

func (a *discordBotAdapter) SendEmbed(channelID string, embed discord.Embed) {
	a.bot.sendEmbed(channelID, embed)
}

func discordSender(state *dispatchState) taskboard.DiscordEmbedSender {
	if state == nil || state.discordBot == nil {
		return nil
	}
	return &discordBotAdapter{bot: state.discordBot}
}
