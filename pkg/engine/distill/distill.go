package distill

import (
	"context"
	"strings"

	"github.com/johncui/PAIM/pkg/model"
)

// Distiller converts short-term sensory inputs into structured triples.
type Distiller interface {
	Distill(ctx context.Context, inputs []model.SensoryInput) ([]model.Triple, error)
}

// HeuristicDistiller is a lightweight placeholder distiller using simple rules.
type HeuristicDistiller struct{}

func NewHeuristic() *HeuristicDistiller { return &HeuristicDistiller{} }

// Distill attempts to derive triples using naive heuristics:
// - If metadata contains subject/predicate/object keys, use them.
// - Otherwise, create a generic "notes" triple linking source -> content snippet.
func (h *HeuristicDistiller) Distill(_ context.Context, inputs []model.SensoryInput) ([]model.Triple, error) {
	var triples []model.Triple
	for _, in := range inputs {
		subject, _ := in.Metadata["subject"].(string)
		predicate, _ := in.Metadata["predicate"].(string)
		object, _ := in.Metadata["object"].(string)
		if subject != "" && predicate != "" && object != "" {
			triples = append(triples, model.Triple{
				Subject:    subject,
				Predicate:  predicate,
				Object:     object,
				Confidence: 0.9,
			})
			continue
		}

		snippet := strings.TrimSpace(in.Content)
		if len(snippet) > 80 {
			snippet = snippet[:80]
		}
		if snippet == "" {
			continue
		}
		triples = append(triples, model.Triple{
			Subject:    defaultIfEmpty(in.Source, "user"),
			Predicate:  "notes",
			Object:     snippet,
			Confidence: 0.4,
		})
	}
	return triples, nil
}

func defaultIfEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
