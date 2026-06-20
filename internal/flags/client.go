package flags

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"time"
)

type FraudModel struct {
	Enabled        bool `json:"enabled"`
	RolloutPercent int  `json:"rolloutPercent"`
}

type Response struct {
	FraudModel FraudModel `json:"fraudModel"`
}

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 2 * time.Second},
	}
}

func (c *Client) Flags(ctx context.Context) (Response, error) {
	var flags Response
	if c.baseURL == "" {
		return flags, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/flags", nil)
	if err != nil {
		return flags, err
	}

	res, err := c.http.Do(req)
	if err != nil {
		return flags, err
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return flags, fmt.Errorf("flag service returned %s", res.Status)
	}

	if err := json.NewDecoder(res.Body).Decode(&flags); err != nil {
		return flags, err
	}
	return flags, nil
}

func EnabledFor(flag FraudModel, key string) bool {
	if !flag.Enabled || flag.RolloutPercent <= 0 {
		return false
	}
	if flag.RolloutPercent >= 100 {
		return true
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(key))
	return int(hasher.Sum32()%100) < flag.RolloutPercent
}
