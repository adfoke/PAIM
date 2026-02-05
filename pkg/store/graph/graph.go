package graph

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/johncui/PAIM/pkg/model"
)

// Store encapsulates CRUD for triples.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// UpsertTriple inserts or updates confidence if duplicate.
func (s *Store) UpsertTriple(ctx context.Context, t model.Triple) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
        INSERT INTO triples(subject, predicate, object, confidence)
        VALUES(?, ?, ?, ?)
        ON CONFLICT(subject, predicate, object) DO UPDATE SET confidence=excluded.confidence;
    `, t.Subject, t.Predicate, t.Object, t.Confidence)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// SearchFacts performs a LIKE-based search on subject/object and limits results.
func (s *Store) SearchFacts(ctx context.Context, term string, limit int) ([]model.Triple, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, subject, predicate, object, confidence, created_at
        FROM triples
        WHERE subject LIKE ? OR object LIKE ?
        ORDER BY created_at DESC
        LIMIT ?;
    `, "%"+term+"%", "%"+term+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Triple
	for rows.Next() {
		var t model.Triple
		if err := rows.Scan(&t.ID, &t.Subject, &t.Predicate, &t.Object, &t.Confidence, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// OneHopNeighbors returns triples connected to an entity.
func (s *Store) OneHopNeighbors(ctx context.Context, entity string, limit int) ([]model.Triple, error) {
	rows, err := s.db.QueryContext(ctx, `
        SELECT id, subject, predicate, object, confidence, created_at
        FROM triples
        WHERE subject = ? OR object = ?
        ORDER BY confidence DESC, created_at DESC
        LIMIT ?;
    `, entity, entity, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []model.Triple
	for rows.Next() {
		var t model.Triple
		if err := rows.Scan(&t.ID, &t.Subject, &t.Predicate, &t.Object, &t.Confidence, &t.CreatedAt); err != nil {
			return nil, err
		}
		res = append(res, t)
	}
	return res, rows.Err()
}

// DeleteAll clears triples. Useful for tests.
func (s *Store) DeleteAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM triples; VACUUM;`)
	return err
}

// DebugDump returns all triples for logging.
func (s *Store) DebugDump(ctx context.Context) ([]model.Triple, error) {
	return s.SearchFacts(ctx, "", 100)
}

func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM triples;`).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Store) String() string {
	cnt, _ := s.Count(context.Background())
	return fmt.Sprintf("graphStore(count=%d)", cnt)
}
