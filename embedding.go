package main

import (
	"context"
	"time"

	"tetora/internal/knowledge"
)

// EmbeddingSearchResult is a type alias for knowledge.EmbeddingSearchResult.
type EmbeddingSearchResult = knowledge.EmbeddingSearchResult

// embeddingRecord is a type alias for knowledge.EmbeddingRecord (used in tests).
type embeddingRecord = knowledge.EmbeddingRecord

// embeddingCfg converts root config's EmbeddingConfig to knowledge package's type.
func embeddingCfg(cfg EmbeddingConfig) knowledge.EmbeddingConfig {
	return knowledge.EmbeddingConfig{
		Enabled:    cfg.Enabled,
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		Endpoint:   cfg.Endpoint,
		APIKey:     cfg.APIKey,
		Dimensions: cfg.Dimensions,
		BatchSize:  cfg.BatchSize,
		MMR: knowledge.MMRConfig{
			Enabled: cfg.MMR.Enabled,
			Lambda:  cfg.MMR.Lambda,
		},
		TemporalDecay: knowledge.TemporalConfig{
			Enabled:      cfg.TemporalDecay.Enabled,
			HalfLifeDays: cfg.TemporalDecay.HalfLifeDays,
		},
	}
}

func initEmbeddingDB(dbPath string) error {
	return knowledge.InitEmbeddingDB(dbPath)
}

func getEmbeddings(ctx context.Context, cfg *Config, texts []string) ([][]float32, error) {
	return knowledge.GetEmbeddings(ctx, embeddingCfg(cfg.Embedding), texts)
}

func getEmbedding(ctx context.Context, cfg *Config, text string) ([]float32, error) {
	return knowledge.GetEmbedding(ctx, embeddingCfg(cfg.Embedding), text)
}

func storeEmbedding(dbPath string, source, sourceID, content string, vec []float32, metadata map[string]interface{}) error {
	return knowledge.StoreEmbedding(dbPath, source, sourceID, content, vec, metadata)
}

func loadEmbeddings(dbPath, source string) ([]embeddingRecord, error) {
	return knowledge.LoadEmbeddings(dbPath, source)
}

func vectorSearch(dbPath string, queryVec []float32, source string, topK int) ([]EmbeddingSearchResult, error) {
	return knowledge.VectorSearch(dbPath, queryVec, source, topK)
}

func hybridSearch(ctx context.Context, cfg *Config, query string, source string, topK int) ([]EmbeddingSearchResult, error) {
	return knowledge.HybridSearch(ctx, embeddingCfg(cfg.Embedding), cfg.HistoryDB, cfg.KnowledgeDir, query, source, topK)
}

func reindexAll(ctx context.Context, cfg *Config) error {
	return knowledge.ReindexAll(ctx, embeddingCfg(cfg.Embedding), cfg.HistoryDB, cfg.KnowledgeDir)
}

func embeddingStatus(dbPath string) (map[string]interface{}, error) {
	return knowledge.EmbeddingStatus(dbPath)
}

// Unexported wrappers used by embedding_test.go (package main).

func cosineSimilarity(a, b []float32) float32 {
	return knowledge.CosineSimilarity(a, b)
}

func serializeVec(vec []float32) []byte {
	return knowledge.SerializeVec(vec)
}

func deserializeVec(data []byte) []float32 {
	return knowledge.DeserializeVec(data)
}

func deserializeVecFromHex(hexStr string) []float32 {
	return knowledge.DeserializeVecFromHex(hexStr)
}

func contentHashSHA256(content string) string {
	return knowledge.ContentHashSHA256(content)
}

func rrfMerge(a, b []EmbeddingSearchResult, k int) []EmbeddingSearchResult {
	return knowledge.RRFMerge(a, b, k)
}

func mmrRerank(results []EmbeddingSearchResult, queryVec []float32, lambda float64, topK int) []EmbeddingSearchResult {
	return knowledge.MMRRerank(results, queryVec, lambda, topK)
}

func contentToVec(content string, dims int) []float32 {
	return knowledge.ContentToVec(content, dims)
}

func temporalDecay(score float64, createdAt time.Time, halfLifeDays float64) float64 {
	return knowledge.TemporalDecay(score, createdAt, halfLifeDays)
}

func chunkText(text string, maxChars, overlap int) []string {
	return knowledge.ChunkText(text, maxChars, overlap)
}

func embeddingMMRLambdaOrDefault(cfg EmbeddingConfig) float64 {
	return knowledge.EmbeddingConfig(embeddingCfg(cfg)).MmrLambdaOrDefault()
}

func embeddingDecayHalfLifeOrDefault(cfg EmbeddingConfig) float64 {
	return knowledge.EmbeddingConfig(embeddingCfg(cfg)).DecayHalfLifeOrDefault()
}
