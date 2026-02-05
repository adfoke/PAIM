package model

import (
	"context"
	"time"
)

// SensoryInput represents raw input captured by the assistant.
type SensoryInput struct {
	Content  string                 `json:"content"`
	Source   string                 `json:"source"`
	Metadata map[string]interface{} `json:"metadata"`
}

// LogEntry mirrors memory_logs rows.
type LogEntry struct {
	ID         string                 `json:"id"`
	Timestamp  time.Time              `json:"timestamp"`
	SourceType string                 `json:"source_type"`
	Content    string                 `json:"content"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// Triple represents a semantic fact.
type Triple struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	Confidence float64   `json:"confidence"`
	CreatedAt  time.Time `json:"created_at"`
}

// RecalledContext combines vector and graph results.
type RecalledContext struct {
	RelatedLogs  []LogEntry `json:"related_logs"`
	RelatedFacts []Triple   `json:"related_facts"`
}

// MemoryStore captures the core interface described in README.
type MemoryStore interface {
	Observe(ctx context.Context, input SensoryInput) error
	Recall(ctx context.Context, query string, topK int) (*RecalledContext, error)
	Consolidate(ctx context.Context) error
}

// EmbeddingClient produces embeddings compatible with SQLite-VSS.
type EmbeddingClient interface {
	EmbedText(ctx context.Context, text string) ([]float64, error)
}
