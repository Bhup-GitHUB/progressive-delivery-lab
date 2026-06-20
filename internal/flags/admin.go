package flags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type AdminClient struct {
	baseURL string
	http    *http.Client
}

func NewAdminClient(baseURL string) *AdminClient {
	return &AdminClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 3 * time.Second},
	}
}

func (c *AdminClient) SetFraudModel(ctx context.Context, flag FraudModel) error {
	body, err := json.Marshal(flag)
	if err != nil {
		return err
	}
	return c.post(ctx, "/flags/fraud-model", body)
}

func (c *AdminClient) KillFraudModel(ctx context.Context) error {
	return c.post(ctx, "/flags/fraud-model/kill", nil)
}

func (c *AdminClient) post(ctx context.Context, path string, body []byte) error {
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
		return fmt.Errorf("flag service returned %s", res.Status)
	}
	return nil
}
