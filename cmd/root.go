package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/radjathaher/noise-cli/internal/apperror"
	"github.com/radjathaher/noise-cli/internal/oauth"
	"github.com/radjathaher/noise-cli/internal/output"
	"github.com/radjathaher/noise-cli/internal/reporting"
	commandruntime "github.com/radjathaher/noise-cli/internal/runtime"
	"github.com/spf13/cobra"
)

const (
	defaultBaseURL = "https://reporting.getnoise.com"
	defaultMCPURL  = "https://reporting.getnoise.com/mcp"
)

func Execute() int {
	root := NewRootCommand()
	err := root.ExecuteContext(context.Background())
	if err == nil {
		return 0
	}

	exitCode, errorCode := classifyError(err)

	_ = output.WriteJSON(root.ErrOrStderr(), map[string]any{
		"error": map[string]any{
			"code":    errorCode,
			"message": err.Error(),
		},
	}, false)

	return exitCode
}

func NewRootCommand() *cobra.Command {
	options := &commandruntime.Options{}
	command := &cobra.Command{
		Use:           "noise",
		Short:         "Inspect and manage Noise campaigns",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       Version,
		PersistentPreRunE: func(command *cobra.Command, _ []string) error {
			if options.Timeout <= 0 {
				return apperror.New("usage_error", 2, "--timeout must be greater than zero")
			}
			command.SetContext(commandruntime.WithOptions(command.Context(), options))
			if options.Debug {
				_ = output.WriteJSON(command.ErrOrStderr(), map[string]any{"debug": map[string]string{
					"command":       command.CommandPath(),
					"reporting_url": redactedURL(options.BaseURL),
					"mcp_url":       redactedURL(options.MCPURL),
				}}, false)
			}
			return nil
		},
	}

	flags := command.PersistentFlags()
	flags.StringVar(&options.BaseURL, "base-url", envOrDefault("NOISE_BASE_URL", defaultBaseURL), "Reporting API base URL")
	flags.StringVar(&options.MCPURL, "mcp-url", envOrDefault("NOISE_MCP_URL", defaultMCPURL), "MCP endpoint URL")
	flags.DurationVar(&options.Timeout, "timeout", 30*time.Second, "request timeout")
	flags.BoolVar(&options.Pretty, "pretty", false, "indent JSON output")
	flags.BoolVar(&options.Debug, "debug", false, "write redacted debug details to stderr")
	command.SetVersionTemplate(fmt.Sprintf("noise %s\n", Version))
	command.AddCommand(ReportingCommands()...)
	managementOptions := ManagementOptions{
		MCPURLValue: func() string { return options.MCPURL },
	}
	command.AddCommand(NewManagementCommands(managementOptions)...)

	return command
}

func classifyError(err error) (int, string) {
	var appError *apperror.Error
	if errors.As(err, &appError) {
		return appError.Exit, appError.Code
	}
	var apiError *reporting.APIError
	if errors.As(err, &apiError) {
		return classifyHTTPStatus(apiError.StatusCode)
	}
	if errors.Is(err, oauth.ErrLoginRequired) {
		return 3, "authentication_error"
	}
	var networkError net.Error
	if errors.As(err, &networkError) || errors.Is(err, context.DeadlineExceeded) {
		return 8, "upstream_error"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "required"), strings.Contains(message, "invalid argument"),
		strings.Contains(message, "unknown command"), strings.Contains(message, "unknown flag"):
		return 2, "usage_error"
	case strings.Contains(message, "oauth"), strings.Contains(message, "access token"),
		strings.Contains(message, "http 401"), strings.Contains(message, "http 403"):
		return 3, "authentication_error"
	case strings.Contains(message, "not available"), strings.Contains(message, "capability"):
		return 4, "not_found"
	case strings.Contains(message, "rate limit"), strings.Contains(message, "http 429"):
		return 7, "rate_limited"
	case strings.Contains(message, "mcp http"), strings.Contains(message, "request failed"):
		return 8, "upstream_error"
	default:
		return 1, "internal_error"
	}
}

func classifyHTTPStatus(status int) (int, string) {
	switch status {
	case 400, 422:
		return 6, "validation_error"
	case 401, 403:
		return 3, "authentication_error"
	case 404:
		return 4, "not_found"
	case 409:
		return 5, "conflict"
	case 429:
		return 7, "rate_limited"
	default:
		return 8, "upstream_error"
	}
}

func redactedURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "invalid"
	}
	return parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()
}

func envOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
