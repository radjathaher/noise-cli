package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	ProtocolVersion20251125 = "2025-11-25"
	ProtocolVersion20250618 = "2025-06-18"
)

var SupportedProtocolVersions = []string{ProtocolVersion20251125, ProtocolVersion20250618}

type Tool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	Annotations  ToolAnnotations `json:"annotations,omitempty"`
}

type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

func (tool Tool) ProvenReadOnly() bool {
	return tool.Annotations.ReadOnlyHint != nil && *tool.Annotations.ReadOnlyHint
}

func (tool Tool) HasBooleanConfirm() bool {
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	if json.Unmarshal(tool.InputSchema, &schema) != nil {
		return false
	}
	confirm, ok := schema.Properties["confirm"]
	return ok && confirm.Type == "boolean"
}

func ParseArguments(value string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var arguments map[string]any
	if err := decoder.Decode(&arguments); err != nil {
		return nil, fmt.Errorf("--arguments must be one JSON object: %w", err)
	}
	if arguments == nil {
		return nil, errors.New("--arguments must be a JSON object, not null")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("--arguments must contain exactly one JSON object")
	}
	return arguments, nil
}

type ListToolsResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type CallToolResult struct {
	Content           []json.RawMessage `json:"content,omitempty"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string                     `json:"protocolVersion"`
	Capabilities    map[string]json.RawMessage `json:"capabilities"`
	ServerInfo      ServerInfo                 `json:"serverInfo"`
	Instructions    string                     `json:"instructions,omitempty"`
}

type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return e.Message
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
