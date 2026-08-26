package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInitializeSessionAndListTools(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var message struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Header.Get("Authorization") != "Bearer access" {
			t.Errorf("Authorization = %q", request.Header.Get("Authorization"))
		}
		switch message.Method {
		case "initialize":
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("Mcp-Session-Id", "session-123")
			writeRPCResult(writer, message.ID, map[string]any{
				"protocolVersion": ProtocolVersion20251125,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "test", "version": "1"},
			})
		case "notifications/initialized":
			assertSessionHeaders(t, request)
			writer.WriteHeader(http.StatusAccepted)
		case "tools/list":
			assertSessionHeaders(t, request)
			writer.Header().Set("Content-Type", "application/json")
			writeRPCResult(writer, message.ID, map[string]any{
				"tools": []map[string]any{{"name": "campaign_get", "inputSchema": map[string]any{"type": "object"}}},
			})
		default:
			t.Errorf("unexpected method %q", message.Method)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server)
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	result, err := client.ListTools(context.Background())
	if err != nil || len(result.Tools) != 1 || result.Tools[0].Name != "campaign_get" {
		t.Fatalf("ListTools() = %#v, %v", result, err)
	}
}

func TestToolsFailClosedWhenCapabilityMissing(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var message struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(request.Body).Decode(&message)
		if message.Method == "notifications/initialized" {
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		writeRPCResult(writer, message.ID, map[string]any{
			"protocolVersion": ProtocolVersion20251125,
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]string{"name": "test", "version": "1"},
		})
	}))
	defer server.Close()
	client := newTestClient(t, server)
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	_, err := client.ListTools(context.Background())
	if err == nil || !strings.Contains(err.Error(), "tools") {
		t.Fatalf("ListTools() error = %v", err)
	}
}

func TestCallToolDoesNotRetryTransportFailure(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var message struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(request.Body).Decode(&message)
		switch message.Method {
		case "initialize":
			writer.Header().Set("Content-Type", "application/json")
			writeRPCResult(writer, message.ID, map[string]any{
				"protocolVersion": ProtocolVersion20251125,
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "test", "version": "1"},
			})
		case "notifications/initialized":
			writer.WriteHeader(http.StatusAccepted)
		case "tools/call":
			calls.Add(1)
			http.Error(writer, "uncertain outcome", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	client := newTestClient(t, server)
	if _, err := client.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := client.CallTool(context.Background(), "write", map[string]any{}); err == nil {
		t.Fatal("CallTool() error = nil")
	}
	if calls.Load() != 1 {
		t.Fatalf("tools/call requests = %d, want 1", calls.Load())
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := NewClient(Config{
		Endpoint: server.URL, HTTPClient: server.Client(),
		TokenProvider: func(context.Context) (string, error) { return "access", nil },
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func writeRPCResult(writer http.ResponseWriter, id int64, result any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func assertSessionHeaders(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Header.Get("Mcp-Session-Id") != "session-123" {
		t.Errorf("Mcp-Session-Id = %q", request.Header.Get("Mcp-Session-Id"))
	}
	if request.Header.Get("MCP-Protocol-Version") != ProtocolVersion20251125 {
		t.Errorf("MCP-Protocol-Version = %q", request.Header.Get("MCP-Protocol-Version"))
	}
}
