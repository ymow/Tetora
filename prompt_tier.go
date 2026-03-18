package main

import (
	"tetora/internal/classify"
	"tetora/internal/prompt"
)

func buildTieredPrompt(cfg *Config, task *Task, agentName string, complexity classify.Complexity) {
	prompt.BuildTieredPrompt(cfg, task, agentName, complexity, prompt.Deps{
		ResolveProviderName:    resolveProviderName,
		LoadSoulFile:           loadSoulFile,
		LoadAgentPrompt:        loadAgentPrompt,
		ResolveWorkspace:       resolveWorkspace,
		BuildReflectionContext: buildReflectionContext,
		LoadWritingStyle:       loadWritingStyle,
		BuildSkillsPrompt:      buildSkillsPrompt,
		InjectWorkspaceContent: injectWorkspaceContent,
		EstimateDirSize:        estimateDirSize,
	})
}

func truncateToChars(s string, maxChars int) string {
	return prompt.TruncateToChars(s, maxChars)
}

func truncateLessonsToRecent(content string, n int) string {
	return prompt.TruncateLessonsToRecent(content, n)
}
