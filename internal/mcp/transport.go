package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

func (c *Client) call(ctx context.Context, method string, params any, target any) error {
	id := c.requestID()
	request := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	response, err := c.post(ctx, request, false)
	if err != nil {
		return err
	}
	parsed, err := decodeRPCResponse(response.body, response.mediaType, id)
	if err != nil {
		return err
	}
	if parsed.Error != nil {
		return parsed.Error
	}
	if len(parsed.Result) == 0 {
		return errors.New("MCP response has no result")
	}
	if err := json.Unmarshal(parsed.Result, target); err != nil {
		return fmt.Errorf("decode MCP result: %w", err)
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	request := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	_, err := c.post(ctx, request, true)
	return err
}

type httpResponse struct {
	body      []byte
	mediaType string
}

func (c *Client) post(ctx context.Context, message rpcRequest, notification bool) (httpResponse, error) {
	body, err := json.Marshal(message)
	if err != nil {
		return httpResponse{}, err
	}
	req, err := c.newRequest(ctx, body)
	if err != nil {
		return httpResponse{}, err
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return httpResponse{}, fmt.Errorf("MCP request failed: %w", err)
	}
	defer resp.Body.Close()
	return c.readResponse(resp, notification)
}

func (c *Client) newRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	c.mu.Lock()
	sessionID := c.sessionID
	protocolVersion := c.protocolVersion
	c.mu.Unlock()
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	if protocolVersion != "" {
		req.Header.Set("MCP-Protocol-Version", protocolVersion)
	}
	if c.cfg.TokenProvider != nil {
		token, tokenErr := c.cfg.TokenProvider(ctx)
		if tokenErr != nil {
			return nil, tokenErr
		}
		if token == "" {
			return nil, errors.New("token provider returned an empty access token")
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

func (c *Client) readResponse(resp *http.Response, notification bool) (httpResponse, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return httpResponse{}, fmt.Errorf("read MCP response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpResponse{}, fmt.Errorf("MCP HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if notification {
		if resp.StatusCode != http.StatusAccepted || len(bytes.TrimSpace(data)) != 0 {
			return httpResponse{}, errors.New("MCP notification response must be HTTP 202 with an empty body")
		}
		return httpResponse{}, nil
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return httpResponse{}, fmt.Errorf("parse MCP Content-Type: %w", err)
	}
	if mediaType != "application/json" && mediaType != "text/event-stream" {
		return httpResponse{}, fmt.Errorf("unsupported MCP Content-Type %q", mediaType)
	}
	if assigned := resp.Header.Get("Mcp-Session-Id"); assigned != "" {
		if !visibleASCII(assigned) {
			return httpResponse{}, errors.New("MCP session ID contains invalid characters")
		}
		c.mu.Lock()
		c.sessionID = assigned
		c.mu.Unlock()
	}
	return httpResponse{body: data, mediaType: mediaType}, nil
}

func (c *Client) requestID() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

func decodeRPCResponse(data []byte, mediaType string, expectedID int64) (rpcResponse, error) {
	if mediaType == "application/json" {
		return parseRPCResponse(data, expectedID)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16<<20)
	var eventData strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventData.Len() > 0 {
				response, err := parseRPCResponse([]byte(strings.TrimSuffix(eventData.String(), "\n")), expectedID)
				if err == nil {
					return response, nil
				}
				eventData.Reset()
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			eventData.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return rpcResponse{}, fmt.Errorf("read MCP SSE response: %w", err)
	}
	if eventData.Len() > 0 {
		return parseRPCResponse([]byte(strings.TrimSuffix(eventData.String(), "\n")), expectedID)
	}
	return rpcResponse{}, errors.New("MCP SSE stream ended without the requested response")
}

func parseRPCResponse(data []byte, expectedID int64) (rpcResponse, error) {
	var response rpcResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return rpcResponse{}, fmt.Errorf("decode MCP JSON-RPC response: %w", err)
	}
	if response.JSONRPC != "2.0" {
		return rpcResponse{}, errors.New("MCP response has invalid jsonrpc version")
	}
	var id int64
	if err := json.Unmarshal(response.ID, &id); err != nil || id != expectedID {
		return rpcResponse{}, fmt.Errorf("MCP response ID does not match request %s", strconv.FormatInt(expectedID, 10))
	}
	return response, nil
}

func visibleASCII(value string) bool {
	for _, character := range []byte(value) {
		if character < 0x21 || character > 0x7e {
			return false
		}
	}
	return true
}
