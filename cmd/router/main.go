package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	mathrand "math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"progressive-delivery-lab/internal/env"
	"progressive-delivery-lab/internal/httpx"
	"progressive-delivery-lab/internal/metrics"
)

type target string

const (
	stable target = "stable"
	canary target = "canary"
)

type router struct {
	stableURL     *url.URL
	canaryURL     *url.URL
	client        *http.Client
	mu            sync.RWMutex
	canaryPercent int
	stableMetrics *metrics.Window
	canaryMetrics *metrics.Window
}

type rolloutRequest struct {
	CanaryPercent int `json:"canaryPercent"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	stableURL := mustURL(env.String("STABLE_URL", "http://app-stable:8080"))
	canaryURL := mustURL(env.String("CANARY_URL", "http://app-canary:8080"))

	r := &router{
		stableURL:     stableURL,
		canaryURL:     canaryURL,
		client:        &http.Client{Timeout: 5 * time.Second},
		canaryPercent: clampPercent(env.Int("CANARY_PERCENT", 1)),
		stableMetrics: metrics.NewWindow(4000),
		canaryMetrics: metrics.NewWindow(4000),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", r.health)
	mux.HandleFunc("GET /metrics", r.metrics)
	mux.HandleFunc("POST /rollout", r.rollout)
	mux.HandleFunc("POST /rollback", r.rollback)
	mux.HandleFunc("/", r.proxy)

	addr := env.String("ADDR", ":8080")
	if err := http.ListenAndServe(addr, mux); err != nil {
		panic(err)
	}
}

func (r *router) health(w http.ResponseWriter, req *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"canaryPercent": r.percent(),
	})
}

func (r *router) metrics(w http.ResponseWriter, req *http.Request) {
	httpx.JSON(w, http.StatusOK, r.snapshot())
}

func (r *router) rollout(w http.ResponseWriter, req *http.Request) {
	var input rolloutRequest
	if err := httpx.DecodeJSON(req, &input); err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	r.setPercent(input.CanaryPercent)
	httpx.JSON(w, http.StatusOK, r.snapshot())
}

func (r *router) rollback(w http.ResponseWriter, req *http.Request) {
	r.setPercent(0)
	httpx.JSON(w, http.StatusOK, r.snapshot())
}

func (r *router) proxy(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/checkout" && req.URL.Path != "/fraud-check" {
		http.NotFound(w, req)
		return
	}

	chosen := r.choose()
	targetURL := r.targetURL(chosen, req)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		httpx.JSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	proxyReq, err := http.NewRequestWithContext(req.Context(), req.Method, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		httpx.JSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	copyHeaders(proxyReq.Header, req.Header)
	if proxyReq.Header.Get("X-User-ID") == "" {
		proxyReq.Header.Set("X-User-ID", fmt.Sprintf("local-user-%d", mathrand.Int64()))
	}

	start := time.Now()
	res, err := r.client.Do(proxyReq)
	latency := time.Since(start)
	if err != nil {
		r.record(chosen, latency, true)
		log.Printf("target=%s status=502 latency_ms=%d canary_percent=%d path=%s", chosen, latency.Milliseconds(), r.percent(), req.URL.Path)
		httpx.JSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "target": chosen})
		return
	}
	defer res.Body.Close()

	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		r.record(chosen, latency, true)
		httpx.JSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "target": chosen})
		return
	}

	failed := res.StatusCode >= 500
	r.record(chosen, latency, failed)
	log.Printf("target=%s status=%d latency_ms=%d canary_percent=%d path=%s", chosen, res.StatusCode, latency.Milliseconds(), r.percent(), req.URL.Path)

	copyHeaders(w.Header(), res.Header)
	w.Header().Set("X-Progressive-Delivery-Target", string(chosen))
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(responseBody)
}

func (r *router) choose() target {
	percent := r.percent()
	if percent <= 0 {
		return stable
	}
	if percent >= 100 {
		return canary
	}
	if mathrand.IntN(100) < percent {
		return canary
	}
	return stable
}

func (r *router) targetURL(chosen target, req *http.Request) *url.URL {
	base := r.stableURL
	if chosen == canary {
		base = r.canaryURL
	}

	next := *base
	next.Path = singleJoiningSlash(base.Path, req.URL.Path)
	next.RawQuery = req.URL.RawQuery
	return &next
}

func (r *router) record(chosen target, latency time.Duration, failed bool) {
	if chosen == canary {
		r.canaryMetrics.Record(latency, failed)
		return
	}
	r.stableMetrics.Record(latency, failed)
}

func (r *router) snapshot() map[string]any {
	return map[string]any{
		"canaryPercent": r.percent(),
		"stable":        r.stableMetrics.Snapshot(),
		"canary":        r.canaryMetrics.Snapshot(),
	}
}

func (r *router) percent() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.canaryPercent
}

func (r *router) setPercent(percent int) {
	r.mu.Lock()
	r.canaryPercent = clampPercent(percent)
	r.mu.Unlock()
	log.Printf("rollout canary_percent=%d", clampPercent(percent))
}

func mustURL(value string) *url.URL {
	parsed, err := url.Parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	default:
		return a + b
	}
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
