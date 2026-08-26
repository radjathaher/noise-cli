package cmd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/radjathaher/noise-cli/internal/apperror"
	"github.com/radjathaher/noise-cli/internal/mcp"
	"github.com/radjathaher/noise-cli/internal/oauth"
	commandruntime "github.com/radjathaher/noise-cli/internal/runtime"
	"github.com/spf13/cobra"
)

type ManagementOptions struct {
	Input       io.Reader
	Output      io.Writer
	HTTPClient  *http.Client
	Getenv      func(string) string
	OpenBrowser func(string) error
	MCPURL      string
	MCPURLValue func() string
}

const DefaultMCPURL = "https://reporting.getnoise.com/mcp"

func NewManagementCommands(options ManagementOptions) []*cobra.Command {
	options = normalizeManagementOptions(options)
	return []*cobra.Command{NewAuthCommand(options), NewMCPCommand(options)}
}

func NewAuthCommand(options ManagementOptions) *cobra.Command {
	options = normalizeManagementOptions(options)
	command := &cobra.Command{
		Use:   "auth",
		Short: "Manage Noise MCP authentication",
	}
	command.AddCommand(newAuthStatusCommand(options), newAuthLoginCommand(options), newAuthLogoutCommand(options))
	return command
}

func NewMCPCommand(options ManagementOptions) *cobra.Command {
	options = normalizeManagementOptions(options)
	command := &cobra.Command{
		Use:   "mcp",
		Short: "Inspect and call Noise MCP tools",
	}
	command.AddCommand(newMCPToolsCommand(options), newMCPDescribeCommand(options), newMCPCallCommand(options))
	return command
}

func newAuthStatusCommand(options ManagementOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the selected authentication source",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return writeCommandJSON(command, newOAuthManager(command, options).Status())
		},
	}
}

func newAuthLoginCommand(options ManagementOptions) *cobra.Command {
	var manual bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with client credentials or browser PKCE (client ID required)",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			token, err := newOAuthManager(command, options).Login(command.Context(), manual)
			if err != nil {
				return err
			}
			return writeCommandJSON(command, map[string]any{
				"authenticated": true,
				"expires_at":    token.ExpiresAt,
				"refreshable":   token.RefreshToken != "",
			})
		},
	}
	command.Flags().BoolVar(&manual, "manual", false, "print the URL and read a pasted code or callback URL (NOISE_MCP_CLIENT_ID required)")
	return command
}

func newAuthLogoutCommand(options ManagementOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the stored OAuth token",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := newOAuthManager(command, options).Logout(); err != nil {
				return err
			}
			return writeCommandJSON(command, map[string]bool{"logged_out": true})
		},
	}
}

func newMCPToolsCommand(options ManagementOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "tools",
		Short: "List tools declared by the MCP server",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := initializedClient(command.Context(), options)
			if err != nil {
				return err
			}
			result, err := client.ListTools(command.Context())
			if err != nil {
				return err
			}
			return writeCommandJSON(command, result)
		},
	}
}

func newMCPDescribeCommand(options ManagementOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <tool-name>",
		Short: "Show the exact schema declared for one MCP tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := initializedClient(command.Context(), options)
			if err != nil {
				return err
			}
			tool, err := client.DescribeTool(command.Context(), args[0])
			if err != nil {
				return err
			}
			return writeCommandJSON(command, tool)
		},
	}
}

func newMCPCallCommand(options ManagementOptions) *cobra.Command {
	var rawArguments string
	var apply bool
	command := &cobra.Command{
		Use:   "call <tool-name>",
		Short: "Preview or call one MCP tool",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			arguments, err := mcp.ParseArguments(rawArguments)
			if err != nil {
				return err
			}
			client, err := initializedClient(command.Context(), options)
			if err != nil {
				return err
			}
			tool, err := client.DescribeTool(command.Context(), args[0])
			if err != nil {
				return err
			}
			if tool.ProvenReadOnly() {
				result, callErr := client.CallTool(command.Context(), tool.Name, arguments)
				return writeToolResult(command, result, callErr)
			}
			if !tool.HasBooleanConfirm() {
				return apperror.New("unsafe_write", 9, "write-like MCP tool has no boolean confirm guard; refusing to call it")
			}
			previewArguments := cloneArguments(arguments)
			previewArguments["confirm"] = false
			preview, err := client.CallTool(command.Context(), tool.Name, previewArguments)
			if err != nil {
				return err
			}
			if !apply || preview.IsError {
				return writeToolResult(command, preview, nil)
			}
			applyArguments := cloneArguments(arguments)
			applyArguments["confirm"] = true
			result, err := client.CallTool(command.Context(), tool.Name, applyArguments)
			if err != nil {
				return &apperror.Error{
					Cause: err, Code: "ambiguous_write", Exit: 9,
					Message: "write result is ambiguous; inspect current state and audit log before retrying: " + err.Error(),
				}
			}
			return writeCommandJSON(command, map[string]any{"preview": preview, "result": result})
		},
	}
	command.Flags().StringVar(&rawArguments, "arguments", "{}", "tool arguments as one JSON object")
	command.Flags().BoolVar(&apply, "apply", false, "after server preview, confirm and execute a guarded write")
	return command
}

func initializedClient(ctx context.Context, options ManagementOptions) (*mcp.Client, error) {
	endpoint := managementMCPURL(ctx, options)
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: commandruntime.FromContext(ctx).Timeout}
	}
	manager := newOAuthManagerFromURL(options, endpoint)
	client, err := mcp.NewClient(mcp.Config{
		Endpoint:      endpoint,
		HTTPClient:    options.HTTPClient,
		TokenProvider: manager.AccessToken,
		ClientName:    "noise-cli",
		ClientVersion: Version,
	})
	if err != nil {
		return nil, err
	}
	if _, err := client.Initialize(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func newOAuthManager(command *cobra.Command, options ManagementOptions) *oauth.Manager {
	return newOAuthManagerFromURL(optionsForCommand(command, options), managementMCPURL(command.Context(), options))
}

func newOAuthManagerFromURL(options ManagementOptions, endpoint string) *oauth.Manager {
	return oauth.NewManager(oauth.Config{
		ResourceURL:         endpoint,
		ResourceMetadataURL: strings.TrimSpace(options.Getenv("NOISE_MCP_RESOURCE_METADATA_URL")),
		AuthorizationURL:    strings.TrimSpace(options.Getenv("NOISE_MCP_AUTHORIZATION_URL")),
		TokenURL:            strings.TrimSpace(options.Getenv("NOISE_MCP_TOKEN_URL")),
		ClientID:            options.Getenv("NOISE_MCP_CLIENT_ID"),
		ClientSecret:        options.Getenv("NOISE_MCP_CLIENT_SECRET"),
		RefreshToken:        options.Getenv("NOISE_MCP_REFRESH_TOKEN"),
		Scope:               options.Getenv("NOISE_MCP_SCOPE"),
		TokenPath:           options.Getenv("NOISE_MCP_TOKEN_FILE"),
		RedirectURL:         options.Getenv("NOISE_MCP_REDIRECT_URL"),
		HTTPClient:          options.HTTPClient,
		Getenv:              options.Getenv,
		OpenBrowser:         options.OpenBrowser,
		Input:               options.Input,
		Output:              options.Output,
	})
}

func managementMCPURL(ctx context.Context, options ManagementOptions) string {
	if endpoint := strings.TrimSpace(commandruntime.FromContext(ctx).MCPURL); endpoint != "" {
		return endpoint
	}
	if endpoint := strings.TrimSpace(options.Getenv("NOISE_MCP_URL")); endpoint != "" {
		return endpoint
	}
	if options.MCPURLValue != nil {
		if endpoint := strings.TrimSpace(options.MCPURLValue()); endpoint != "" {
			return endpoint
		}
	}
	if endpoint := strings.TrimSpace(options.MCPURL); endpoint != "" {
		return endpoint
	}
	return DefaultMCPURL
}

func optionsForCommand(command *cobra.Command, options ManagementOptions) ManagementOptions {
	options.Input = command.InOrStdin()
	options.Output = command.OutOrStdout()
	return options
}

func normalizeManagementOptions(options ManagementOptions) ManagementOptions {
	if options.Getenv == nil {
		options.Getenv = os.Getenv
	}
	return options
}

func writeToolResult(command *cobra.Command, result mcp.CallToolResult, callErr error) error {
	if callErr != nil {
		return callErr
	}
	if err := writeCommandJSON(command, result); err != nil {
		return err
	}
	if result.IsError {
		return apperror.New("validation_error", 6, "MCP tool returned isError=true")
	}
	return nil
}

func writeCommandJSON(command *cobra.Command, value any) error {
	pretty, _ := command.Flags().GetBool("pretty")
	return writeJSON(command.OutOrStdout(), value, pretty)
}

func writeJSON(output io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}
