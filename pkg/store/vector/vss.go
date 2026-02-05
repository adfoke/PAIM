package vector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// Store wraps vector search operations using sqlite-vss.
type Store struct {
	db      *sql.DB
	enabled bool
	dim     int
}

func New(db *sql.DB, enabled bool, dim int) *Store {
	return &Store{db: db, enabled: enabled, dim: dim}
}

func (s *Store) Enabled() bool { return s.enabled }

// UpsertEmbedding stores an embedding linked to a memory log id.
func (s *Store) UpsertEmbedding(ctx context.Context, logID string, embedding []float64) error {
	if !s.enabled {
		return nil
	}
	if len(embedding) == 0 {
		return errors.New("embedding is empty")
	}
	if s.dim > 0 && len(embedding) != s.dim {
		return fmt.Errorf("embedding dimension mismatch: got %d want %d", len(embedding), s.dim)
	}

	vec := toJSON(embedding)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx, `INSERT INTO vss_memories(content_embedding) VALUES (json(?))`, vec)
	if err != nil {
		return err
	}
	rowID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO vss_payload(rowid, log_id) VALUES (?, ?)`, rowID, logID); err != nil {
		return err
	}
	return tx.Commit()
}

// Search returns log ids ordered by vector similarity.
func (s *Store) Search(ctx context.Context, embedding []float64, topK int) ([]string, error) {
	if !s.enabled {
		return nil, nil
	}
	if topK <= 0 {
		topK = 5
	}
	if s.dim > 0 && len(embedding) != s.dim {
		return nil, fmt.Errorf("embedding dimension mismatch: got %d want %d", len(embedding), s.dim)
	}

	vec := toJSON(embedding)

	rows, err := s.db.QueryContext(ctx, `
        SELECT p.log_id
        FROM vss_memories
        JOIN vss_payload p ON p.rowid = vss_memories.rowid
        WHERE content_embedding MATCH vss_search(json(?))
        LIMIT ?;`, vec, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func toJSON(vec []float64) string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range vec {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf("%g", v))
	}
	b.WriteString("]")
	return b.String()
}
