package provider_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hi-tools/internal/llm/provider"
)

func TestParseModelsOK(t *testing.T) {
	body := `{"object":"list","data":[{"id":"zeta"},{"id":"alpha"},{"id":"mid"}]}`
	got, err := provider.ParseModels(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseModels: %v", err)
	}
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("ParseModels = %v, muốn %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ParseModels = %v, muốn %v (đã sort)", got, want)
		}
	}
}

func TestParseModelsMissingData(t *testing.T) {
	for _, body := range []string{`{}`, `{"object":"list"}`, `{"data":null}`} {
		if _, err := provider.ParseModels(strings.NewReader(body)); err == nil {
			t.Fatalf("ParseModels(%q) = nil error, muốn lỗi thiếu data", body)
		}
	}
}

func TestParseModelsMalformedJSON(t *testing.T) {
	if _, err := provider.ParseModels(strings.NewReader(`{"data":[`)); err == nil {
		t.Fatal("ParseModels JSON hỏng trả nil error")
	}
	if _, err := provider.ParseModels(strings.NewReader(`not json`)); err == nil {
		t.Fatal("ParseModels dữ liệu không JSON trả nil error")
	}
}

func TestListModelsOK(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":[{"id":"b"},{"id":"a"}]}`))
	}))
	defer srv.Close()

	client := provider.NewClient()
	got, err := client.ListModels(context.Background(), srv.URL+"/v1", "sk-test-key")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("path = %q, muốn /v1/models", gotPath)
	}
	if gotAuth != "Bearer sk-test-key" {
		t.Fatalf("Authorization = %q, muốn Bearer sk-test-key", gotAuth)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ListModels = %v, muốn [a b]", got)
	}
}

func TestListModelsHTTPErrorMentionsStatusNotKey(t *testing.T) {
	const key = "sk-SECRET-MUST-NOT-LEAK"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := provider.NewClient()
	_, err := client.ListModels(context.Background(), srv.URL+"/v1", key)
	if err == nil {
		t.Fatal("ListModels status 401 trả nil error")
	}

	var httpErr *provider.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("lỗi %v không phải *HTTPError", err)
	}
	if httpErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("StatusCode = %d, muốn 401", httpErr.StatusCode)
	}
	if !strings.Contains(httpErr.Error(), "401") {
		t.Fatalf("HTTPError.Error() = %q, muốn nêu 401", httpErr.Error())
	}
	if strings.Contains(err.Error(), key) {
		t.Fatalf("lỗi làm lộ api key: %q", err.Error())
	}
}

func TestListModelsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"nope":true}`))
	}))
	defer srv.Close()

	client := provider.NewClient()
	if _, err := client.ListModels(context.Background(), srv.URL+"/v1", "k"); err == nil {
		t.Fatal("ListModels response thiếu data trả nil error")
	}
}

func TestNewClientHasTimeout(t *testing.T) {
	client := provider.NewClient()
	if client.HTTP == nil {
		t.Fatal("NewClient().HTTP = nil")
	}
	if client.HTTP.Timeout == 0 {
		t.Fatal("client HTTP không có timeout (muốn 15s)")
	}
}

func TestHTTPErrorNoDuplicateCode(t *testing.T) {
	for _, e := range []*provider.HTTPError{
		{StatusCode: 401, Status: "401 Unauthorized"},
		{StatusCode: 401},
	} {
		if got := e.Error(); got != "HTTP 401 Unauthorized" {
			t.Fatalf("Error() = %q, muốn \"HTTP 401 Unauthorized\"", got)
		}
	}
}
