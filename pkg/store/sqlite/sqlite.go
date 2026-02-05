package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Config controls SQLite initialization.
type Config struct {
	Path           string
	ExtensionsPath string
	EnableVSS      bool
	VectorDim      int
	Logger         *slog.Logger
}

// Database wraps the sql.DB handle with feature flags.
type Database struct {
	db        *sql.DB
	enableVSS bool
	vectorDim int
	logger    *slog.Logger
}

// New opens the database, loads extensions if requested, and ensures schema.
func New(ctx context.Context, cfg Config) (*Database, error) {
	if cfg.Path == "" {
		return nil, errors.New("database path is required")
	}

	if cfg.VectorDim == 0 {
		cfg.VectorDim = 1536
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", cfg.Path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(5 * time.Minute)

	wrapper := &Database{db: db, enableVSS: cfg.EnableVSS, vectorDim: cfg.VectorDim, logger: cfg.Logger}

	if cfg.EnableVSS {
		if err := wrapper.loadExtension(ctx, cfg.ExtensionsPath); err != nil {
			return nil, fmt.Errorf("load sqlite-vss extension: %w", err)
		}
	}

	if err := wrapper.ensureSchema(ctx); err != nil {
		return nil, err
	}

	return wrapper, nil
}

func (d *Database) loadExtension(ctx context.Context, extPath string) error {
	if extPath == "" {
		if env := os.Getenv("GO_SQLITE3_EXTENSIONS"); env != "" {
			extPath = env
		}
	}
	if extPath == "" {
		return errors.New("extension path not provided")
	}

	dsn := fmt.Sprintf("file:%s", extPath)
	d.logger.Info("loading sqlite extension", "path", dsn)

	conn, err := d.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// enable extension loading per connection
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL; PRAGMA enable_load_extension=1;"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "SELECT load_extension(?)", extPath); err != nil {
		return err
	}
	return nil
}

func (d *Database) ensureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS memory_logs (
            id TEXT PRIMARY KEY,
            timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
            source_type TEXT,
            content TEXT,
            metadata JSON
        );`,
		`CREATE TABLE IF NOT EXISTS triples (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            subject TEXT NOT NULL,
            predicate TEXT NOT NULL,
            object TEXT NOT NULL,
            confidence REAL DEFAULT 1.0,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            UNIQUE(subject, predicate, object)
        );`,
		`CREATE INDEX IF NOT EXISTS idx_subject ON triples(subject);`,
		`CREATE INDEX IF NOT EXISTS idx_object ON triples(object);`,
	}

	// vector schema if enabled
	if d.enableVSS {
		stmts = append(stmts,
			fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS vss_memories USING vss0(content_embedding(%d));`, d.vectorDim),
			`CREATE TABLE IF NOT EXISTS vss_payload (
                rowid INTEGER PRIMARY KEY,
                log_id TEXT NOT NULL
            );`,
		)
	}

	for _, stmt := range stmts {
		if _, err := d.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// DB returns the underlying database handle.
func (d *Database) DB() *sql.DB {
	return d.db
}

// Close releases the database.
func (d *Database) Close() error {
	return d.db.Close()
}

// HasVSS indicates whether vector search is available.
func (d *Database) HasVSS() bool {
	return d.enableVSS
}

// VectorDim returns configured embedding dimension.
func (d *Database) VectorDim() int {
	return d.vectorDim
}
