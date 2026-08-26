package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type resourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type serverMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
}

func (m *Manager) endpoints(ctx context.Context) (string, string, error) {
	if m.cfg.TokenURL != "" {
		return m.cfg.AuthorizationURL, m.cfg.TokenURL, nil
	}
	resource, err := validResourceURL(m.cfg.ResourceURL)
	if err != nil {
		return "", "", err
	}
	metadataURL := m.cfg.ResourceMetadataURL
	if metadataURL == "" {
		metadataURL = protectedResourceMetadataURL(resource)
	}
	var resourceDocument resourceMetadata
	if err := m.getJSON(ctx, metadataURL, &resourceDocument); err != nil {
		return "", "", fmt.Errorf("discover OAuth protected resource metadata: %w", err)
	}
	if len(resourceDocument.AuthorizationServers) == 0 {
		return "", "", errors.New("OAuth protected resource metadata has no authorization_servers")
	}
	metadata, err := m.authorizationServerMetadata(ctx, resourceDocument.AuthorizationServers[0])
	if err != nil {
		return "", "", err
	}
	authorizationEndpoint := firstNonEmpty(m.cfg.AuthorizationURL, metadata.AuthorizationEndpoint)
	tokenEndpoint := firstNonEmpty(m.cfg.TokenURL, metadata.TokenEndpoint)
	if tokenEndpoint == "" {
		return "", "", errors.New("OAuth metadata has no token_endpoint")
	}
	return authorizationEndpoint, tokenEndpoint, nil
}

func validResourceURL(value string) (*url.URL, error) {
	resource, err := url.Parse(value)
	if err != nil || resource.Scheme == "" || resource.Host == "" || resource.Fragment != "" {
		return nil, errors.New("NOISE_MCP_URL must be an absolute URL without a fragment")
	}
	return resource, nil
}

func protectedResourceMetadataURL(resource *url.URL) string {
	result := resource.Scheme + "://" + resource.Host + "/.well-known/oauth-protected-resource"
	if path := strings.Trim(resource.EscapedPath(), "/"); path != "" {
		result += "/" + path
	}
	return result
}

func (m *Manager) authorizationServerMetadata(ctx context.Context, issuer string) (serverMetadata, error) {
	metadataURL, err := authorizationMetadataURL(issuer)
	if err != nil {
		return serverMetadata{}, err
	}
	var metadata serverMetadata
	if err := m.getJSON(ctx, metadataURL, &metadata); err != nil {
		return serverMetadata{}, fmt.Errorf("discover OAuth authorization server metadata: %w", err)
	}
	return metadata, nil
}

func authorizationMetadataURL(issuer string) (string, error) {
	u, err := url.Parse(issuer)
	if err != nil || u.Scheme == "" || u.Host == "" || u.Fragment != "" {
		return "", errors.New("OAuth authorization server URL is invalid")
	}
	path := strings.Trim(u.EscapedPath(), "/")
	u.Path = "/.well-known/oauth-authorization-server"
	if path != "" {
		u.Path += "/" + path
	}
	u.RawPath = ""
	u.RawQuery = ""
	return u.String(), nil
}

func (m *Manager) getJSON(ctx context.Context, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	return nil
}

func firstNonEmpty(first, second string) string {
	if first != "" {
		return first
	}
	return second
}
