package main

import "tetora/internal/classify"

// RequestComplexity is an alias for classify.Complexity.
type RequestComplexity = classify.Complexity

const (
	ComplexitySimple   = classify.Simple
	ComplexityStandard = classify.Standard
	ComplexityComplex  = classify.Complex
)

// chatSources re-exports classify.ChatSources for use by isChatSource() in sprite.go.
var chatSources = classify.ChatSources

func classifyComplexity(prompt, source string) RequestComplexity {
	return classify.Classify(prompt, source)
}

func complexityMaxSessionMessages(c RequestComplexity) int {
	return classify.MaxSessionMessages(c)
}

func complexityMaxSessionChars(c RequestComplexity) int {
	return classify.MaxSessionChars(c)
}
