package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"

	iwf "tetora/internal/workflow"
	"tetora/internal/version"
)

// ConfigVersion is an alias for version.Version so that existing callers in
// the root package continue to compile without modification.
type ConfigVersion = version.Version

//go:embed examples/templates/*.json
var templateFS embed.FS

// --- Type Aliases ---
// These aliases let root-package code continue to use the unqualified names
// while the canonical definitions live in internal/workflow.

type Workflow = iwf.Workflow
type WorkflowStep = iwf.WorkflowStep
type ResponseMapping = iwf.ResponseMapping
type WorkflowContext = iwf.WorkflowContext
type WorkflowStepResult = iwf.WorkflowStepResult
type TemplateSummary = iwf.TemplateSummary

// --- Directory helpers ---

func workflowDir(cfg *Config) string      { return iwf.WorkflowDir(cfg) }
func ensureWorkflowDir(cfg *Config) error { return iwf.EnsureWorkflowDir(cfg) }

// --- Load / Save ---

func loadWorkflow(path string) (*Workflow, error) { return iwf.LoadWorkflow(path) }

func loadWorkflowByName(cfg *Config, name string) (*Workflow, error) {
	return iwf.LoadWorkflowByName(cfg, name)
}

func saveWorkflow(cfg *Config, w *Workflow) error { return iwf.SaveWorkflow(cfg, w) }

func deleteWorkflow(cfg *Config, name string) error { return iwf.DeleteWorkflow(cfg, name) }

func listWorkflows(cfg *Config) ([]*Workflow, error) { return iwf.ListWorkflows(cfg) }

// --- Validation ---

func validateWorkflow(w *Workflow) []string { return iwf.ValidateWorkflow(w) }
func validateStep(s WorkflowStep, allIDs map[string]bool) []string {
	return iwf.ValidateStep(s, allIDs)
}

// --- DAG helpers ---

func detectCycle(steps []WorkflowStep) string       { return iwf.DetectCycle(steps) }
func topologicalSort(steps []WorkflowStep) []string { return iwf.TopologicalSort(steps) }

// --- Variable Template System ---

// templateVarRe matches {{...}} template expressions — re-exported from internal/workflow.
var templateVarRe = iwf.TemplateVarRe

func newWorkflowContext(w *Workflow, inputOverrides map[string]string) *WorkflowContext {
	return iwf.NewWorkflowContext(w, inputOverrides)
}

func resolveTemplate(tmpl string, wCtx *WorkflowContext) string {
	return iwf.ResolveTemplate(tmpl, wCtx)
}

func resolveExpr(expr string, wCtx *WorkflowContext) string {
	return iwf.ResolveExpr(expr, wCtx)
}

// --- Condition Evaluation ---

func evalCondition(expr string, wCtx *WorkflowContext) bool {
	return iwf.EvalCondition(expr, wCtx)
}

// --- Utility ---

func getStepByID(w *Workflow, id string) *WorkflowStep { return iwf.GetStepByID(w, id) }
func stepType(s *WorkflowStep) string                  { return iwf.StepType(s) }

// --- Template Gallery ---

// cachedTemplates holds the pre-computed template summaries (static from embed.FS).
var cachedTemplates []TemplateSummary

// listTemplates returns summaries of all embedded workflow templates (cached after first call).
func listTemplates() []TemplateSummary {
	if cachedTemplates != nil {
		return cachedTemplates
	}
	entries, err := templateFS.ReadDir("examples/templates")
	if err != nil {
		return nil
	}
	var templates []TemplateSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := templateFS.ReadFile("examples/templates/" + e.Name())
		if err != nil {
			continue
		}
		var wf Workflow
		if err := json.Unmarshal(data, &wf); err != nil {
			continue
		}
		var varNames []string
		for k := range wf.Variables {
			varNames = append(varNames, k)
		}
		category := ""
		name := strings.TrimPrefix(wf.Name, "tpl-")
		if idx := strings.Index(name, "-"); idx > 0 {
			category = name[:idx]
		}
		templates = append(templates, TemplateSummary{
			Name:        wf.Name,
			Description: wf.Description,
			StepCount:   len(wf.Steps),
			Variables:   varNames,
			Category:    category,
		})
	}
	cachedTemplates = templates
	return templates
}

// loadTemplate loads a full workflow template by name.
func loadTemplate(name string) (*Workflow, error) {
	// Sanitize to prevent path traversal.
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, "..") {
		return nil, fmt.Errorf("invalid template name")
	}
	fileName := name + ".json"
	if !strings.HasPrefix(name, "tpl-") {
		fileName = "tpl-" + fileName
	}
	data, err := templateFS.ReadFile("examples/templates/" + fileName)
	if err != nil {
		return nil, fmt.Errorf("template %q not found", name)
	}
	var wf Workflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	return &wf, nil
}

// installTemplate copies a template to the user's workflows directory with an optional new name.
func installTemplate(cfg *Config, templateName, newName string) error {
	wf, err := loadTemplate(templateName)
	if err != nil {
		return err
	}
	if newName != "" {
		wf.Name = newName
	}
	return saveWorkflow(cfg, wf)
}

// buildStepTask converts a workflow dispatch step into a Task for execution.
func buildStepTask(s *WorkflowStep, wCtx *WorkflowContext, workflowName string) Task {
	return Task{
		ID:             newUUID(),
		Name:           fmt.Sprintf("%s/%s", workflowName, s.ID),
		Prompt:         resolveTemplate(s.Prompt, wCtx),
		Agent:          resolveTemplate(s.Agent, wCtx),
		Model:          resolveTemplate(s.Model, wCtx),
		Provider:       resolveTemplate(s.Provider, wCtx),
		Timeout:        resolveTemplate(s.Timeout, wCtx),
		Budget:         s.Budget,
		PermissionMode: resolveTemplate(s.PermissionMode, wCtx),
		Source:         "workflow:" + workflowName,
	}
}

// restoreWorkflowVersion restores a workflow to a saved version.
func restoreWorkflowVersion(dbPath string, cfg *Config, versionID string) error {
	return iwf.RestoreWorkflowVersion(dbPath, cfg, versionID)
}
