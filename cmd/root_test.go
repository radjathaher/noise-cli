package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/radjathaher/noise-cli/internal/reporting"
)

func TestRootVersion(t *testing.T) {
	command := NewRootCommand()
	stdout := &bytes.Buffer{}
	command.SetOut(stdout)
	command.SetArgs([]string{"--version"})

	if err := command.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if got, want := stdout.String(), "noise dev\n"; got != want {
		t.Fatalf("version output = %q, want %q", got, want)
	}
}

func TestClassifyReportingErrors(t *testing.T) {
	tests := []struct {
		status int
		exit   int
		code   string
	}{
		{status: 401, exit: 3, code: "authentication_error"},
		{status: 404, exit: 4, code: "not_found"},
		{status: 409, exit: 5, code: "conflict"},
		{status: 422, exit: 6, code: "validation_error"},
		{status: 429, exit: 7, code: "rate_limited"},
		{status: 503, exit: 8, code: "upstream_error"},
	}
	for _, test := range tests {
		exit, code := classifyError(&reporting.APIError{StatusCode: test.status})
		if exit != test.exit || code != test.code {
			t.Errorf("HTTP %d classified as (%d, %q), want (%d, %q)", test.status, exit, code, test.exit, test.code)
		}
	}
}

func TestClassifyUsageError(t *testing.T) {
	exit, code := classifyError(errors.New("unknown flag: --wat"))
	if exit != 2 || code != "usage_error" {
		t.Fatalf("classification = (%d, %q)", exit, code)
	}
}

func TestClassifyMissingCredentials(t *testing.T) {
	exit, code := classifyError(errors.New("NOISE_ORG and NOISE_API_KEY are required"))
	if exit != 3 || code != "authentication_error" {
		t.Fatalf("classification = (%d, %q)", exit, code)
	}
}

func TestRedactedURLDropsCredentialsAndQuery(t *testing.T) {
	got := redactedURL("https://user:secret@example.com/mcp?token=secret")
	if got != "https://example.com/mcp" {
		t.Fatalf("redacted URL = %q", got)
	}
}
