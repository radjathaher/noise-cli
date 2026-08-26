package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteToolPreviewsThenConfirms(t *testing.T) {
	t.Parallel()
	var confirms []bool
	server := newManagementMCPServer(t, true, func(arguments map[string]any) {
		confirm, ok := arguments["confirm"].(bool)
		if !ok {
			t.Errorf("confirm = %#v", arguments["confirm"])
		}
		confirms = append(confirms, confirm)
	})
	defer server.Close()
	output := &bytes.Buffer{}
	command := NewMCPCommand(testManagementOptions(server))
	command.SetOut(output)
	command.SetErr(output)
	command.SetArgs([]string{"call", "campaign_update", "--arguments", `{"campaign_id":12}`, "--apply"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(confirms) != 2 || confirms[0] || !confirms[1] {
		t.Fatalf("confirm sequence = %v, want [false true]", confirms)
	}
	if !strings.Contains(output.String(), `"preview"`) || !strings.Contains(output.String(), `"result"`) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteToolWithoutApplyOnlyCallsServerPreview(t *testing.T) {
	t.Parallel()
	var confirms []bool
	server := newManagementMCPServer(t, true, func(arguments map[string]any) {
		confirms = append(confirms, arguments["confirm"].(bool))
	})
	defer server.Close()
	output := &bytes.Buffer{}
	command := NewMCPCommand(testManagementOptions(server))
	command.SetOut(output)
	command.SetArgs([]string{"call", "campaign_update", "--arguments", `{}`})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(confirms) != 1 || confirms[0] {
		t.Fatalf("confirm sequence = %v, want [false]", confirms)
	}
	if !strings.Contains(output.String(), "server preview") {
		t.Fatalf("output = %q", output.String())
	}
}

func TestWriteToolWithoutConfirmSchemaFailsClosed(t *testing.T) {
	t.Parallel()
	calls := 0
	server := newManagementMCPServer(t, false, func(map[string]any) { calls++ })
	defer server.Close()
	command := NewMCPCommand(testManagementOptions(server))
	command.SetArgs([]string{"call", "campaign_update", "--arguments", `{}`, "--apply"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "no boolean confirm guard") {
		t.Fatalf("Execute() error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("tools/call requests = %d, want 0", calls)
	}
}

func testManagementOptions(server *httptest.Server) ManagementOptions {
	return ManagementOptions{
		MCPURL: server.URL, HTTPClient: server.Client(),
		Getenv: func(name string) string {
			if name == "NOISE_MCP_ACCESS_TOKEN" {
				return "access"
			}
			return ""
		},
	}
}

func newManagementMCPServer(t *testing.T, confirmSchema bool, onCall func(map[string]any)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var message struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(request.Body).Decode(&message); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if message.Method == "notifications/initialized" {
			writer.WriteHeader(http.StatusAccepted)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch message.Method {
		case "initialize":
			writeCommandRPCResult(writer, message.ID, map[string]any{
				"protocolVersion": "2025-11-25",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]string{"name": "test", "version": "1"},
			})
		case "tools/list":
			properties := map[string]any{}
			if confirmSchema {
				properties["confirm"] = map[string]string{"type": "boolean"}
			}
			writeCommandRPCResult(writer, message.ID, map[string]any{"tools": []map[string]any{{
				"name": "campaign_update", "inputSchema": map[string]any{"type": "object", "properties": properties},
			}}})
		case "tools/call":
			var params struct {
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(message.Params, &params)
			onCall(params.Arguments)
			text := "applied"
			if confirm, _ := params.Arguments["confirm"].(bool); !confirm {
				text = "server preview"
			}
			writeCommandRPCResult(writer, message.ID, map[string]any{
				"content": []map[string]string{{"type": "text", "text": text}},
			})
		}
	}))
}

func writeCommandRPCResult(writer http.ResponseWriter, id int64, result any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}
