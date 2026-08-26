package reporting

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuthenticatedRequestsPreserveJSONAndHeaders(t *testing.T) {
	t.Parallel()
	want := `{"data":[{"id":9007199254740993}],"future":{"value":true}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/campaigns" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("x-noise-org") != "acme" {
			t.Errorf("org header = %q", request.Header.Get("x-noise-org"))
		}
		if request.Header.Get("x-noise-token") != "secret-token" {
			t.Errorf("token header = %q", request.Header.Get("x-noise-token"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(want))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "acme", "secret-token")
	got, err := client.ListCampaigns(context.Background())
	if err != nil {
		t.Fatalf("ListCampaigns() error = %v", err)
	}
	if string(got) != want {
		t.Fatalf("response changed:\nwant %s\n got %s", want, got)
	}
}

func TestHealthDoesNotSendCredentials(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("x-noise-org") != "" || request.Header.Get("x-noise-token") != "" {
			t.Error("health request sent credentials")
		}
		_, _ = writer.Write([]byte(`{"status":"healthy"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "acme", "secret-token")
	if _, err := client.Health(context.Background()); err != nil {
		t.Fatalf("Health() error = %v", err)
	}
}

func TestMissingCredentialsFailBeforeRequest(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "", "")
	_, err := client.ListPlaybooks(context.Background())
	if err == nil || !strings.Contains(err.Error(), "NOISE_ORG and NOISE_API_KEY") {
		t.Fatalf("ListPlaybooks() error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("server received %d requests", requests.Load())
	}
}

func TestRetries429And503(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requestNumber := requests.Add(1)
		writer.Header().Set("Retry-After", "0")
		switch requestNumber {
		case 1:
			writer.WriteHeader(http.StatusTooManyRequests)
		case 2:
			writer.WriteHeader(http.StatusServiceUnavailable)
		default:
			_, _ = writer.Write([]byte(`{"data":[]}`))
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "acme", "secret-token")
	payload, err := client.ListCampaigns(context.Background())
	if err != nil {
		t.Fatalf("ListCampaigns() error = %v", err)
	}
	if string(payload) != `{"data":[]}` || requests.Load() != 3 {
		t.Fatalf("payload = %s, requests = %d", payload, requests.Load())
	}
}

func TestRequestTimeout(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	client, err := NewClient(Config{
		BaseURL: server.URL, Org: "acme", APIKey: "secret-token",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.ListCampaigns(context.Background())
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ListCampaigns() error = %v", err)
	}
}

func TestAPIErrorIsStructuredAndDoesNotExposeCredentials(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"error":"invalid credentials","code":"forbidden"}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "private-org", "secret-token")
	_, err := client.ListCampaigns(context.Background())
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusForbidden || apiError.Code != "forbidden" {
		t.Fatalf("error = %#v", err)
	}
	if strings.Contains(err.Error(), "private-org") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("error exposed credentials: %v", err)
	}
}

func TestRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL, "acme", "secret-token")
	if _, err := client.ListCampaigns(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("ListCampaigns() error = %v", err)
	}
}

func newTestClient(t *testing.T, baseURL, org, token string) *Client {
	t.Helper()
	client, err := NewClient(Config{BaseURL: baseURL, Org: org, APIKey: token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}
