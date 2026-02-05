package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/johncui/PAIM/pkg/model"
	"github.com/johncui/PAIM/pkg/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := loadConfig()

	ctx := context.Background()
	engine, err := store.NewMemoryEngine(ctx, store.Options{
		DBPath:         cfg.DBPath,
		EnableVSS:      cfg.EnableVSS,
		ExtensionsPath: cfg.ExtensionsPath,
		VectorDim:      cfg.VectorDim,
		BufferSize:     cfg.BufferSize,
		BufferTTL:      cfg.BufferTTL,
		Logger:         logger,
	})
	if err != nil {
		log.Fatalf("failed to init engine: %v", err)
	}
	defer engine.Close()

	go startConsolidationLoop(ctx, engine, cfg.ConsolidationEvery, logger)

	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Logger, middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	r.Post("/remember", func(w http.ResponseWriter, req *http.Request) {
		var in model.SensoryInput
		if err := json.NewDecoder(req.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if in.Source == "" {
			in.Source = "chat"
		}
		if err := engine.Observe(req.Context(), in); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	r.Get("/ask", func(w http.ResponseWriter, req *http.Request) {
		query := req.URL.Query().Get("q")
		topKStr := req.URL.Query().Get("k")
		topK := 5
		if topKStr != "" {
			if v, err := strconv.Atoi(topKStr); err == nil {
				topK = v
			}
		}
		res, err := engine.Recall(req.Context(), query, topK)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, res)
	})

	addr := cfg.ListenAddr
	logger.Info("starting PAIM server", "addr", addr, "db", cfg.DBPath, "vss", cfg.EnableVSS)
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// ------------ config & helpers ------------

type config struct {
	ListenAddr         string
	DBPath             string
	EnableVSS          bool
	ExtensionsPath     string
	VectorDim          int
	BufferSize         int
	BufferTTL          time.Duration
	ConsolidationEvery time.Duration
}

func loadConfig() config {
	return config{
		ListenAddr:         getenv("PAIM_LISTEN_ADDR", ":8080"),
		DBPath:             getenv("PAIM_DB_PATH", "paim.db"),
		EnableVSS:          getenvBool("PAIM_ENABLE_VSS", false),
		ExtensionsPath:     os.Getenv("GO_SQLITE3_EXTENSIONS"),
		VectorDim:          getenvInt("PAIM_VECTOR_DIM", 1536),
		BufferSize:         getenvInt("PAIM_BUFFER_SIZE", 128),
		BufferTTL:          getenvDuration("PAIM_BUFFER_TTL", 30*time.Minute),
		ConsolidationEvery: getenvDuration("PAIM_CONSOLIDATION_EVERY", 5*time.Minute),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getenvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func startConsolidationLoop(ctx context.Context, engine model.MemoryStore, every time.Duration, logger *slog.Logger) {
	if every <= 0 {
		every = 5 * time.Minute
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := engine.Consolidate(ctx); err != nil {
				logger.Error("consolidation failed", "err", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
