package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnvironmentAccessTokenHasFirstPrecedence(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()
	manager := NewManager(Config{
		ResourceURL: server.URL, AuthorizationURL: server.URL, TokenURL: server.URL,
		ClientID: "client", ClientSecret: "secret", HTTPClient: server.Client(),
		Getenv: func(name string) string {
			if name == "NOISE_MCP_ACCESS_TOKEN" {
				return "direct-token"
			}
			return ""
		},
	})
	token, err := manager.AccessToken(context.Background())
	if err != nil || token != "direct-token" {
		t.Fatalf("AccessToken() = %q, %v", token, err)
	}
	if requests != 0 {
		t.Fatalf("token endpoint requests = %d, want 0", requests)
	}
}

func TestEnvironmentRefreshTokenWorksHeadlessly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
		}
		if request.Form.Get("grant_type") != "refresh_token" || request.Form.Get("refresh_token") != "refresh-value" {
			t.Errorf("form = %v", request.Form)
		}
		if request.Form.Get("resource") != "https://reporting.getnoise.com/mcp" {
			t.Errorf("resource = %q", request.Form.Get("resource"))
		}
		_, _ = writer.Write([]byte(`{"access_token":"refreshed","expires_in":3600}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		ResourceURL:      "https://reporting.getnoise.com/mcp",
		AuthorizationURL: server.URL, TokenURL: server.URL, HTTPClient: server.Client(),
		RefreshToken: "refresh-value", Now: func() time.Time { return time.Unix(100, 0) },
	})
	token, err := manager.AccessToken(context.Background())
	if err != nil || token != "refreshed" {
		t.Fatalf("AccessToken() = %q, %v", token, err)
	}
}

func TestClientCredentialsUsesBasicAuthentication(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		clientID, secret, ok := request.BasicAuth()
		if !ok || clientID != "client" || secret != "secret" {
			t.Errorf("BasicAuth() = %q, %q, %v", clientID, secret, ok)
		}
		if err := request.ParseForm(); err != nil || request.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("form = %v, error = %v", request.Form, err)
		}
		_, _ = writer.Write([]byte(`{"access_token":"machine-token"}`))
	}))
	defer server.Close()
	manager := NewManager(Config{
		ResourceURL:      "https://reporting.getnoise.com/mcp",
		AuthorizationURL: server.URL, TokenURL: server.URL, HTTPClient: server.Client(),
		ClientID: "client", ClientSecret: "secret", Getenv: func(string) string { return "" },
	})
	token, err := manager.AccessToken(context.Background())
	if err != nil || token != "machine-token" {
		t.Fatalf("AccessToken() = %q, %v", token, err)
	}
}

func TestLoginStoresTokenWithOwnerOnlyPermissions(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "saved-token", "refresh_token": "saved-refresh"})
	}))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	manager := NewManager(Config{
		ResourceURL:      "https://reporting.getnoise.com/mcp",
		AuthorizationURL: server.URL, TokenURL: server.URL, HTTPClient: server.Client(),
		ClientID: "client", ClientSecret: "secret", TokenPath: path,
	})
	if _, err := manager.Login(context.Background(), false); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o600 {
		t.Fatalf("token permissions = %o, want 600", permissions)
	}
	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(data) {
		t.Fatalf("token file invalid: %s, %v", data, err)
	}
}
