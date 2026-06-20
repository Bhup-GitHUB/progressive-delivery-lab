package main

import (
	"context"
	"log"
	"time"

	"progressive-delivery-lab/internal/env"
	"progressive-delivery-lab/internal/flags"
	"progressive-delivery-lab/internal/routerclient"
)

var stages = []int{1, 5, 25, 50, 100}

type controller struct {
	router       *routerclient.Client
	flags        *flags.AdminClient
	interval     time.Duration
	minCanaryReq int
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	c := &controller{
		router:       routerclient.New(env.String("ROUTER_URL", "http://router:8080")),
		flags:        flags.NewAdminClient(env.String("FEATURE_FLAG_URL", "http://flag-service:8080")),
		interval:     time.Duration(env.Int("CHECK_INTERVAL_SECONDS", 10)) * time.Second,
		minCanaryReq: env.Int("MIN_CANARY_REQUESTS", 5),
	}

	log.Printf("controller started stages=%v error_threshold=0.02 p99_threshold_ms=500", stages)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		c.check(context.Background())
		<-ticker.C
	}
}

func (c *controller) check(ctx context.Context) {
	data, err := c.router.Metrics(ctx)
	if err != nil {
		log.Printf("health_check status=error reason=%q", err.Error())
		return
	}

	if data.CanaryPercent == 0 {
		log.Printf("health_check canary_percent=0 state=rolled_back")
		return
	}

	if data.Canary.Requests < c.minCanaryReq {
		log.Printf("health_check canary_percent=%d canary_requests=%d state=waiting_for_samples", data.CanaryPercent, data.Canary.Requests)
		return
	}

	if unhealthy(data) {
		log.Printf("rollback triggered canary_percent=%d error_rate=%.4f p99_ms=%d", data.CanaryPercent, data.Canary.ErrorRate, data.Canary.P99MS)
		if err := c.router.Rollback(ctx); err != nil {
			log.Printf("rollback router_status=error reason=%q", err.Error())
		}
		if err := c.flags.KillFraudModel(ctx); err != nil {
			log.Printf("rollback kill_switch_status=error reason=%q", err.Error())
		}
		log.Printf("rollback complete canary_percent=0 fraud_model=disabled")
		return
	}

	next, ok := nextStage(data.CanaryPercent)
	if !ok {
		log.Printf("health_check canary_percent=%d state=complete error_rate=%.4f p99_ms=%d", data.CanaryPercent, data.Canary.ErrorRate, data.Canary.P99MS)
		return
	}

	if err := c.router.Rollout(ctx, next); err != nil {
		log.Printf("promotion status=error from=%d to=%d reason=%q", data.CanaryPercent, next, err.Error())
		return
	}
	if err := c.flags.SetFraudModel(ctx, flags.FraudModel{Enabled: true, RolloutPercent: next}); err != nil {
		log.Printf("promotion flag_status=error rollout_percent=%d reason=%q", next, err.Error())
		return
	}
	log.Printf("promotion complete from=%d to=%d error_rate=%.4f p99_ms=%d", data.CanaryPercent, next, data.Canary.ErrorRate, data.Canary.P99MS)
}

func unhealthy(data routerclient.Metrics) bool {
	return data.Canary.ErrorRate >= 0.02 || data.Canary.P99MS >= 500
}

func nextStage(current int) (int, bool) {
	for _, stage := range stages {
		if stage > current {
			return stage, true
		}
	}
	return current, false
}
