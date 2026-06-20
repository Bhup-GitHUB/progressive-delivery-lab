package routerclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"progressive-delivery-lab/internal/metrics"
)

type Metrics struct {
	CanaryPercent int              `json:"canaryPercent"`
	Stable        metrics.Snapshot `json:"stable"`
	Canary        metrics.Snapshot `json:"canary"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *Client) Metrics(ctx context.Context) (Metrics, error) {
	var data Metrics
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/metrics", nil)
	if err != nil {
		return data, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return data, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return data, fmt.Errorf("router returned %s", res.Status)
	}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return data, err
	}
	return data, nil
}

func (c *Client) Rollout(ctx context.Context, percent int) error {
	body, err := json.Marshal(map[string]int{"canaryPercent": percent})
	if err != nil {
		return err
	}
	return c.post(ctx, "/rollout", body)
}

func (c *Client) Rollback(ctx context.Context) error {
	return c.post(ctx, "/rollback", nil)
}

func (c *Client) post(ctx context.Context, path string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("router returned %s", res.Status)
	}
	return nil
}
