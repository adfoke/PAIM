package store

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/johncui/PAIM/pkg/engine/distill"
	"github.com/johncui/PAIM/pkg/memory"
	"github.com/johncui/PAIM/pkg/model"
	"github.com/johncui/PAIM/pkg/store/graph"
	"github.com/johncui/PAIM/pkg/store/sqlite"
	"github.com/johncui/PAIM/pkg/store/vector"
)

// Options configures MemoryEngine.
type Options struct {
	DBPath         string
	EnableVSS      bool
	ExtensionsPath string
	VectorDim      int
	BufferSize     int
	BufferTTL      time.Duration
	Embedder       model.EmbeddingClient
	Distiller      distill.Distiller
	Logger         *slog.Logger
}

// MemoryEngine implements the MemoryStore interface.
type MemoryEngine struct {
	db        *sqlite.Database
	vec       *vector.Store
	graph     *graph.Store
	buffer    *memory.SensoryBuffer
	embedder  model.EmbeddingClient
	distiller distill.Distiller
	logger    *slog.Logger
}

// NewMemoryEngine initializes storage layers.
func NewMemoryEngine(ctx context.Context, opt Options) (*MemoryEngine, error) {
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	if opt.BufferSize == 0 {
		opt.BufferSize = 128
	}
	if opt.BufferTTL == 0 {
		opt.BufferTTL = 30 * time.Minute
	}
	db, err := sqlite.New(ctx, sqlite.Config{
		Path:           opt.DBPath,
		EnableVSS:      opt.EnableVSS,
		ExtensionsPath: opt.ExtensionsPath,
		VectorDim:      opt.VectorDim,
		Logger:         opt.Logger,
	})
	if err != nil {
		return nil, err
	}

	vec := vector.New(db.DB(), db.HasVSS(), db.VectorDim())
	gr := graph.New(db.DB())
	buf := memory.NewSensoryBuffer(opt.BufferSize, opt.BufferTTL)

	dist := opt.Distiller
	if dist == nil {
		dist = distill.NewHeuristic()
	}

	emb := opt.Embedder
	if emb == nil {
		emb = NewHashEmbedder(db.VectorDim())
	}

	return &MemoryEngine{
		db:        db,
		vec:       vec,
		graph:     gr,
		buffer:    buf,
		embedder:  emb,
		distiller: dist,
		logger:    opt.Logger,
	}, nil
}

// Observe writes to sensory buffer and durable log, and optionally vector index.
func (m *MemoryEngine) Observe(ctx context.Context, input model.SensoryInput) error {
	logID, err := m.db.InsertLog(ctx, input)
	if err != nil {
		return err
	}
	m.buffer.Add(input)

	if m.vec.Enabled() && m.embedder != nil {
		emb, err := m.embedder.EmbedText(ctx, input.Content)
		if err != nil {
			return err
		}
		if err := m.vec.UpsertEmbedding(ctx, logID, emb); err != nil {
			return err
		}
	}
	return nil
}

// Recall performs graph + vector retrieval.
func (m *MemoryEngine) Recall(ctx context.Context, query string, topK int) (*model.RecalledContext, error) {
	facts, err := m.graph.SearchFacts(ctx, query, topK)
	if err != nil {
		return nil, err
	}

	var logs []model.LogEntry
	if m.vec.Enabled() && m.embedder != nil {
		emb, err := m.embedder.EmbedText(ctx, query)
		if err != nil {
			return nil, err
		}
		ids, err := m.vec.Search(ctx, emb, topK)
		if err != nil {
			return nil, err
		}
		logs, err = m.db.FetchLogs(ctx, ids)
		if err != nil {
			return nil, err
		}
	}

	return &model.RecalledContext{RelatedLogs: logs, RelatedFacts: facts}, nil
}

// Consolidate distills buffered sensory inputs into triples and writes to graph.
func (m *MemoryEngine) Consolidate(ctx context.Context) error {
	snapshot := m.buffer.Snapshot()
	if len(snapshot) == 0 {
		return nil
	}

	triples, err := m.distiller.Distill(ctx, snapshot)
	if err != nil {
		return err
	}
	for _, t := range triples {
		if _, err := m.graph.UpsertTriple(ctx, t); err != nil {
			return err
		}
	}
	m.buffer.Clear()
	return nil
}

// Close releases resources.
func (m *MemoryEngine) Close() error {
	return m.db.Close()
}

// HashEmbedder is a deterministic, dependency-free embedding stub to keep the
// system local-first by default. Replace with real embedding service when available.
type HashEmbedder struct {
	dim int
}

func NewHashEmbedder(dim int) *HashEmbedder {
	if dim <= 0 {
		dim = 1536
	}
	return &HashEmbedder{dim: dim}
}

// EmbedText hashes the text into a pseudo-random but deterministic vector.
func (h *HashEmbedder) EmbedText(_ context.Context, text string) ([]float64, error) {
	if text == "" {
		text = "empty"
	}
	hash := sha256.Sum256([]byte(text))
	vec := make([]float64, h.dim)
	for i := 0; i < h.dim; i++ {
		// spread hash bits across dimensions
		chunk := binary.LittleEndian.Uint16(hash[(i % 16):])
		vec[i] = float64(chunk%1000) / 1000.0
	}
	// normalize lightly
	var sum float64
	for _, v := range vec {
		sum += v * v
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		norm = 1
	}
	for i := range vec {
		vec[i] /= norm
	}
	return vec, nil
}

var _ model.MemoryStore = (*MemoryEngine)(nil)
var _ model.EmbeddingClient = (*HashEmbedder)(nil)
