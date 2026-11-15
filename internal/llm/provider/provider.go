// Package provider gọi API OpenAI-compatible GET /models và cache kết quả.
package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client là HTTP client gọi provider.
type Client struct {
	HTTP    *http.Client
	Headers map[string]string
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
	if e.Status != "" { // resp.Status đã có mã: "401 Unauthorized"
		return "HTTP " + e.Status
	}
	return fmt.Sprintf("HTTP %d %s", e.StatusCode, http.StatusText(e.StatusCode))
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
	c.setHeaders(req)
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

// setHeaders gửi kèm các header riêng của provider (sau Authorization để không ghi đè key).
func (c *Client) setHeaders(req *http.Request) {
	if c == nil {
		return
	}
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}
}

// Chat gọi POST {baseURL}/chat/completions để kiểm model còn dùng được.
// Không đọc nội dung: model reasoning có thể trả content null.
func (c *Client) Chat(ctx context.Context, baseURL, apiKey, model string) error {
	payload, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 16,
	})
	if err != nil {
		return err
	}
	url := strings.TrimRight(baseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	c.setHeaders(req)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	return nil
}

// Classify xếp lỗi thành auth/quota/http/net/response (key do lệnh ping tự gán).
func Classify(err error) string {
	if err == nil {
		return ""
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "auth"
		case http.StatusPaymentRequired, http.StatusTooManyRequests:
			return "quota"
		default:
			return "http"
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "net"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return "net"
	}
	return "response"
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
