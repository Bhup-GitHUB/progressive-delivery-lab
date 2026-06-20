package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type counters struct {
	total  atomic.Int64
	ok     atomic.Int64
	failed atomic.Int64
	canary atomic.Int64
}

func main() {
	target := flag.String("target", "http://router:8080", "router base URL")
	duration := flag.Duration("duration", 90*time.Second, "run duration")
	rps := flag.Int("rps", 20, "requests per second")
	mode := flag.String("mode", "normal", "normal or bad")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("loadgen started mode=%s target=%s duration=%s rps=%d", *mode, *target, duration.String(), *rps)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}
	counts := &counters{}
	interval := time.Second / time.Duration(max(1, *rps))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var wg sync.WaitGroup
	userIndex := int64(0)

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			log.Printf("loadgen complete total=%d ok=%d failed=%d canary=%d", counts.total.Load(), counts.ok.Load(), counts.failed.Load(), counts.canary.Load())
			return
		case <-ticker.C:
			id := atomic.AddInt64(&userIndex, 1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				send(client, *target, *mode, id, counts)
			}()
		}
	}
}

func send(client *http.Client, target, mode string, userID int64, counts *counters) {
	req, err := http.NewRequest(http.MethodGet, target+"/checkout", nil)
	if err != nil {
		counts.failed.Add(1)
		return
	}
	req.Header.Set("X-User-ID", fmt.Sprintf("%s-user-%d", mode, userID))

	res, err := client.Do(req)
	counts.total.Add(1)
	if err != nil {
		counts.failed.Add(1)
		return
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)

	if res.Header.Get("X-Progressive-Delivery-Target") == "canary" {
		counts.canary.Add(1)
	}
	if res.StatusCode >= 200 && res.StatusCode <= 499 {
		counts.ok.Add(1)
		return
	}
	counts.failed.Add(1)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
