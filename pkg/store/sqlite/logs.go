package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/johncui/PAIM/pkg/model"
)

// InsertLog writes a new memory_log row and returns its id.
func (d *Database) InsertLog(ctx context.Context, input model.SensoryInput) (string, error) {
	if input.Content == "" {
		return "", fmt.Errorf("content is required")
	}
	id := uuid.NewString()
	metaBytes, _ := json.Marshal(input.Metadata)

	_, err := d.db.ExecContext(ctx, `
        INSERT INTO memory_logs(id, timestamp, source_type, content, metadata)
        VALUES(?, CURRENT_TIMESTAMP, ?, ?, ?);
    `, id, input.Source, input.Content, string(metaBytes))
	if err != nil {
		return "", err
	}
	return id, nil
}

// FetchLogs retrieves logs by ids preserving order as best-effort.
func (d *Database) FetchLogs(ctx context.Context, ids []string) ([]model.LogEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	query := `SELECT id, timestamp, source_type, content, metadata FROM memory_logs WHERE id IN (` + placeholders(len(ids)) + `)`
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := d.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]model.LogEntry, 0, len(ids))
	for rows.Next() {
		var e model.LogEntry
		var meta sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.SourceType, &e.Content, &meta); err != nil {
			return nil, err
		}
		if meta.Valid && meta.String != "" {
			_ = json.Unmarshal([]byte(meta.String), &e.Metadata)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, 2*n)
	for i := 0; i < n; i++ {
		out = append(out, '?')
		if i != n-1 {
			out = append(out, ',')
		}
	}
	return string(out)
}

// RecentLogs fetches latest logs limited by n.
func (d *Database) RecentLogs(ctx context.Context, limit int) ([]model.LogEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.db.QueryContext(ctx, `
        SELECT id, timestamp, source_type, content, metadata
        FROM memory_logs
        ORDER BY timestamp DESC
        LIMIT ?;
    `, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.LogEntry
	for rows.Next() {
		var e model.LogEntry
		var meta sql.NullString
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.SourceType, &e.Content, &meta); err != nil {
			return nil, err
		}
		if meta.Valid && meta.String != "" {
			_ = json.Unmarshal([]byte(meta.String), &e.Metadata)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteAllLogs clears logs table.
func (d *Database) DeleteAllLogs(ctx context.Context) error {
	_, err := d.db.ExecContext(ctx, `DELETE FROM memory_logs; VACUUM;`)
	return err
}

// DB exposes internal sql.DB
func (d *Database) SQL() *sql.DB { return d.db }
