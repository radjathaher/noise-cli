package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/radjathaher/noise-cli/internal/reporting"
	commandruntime "github.com/radjathaher/noise-cli/internal/runtime"
	"github.com/spf13/cobra"
)

func TestReportingCommandRoutesAndOutputsRawJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		args      []string
		wantPath  string
		wantQuery map[string]string
	}{
		{name: "health", args: []string{"health"}, wantPath: "/api/health"},
		{name: "daily", args: []string{"report", "daily", "--start-date", "2026-01-01", "--end-date", "2026-01-02", "--group-by", "campaign", "--campaign-id", "12"}, wantPath: "/v1/reports/daily", wantQuery: map[string]string{"start_date": "2026-01-01", "end_date": "2026-01-02", "group_by": "campaign", "campaign_id": "12"}},
		{name: "posts", args: []string{"post", "list", "--start-date", "2026-01-01", "--end-date", "2026-01-31", "--page", "2", "--page-size", "50", "--campaign-id", "12,15"}, wantPath: "/v1/posts", wantQuery: map[string]string{"page": "2", "page_size": "50", "campaign_id": "12,15"}},
		{name: "campaigns", args: []string{"campaign", "list"}, wantPath: "/v1/campaigns"},
		{name: "playbooks", args: []string{"playbook", "list"}, wantPath: "/v1/playbooks"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.wantPath {
					t.Errorf("path = %q, want %q", request.URL.Path, test.wantPath)
				}
				for key, want := range test.wantQuery {
					if got := request.URL.Query().Get(key); got != want {
						t.Errorf("query %s = %q, want %q", key, got, want)
					}
				}
				_, _ = writer.Write([]byte(`{"number":9007199254740993,"future":true}`))
			}))
			defer server.Close()

			client, err := reporting.NewClient(reporting.Config{BaseURL: server.URL, Org: "acme", APIKey: "token"})
			if err != nil {
				t.Fatalf("NewClient() error = %v", err)
			}
			output := &bytes.Buffer{}
			root := &cobra.Command{Use: "noise"}
			root.AddCommand(ReportingCommandsWithClient(client)...)
			command := root
			command.SetOut(output)
			command.SetErr(output)
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if output.String() != "{\"number\":9007199254740993,\"future\":true}\n" {
				t.Fatalf("output = %q", output.String())
			}
		})
	}
}

func TestReportingCommandValidatesBeforeRequest(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := reporting.NewClient(reporting.Config{BaseURL: server.URL, Org: "acme", APIKey: "token"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	command := NewReportingCommandWithClient(client)
	command.SetArgs([]string{"post", "list", "--start-date", "2026-01-01", "--end-date", "2026-01-31", "--page-size", "101"})
	err = command.Execute()
	if err == nil || !strings.Contains(err.Error(), "page-size") {
		t.Fatalf("Execute() error = %v", err)
	}
	if requests != 0 {
		t.Fatalf("server received %d requests", requests)
	}
}

func TestReportingCommandFitsRootIntegration(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "noise"}
	root.AddCommand(ReportingCommands()...)
	command, _, err := root.Find([]string{"campaign", "list"})
	if err != nil || command.CommandPath() != "noise campaign list" {
		t.Fatalf("Find() command = %v, error = %v", command, err)
	}
}

func TestAddReportingCommandsMergesExistingGroups(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "noise"}
	campaign := &cobra.Command{Use: "campaign"}
	campaign.AddCommand(&cobra.Command{Use: "create"})
	playbook := &cobra.Command{Use: "playbook"}
	playbook.AddCommand(&cobra.Command{Use: "archive"})
	root.AddCommand(campaign, playbook)

	if err := AddReportingCommands(root); err != nil {
		t.Fatalf("AddReportingCommands() error = %v", err)
	}
	for _, path := range [][]string{{"campaign", "create"}, {"campaign", "list"}, {"playbook", "archive"}, {"playbook", "list"}} {
		if command, _, err := root.Find(path); err != nil || command.Name() != path[1] {
			t.Fatalf("Find(%v) command = %v, error = %v", path, command, err)
		}
	}
}

func TestReportingCommandUsesRuntimeBaseURLAndPrettyOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/health" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"status":"healthy","count":9007199254740993}`))
	}))
	defer server.Close()

	output := &bytes.Buffer{}
	command := NewReportingCommand()
	command.SetContext(commandruntime.WithOptions(context.Background(), &commandruntime.Options{
		BaseURL: server.URL, Timeout: time.Second, Pretty: true,
	}))
	command.SetOut(output)
	command.SetArgs([]string{"health"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "{\n  \"status\": \"healthy\",\n  \"count\": 9007199254740993\n}\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}
