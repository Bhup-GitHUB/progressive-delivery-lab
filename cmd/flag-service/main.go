package main

import (
	"net/http"
	"sync"

	"progressive-delivery-lab/internal/env"
	"progressive-delivery-lab/internal/flags"
	"progressive-delivery-lab/internal/httpx"
)

type store struct {
	mu         sync.RWMutex
	fraudModel flags.FraudModel
}

type flagUpdate struct {
	Enabled        bool `json:"enabled"`
	RolloutPercent int  `json:"rolloutPercent"`
}

func main() {
	s := &store{
		fraudModel: flags.FraudModel{
			Enabled:        env.Int("FRAUD_MODEL_ENABLED", 0) == 1,
			RolloutPercent: clampPercent(env.Int("FRAUD_MODEL_ROLLOUT_PERCENT", 0)),
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /flags", s.getFlags)
	mux.HandleFunc("POST /flags/fraud-model", s.updateFraudModel)
	mux.HandleFunc("POST /flags/fraud-model/kill", s.killFraudModel)

	addr := env.String("ADDR", ":8080")
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func (s *store) health(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *store) getFlags(w http.ResponseWriter, r *http.Request) {
	httpx.JSON(w, http.StatusOK, s.snapshot())
}

func (s *store) updateFraudModel(w http.ResponseWriter, r *http.Request) {
	var input flagUpdate
	if err := httpx.DecodeJSON(r, &input); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	s.mu.Lock()
	s.fraudModel = flags.FraudModel{
		Enabled:        input.Enabled,
		RolloutPercent: clampPercent(input.RolloutPercent),
	}
	current := s.fraudModel
	s.mu.Unlock()

	httpx.JSON(w, http.StatusOK, flags.Response{FraudModel: current})
}

func (s *store) killFraudModel(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.fraudModel = flags.FraudModel{}
	s.mu.Unlock()

	httpx.JSON(w, http.StatusOK, s.snapshot())
}

func (s *store) snapshot() flags.Response {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return flags.Response{FraudModel: s.fraudModel}
}

func clampPercent(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
