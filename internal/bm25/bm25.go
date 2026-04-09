// Package bm25 provides BM25 (Best Matching 25) text ranking for search.
// BM25 is a bag-of-words retrieval function that ranks documents based on
// query terms appearing in each document, incorporating term frequency
// saturation and document length normalization.
package bm25

import (
	"math"
	"strings"
	"unicode"
)

// Default parameters for BM25.
// k1 controls term frequency saturation (typical range: 1.2–2.0).
// b controls document length normalization (typical range: 0.5–1.0).
const (
	DefaultK1 = 1.5
	DefaultB  = 0.75
)

// Document represents a searchable document with an ID and tokenized content.
type Document struct {
	ID    string
	Terms []string
}

// BM25 holds the precomputed BM25 index for a document collection.
type BM25 struct {
	k1        float64
	b         float64
	avgDocLen float64
	docCount  int
	idf       map[string]float64       // term -> IDF value
	docTerms  map[string][]string       // doc ID -> terms
	docLens   map[string]int            // doc ID -> term count
}

// Tokenize splits text into lowercase alphabetic tokens.
// Non-alphabetic characters act as separators.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string
	var current strings.Builder
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// New creates a BM25 index from the given documents.
func New(docs []Document, k1, b float64) *BM25 {
	if k1 <= 0 {
		k1 = DefaultK1
	}
	if b < 0 || b > 1 {
		b = DefaultB
	}

	bm := &BM25{
		k1:       k1,
		b:        b,
		idf:      make(map[string]float64),
		docTerms: make(map[string][]string),
		docLens:  make(map[string]int),
		docCount: len(docs),
	}

	// Build term frequency per document and document frequency for IDF.
	docFreq := make(map[string]int) // term -> number of docs containing it
	var totalLen int

	for _, doc := range docs {
		bm.docTerms[doc.ID] = doc.Terms
		docLen := len(doc.Terms)
		bm.docLens[doc.ID] = docLen
		totalLen += docLen

		seen := make(map[string]bool)
		for _, term := range doc.Terms {
			if !seen[term] {
				docFreq[term]++
				seen[term] = true
			}
		}
	}

	if bm.docCount > 0 {
		bm.avgDocLen = float64(totalLen) / float64(bm.docCount)
	}

	// Precompute IDF for all terms.
	for term, df := range docFreq {
		// Standard IDF: log((N - df + 0.5) / (df + 0.5) + 1)
		// This is the Robertson/Sparck Jones variant used in Lucene/Elasticsearch.
		bm.idf[term] = math.Log(1 + (float64(bm.docCount)-float64(df)+0.5)/(float64(df)+0.5))
	}

	return bm
}

// Score computes the BM25 score for a single document given query terms.
func (bm *BM25) Score(docID string, queryTerms []string) float64 {
	if bm.docCount == 0 || bm.avgDocLen == 0 {
		return 0
	}

	terms, ok := bm.docTerms[docID]
	if !ok {
		return 0
	}
	docLen := bm.docLens[docID]

	// Compute term frequencies in this document.
	tf := make(map[string]int)
	for _, t := range terms {
		tf[t]++
	}

	var score float64
	for _, q := range queryTerms {
		idf, ok := bm.idf[q]
		if !ok || idf == 0 {
			continue
		}
		freq := tf[q]
		if freq == 0 {
			continue
		}

		// BM25 term score:
		// IDF(q) * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * |D|/avgdl))
		num := float64(freq) * (bm.k1 + 1)
		denom := float64(freq) + bm.k1*(1-bm.b+bm.b*float64(docLen)/bm.avgDocLen)
		score += idf * num / denom
	}

	return score
}

// Search ranks all documents by BM25 score and returns the top N results
// sorted by descending score.
func (bm *BM25) Search(queryTerms []string, topN int) []Result {
	if len(queryTerms) == 0 || bm.docCount == 0 {
		return nil
	}

	results := make([]Result, 0, bm.docCount)
	for docID := range bm.docTerms {
		s := bm.Score(docID, queryTerms)
		if s > 0 {
			results = append(results, Result{
				ID:    docID,
				Score: s,
			})
		}
	}

	// Sort by score descending.
	sortResults(results)

	if topN > 0 && topN < len(results) {
		results = results[:topN]
	}

	return results
}

// Result holds a single search result with its BM25 score.
type Result struct {
	ID    string
	Score float64
}

// sortResults sorts results by score in descending order.
func sortResults(results []Result) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}
