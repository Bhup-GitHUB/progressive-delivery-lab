package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math"
	mathrand "math/rand/v2"
	"net/http"
	"time"

	"progressive-delivery-lab/internal/env"
	"progressive-delivery-lab/internal/flags"
	"progressive-delivery-lab/internal/httpx"
	"progressive-delivery-lab/internal/metrics"
)

type server struct {
	version       string
	errorRate     float64
	baseLatency   time.Duration
	metrics       *metrics.Window
	flagClient    *flags.Client
	startedAt     time.Time
	extraLatency  time.Duration
	fraudErrorPct float64
}

func main() {
	s := &server{
		version:       env.String("VERSION", "v1"),
		errorRate:     normalizeRate(env.Float("ERROR_RATE", 0)),
		baseLatency:   time.Duration(env.Int("BASE_LATENCY_MS", 120)) * time.Millisecond,
		metrics:       metrics.NewWindow(2000),
		flagClient:    flags.NewClient(env.String("FEATURE_FLAG_URL", "")),
		startedAt:     time.Now(),
		extraLatency:  time.Duration(env.Int("FRAUD_MODEL_LATENCY_MS", 80)) * time.Millisecond,
		fraudErrorPct: normalizeRate(env.Float("FRAUD_MODEL_ERROR_RATE", 0)),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /checkout", s.checkout)
	mux.HandleFunc("GET /fraud-check", s.fraudCheck)
	mux.HandleFunc("GET /metrics", s.serviceMetrics)

	addr := env.String("ADDR", ":8080")
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"version":   s.version,
		"uptimeSec": int(time.Since(s.startedAt).Seconds()),
	})
}

func (s *server) checkout(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	userID := requestKey(r)
	model := s.modelEnabled(r.Context(), userID)
	latency := s.baseLatency + jitter(35*time.Millisecond)
	failed := mathrand.Float64() < s.errorRate

	if model {
		latency += s.extraLatency + jitter(50*time.Millisecond)
		if mathrand.Float64() < s.fraudErrorPct {
			failed = true
		}
	}

	time.Sleep(latency)
	s.metrics.Record(time.Since(start), failed)

	if failed {
		httpx.JSON(w, http.StatusInternalServerError, map[string]any{
			"ok":              false,
			"version":         s.version,
			"fraudModelUsed":  model,
			"message":         "checkout failed",
			"latencyMs":       time.Since(start).Milliseconds(),
			"deploymentStage": deploymentStage(s.version),
		})
		return
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"version":         s.version,
		"fraudModelUsed":  model,
		"message":         "checkout approved",
		"latencyMs":       time.Since(start).Milliseconds(),
		"deploymentStage": deploymentStage(s.version),
	})
}

func (s *server) fraudCheck(w http.ResponseWriter, r *http.Request) {
	userID := requestKey(r)
	enabled := s.modelEnabled(r.Context(), userID)
	httpx.JSON(w, http.StatusOK, map[string]any{
		"version":        s.version,
		"fraudModelUsed": enabled,
		"decision":       fraudDecision(enabled),
	})
}

func (s *server) serviceMetrics(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"version": s.version,
		"metrics": s.metrics.Snapshot(),
	})
}

func (s *server) modelEnabled(ctx context.Context, userID string) bool {
	if s.version != "v2" {
		return false
	}

	response, err := s.flagClient.Flags(ctx)
	if err != nil {
		return false
	}
	return flags.EnabledFor(response.FraudModel, userID)
}

func requestKey(r *http.Request) string {
	value := r.Header.Get("X-User-ID")
	if value != "" {
		return value
	}
	value = r.URL.Query().Get("user")
	if value != "" {
		return value
	}
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return time.Now().Format(time.RFC3339Nano)
	}
	return hex.EncodeToString(bytes[:])
}

func deploymentStage(version string) string {
	if version == "v2" {
		return "canary"
	}
	return "stable"
}

func fraudDecision(enabled bool) string {
	if enabled {
		return "new-model-score"
	}
	return "legacy-rules"
}

func jitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(mathrand.Int64N(int64(max)))
}

func normalizeRate(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 1 {
		return value / 100
	}
	return value
}
