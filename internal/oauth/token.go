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
	"time"
)

func (m *Manager) clientCredentials(ctx context.Context) (Token, error) {
	_, tokenURL, err := m.endpoints(ctx)
	if err != nil {
		return Token{}, err
	}
	values := url.Values{"grant_type": {"client_credentials"}, "resource": {m.cfg.ResourceURL}}
	if m.cfg.Scope != "" {
		values.Set("scope", m.cfg.Scope)
	}
	return m.exchange(ctx, tokenURL, values, true)
}

func (m *Manager) refresh(ctx context.Context, refreshToken string) (Token, error) {
	_, tokenURL, err := m.endpoints(ctx)
	if err != nil {
		return Token{}, err
	}
	values := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"resource":      {m.cfg.ResourceURL},
	}
	if m.cfg.ClientID != "" {
		values.Set("client_id", m.cfg.ClientID)
	}
	return m.exchange(ctx, tokenURL, values, m.cfg.ClientSecret != "")
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func (m *Manager) exchange(ctx context.Context, endpoint string, values url.Values, basicAuth bool) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicAuth {
		req.SetBasicAuth(m.cfg.ClientID, m.cfg.ClientSecret)
	}
	resp, err := m.cfg.HTTPClient.Do(req)
	if err != nil {
		return Token{}, fmt.Errorf("request OAuth token: %w", err)
	}
	defer resp.Body.Close()
	var result tokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return Token{}, fmt.Errorf("decode OAuth token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || result.Error != "" {
		return Token{}, tokenResponseError(resp.StatusCode, result)
	}
	if result.AccessToken == "" {
		return Token{}, errors.New("OAuth token response has no access_token")
	}
	return m.toToken(result), nil
}

func tokenResponseError(statusCode int, result tokenResponse) error {
	message := strings.TrimSpace(result.Error + " " + result.Description)
	if message == "" {
		message = fmt.Sprintf("HTTP %d", statusCode)
	}
	return fmt.Errorf("OAuth token request failed: %s", message)
}

func (m *Manager) toToken(result tokenResponse) Token {
	token := Token{
		AccessToken:  result.AccessToken,
		TokenType:    result.TokenType,
		RefreshToken: result.RefreshToken,
		Scope:        result.Scope,
	}
	if result.ExpiresIn > 0 {
		token.ExpiresAt = m.cfg.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	}
	return token
}
