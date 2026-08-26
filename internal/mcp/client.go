package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
)

type TokenProvider func(context.Context) (string, error)

type Config struct {
	Endpoint      string
	HTTPClient    *http.Client
	TokenProvider TokenProvider
	ClientName    string
	ClientVersion string
}

type Client struct {
	cfg Config

	mu              sync.Mutex
	nextID          int64
	sessionID       string
	protocolVersion string
	capabilities    map[string]json.RawMessage
}

func NewClient(cfg Config) (*Client, error) {
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("NOISE_MCP_URL must be an absolute URL")
	}
	if endpoint.Scheme != "https" && endpoint.Hostname() != "127.0.0.1" && endpoint.Hostname() != "localhost" {
		return nil, errors.New("MCP endpoint must use HTTPS except on localhost")
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "noise-cli"
	}
	if cfg.ClientVersion == "" {
		cfg.ClientVersion = "dev"
	}
	return &Client{cfg: cfg, nextID: 1}, nil
}

func (c *Client) Initialize(ctx context.Context) (InitializeResult, error) {
	c.mu.Lock()
	c.sessionID = ""
	c.protocolVersion = ProtocolVersion20251125
	c.capabilities = nil
	c.mu.Unlock()

	params := map[string]any{
		"protocolVersion": ProtocolVersion20251125,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    c.cfg.ClientName,
			"version": c.cfg.ClientVersion,
		},
	}
	var result InitializeResult
	if err := c.call(ctx, "initialize", params, &result); err != nil {
		return InitializeResult{}, err
	}
	if !supportedVersion(result.ProtocolVersion) {
		return InitializeResult{}, fmt.Errorf("MCP server selected unsupported protocol version %q", result.ProtocolVersion)
	}
	c.mu.Lock()
	c.protocolVersion = result.ProtocolVersion
	c.capabilities = result.Capabilities
	c.mu.Unlock()
	if err := c.notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return InitializeResult{}, fmt.Errorf("send MCP initialized notification: %w", err)
	}
	return result, nil
}

func (c *Client) ListTools(ctx context.Context) (ListToolsResult, error) {
	if err := c.requireCapability("tools"); err != nil {
		return ListToolsResult{}, err
	}
	var all []Tool
	cursor := ""
	for {
		params := map[string]string{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page ListToolsResult
		if err := c.call(ctx, "tools/list", params, &page); err != nil {
			return ListToolsResult{}, err
		}
		all = append(all, page.Tools...)
		if page.NextCursor == "" {
			return ListToolsResult{Tools: all}, nil
		}
		cursor = page.NextCursor
	}
}

func (c *Client) DescribeTool(ctx context.Context, name string) (Tool, error) {
	tools, err := c.ListTools(ctx)
	if err != nil {
		return Tool{}, err
	}
	for _, tool := range tools.Tools {
		if tool.Name == name {
			return tool, nil
		}
	}
	return Tool{}, fmt.Errorf("MCP tool %q is not available", name)
}

// CallTool performs exactly one tools/call request. It intentionally does not
// retry because a disconnected write can have an ambiguous outcome.
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallToolResult, error) {
	if err := c.requireCapability("tools"); err != nil {
		return CallToolResult{}, err
	}
	var result CallToolResult
	err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result)
	return result, err
}

func (c *Client) requireCapability(name string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.capabilities == nil {
		return errors.New("MCP client is not initialized")
	}
	if _, ok := c.capabilities[name]; !ok {
		return fmt.Errorf("MCP server did not declare the %q capability", name)
	}
	return nil
}

func supportedVersion(version string) bool {
	for _, supported := range SupportedProtocolVersions {
		if supported == version {
			return true
		}
	}
	return false
}
