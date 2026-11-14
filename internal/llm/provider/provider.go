// Package provider gọi API OpenAI-compatible GET /models và cache kết quả.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Client là HTTP client gọi provider.
type Client struct {
	HTTP *http.Client
}

// NewClient tạo client với timeout 15s.
func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// HTTPError là lỗi status khác 200; không chứa api key.
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	status := e.Status
	if status == "" {
		status = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d %s", e.StatusCode, status)
}

// ListModels gọi GET {baseURL}/models với Bearer token và trả danh sách id đã sort.
func (c *Client) ListModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return ParseModels(resp.Body)
}

func (c *Client) httpClient() *http.Client {
	if c == nil || c.HTTP == nil {
		return &http.Client{Timeout: 15 * time.Second}
	}
	return c.HTTP
}

// ParseModels đọc response chuẩn OpenAI {"data":[{"id":...}]} và sort id.
func ParseModels(r io.Reader) ([]string, error) {
	var payload struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return nil, fmt.Errorf("response /models không hợp lệ: %w", err)
	}
	if payload.Data == nil {
		return nil, fmt.Errorf("response /models thiếu trường data")
	}
	models := make([]string, 0, len(*payload.Data))
	for _, m := range *payload.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	sort.Strings(models)
	return models, nil
}
