package oauth

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type pkceRequest struct {
	authorizationURL string
	tokenURL         string
	redirectURL      string
	verifier         string
	state            string
	listener         net.Listener
}

func (m *Manager) loginPKCE(ctx context.Context, manual bool) (Token, error) {
	request, err := m.preparePKCE(ctx, manual)
	if err != nil {
		return Token{}, err
	}
	if request.listener != nil {
		defer request.listener.Close()
	}
	code, err := m.obtainCode(ctx, request, manual)
	if err != nil {
		return Token{}, err
	}
	values := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {request.redirectURL},
		"client_id":     {m.cfg.ClientID},
		"code_verifier": {request.verifier},
		"resource":      {m.cfg.ResourceURL},
	}
	token, err := m.exchange(ctx, request.tokenURL, values, false)
	if err != nil {
		return Token{}, err
	}
	return token, m.save(token)
}

func (m *Manager) preparePKCE(ctx context.Context, manual bool) (pkceRequest, error) {
	authorizationURL, tokenURL, err := m.endpoints(ctx)
	if err != nil {
		return pkceRequest{}, err
	}
	if authorizationURL == "" {
		return pkceRequest{}, errors.New("OAuth metadata has no authorization_endpoint")
	}
	verifier, err := randomURLSafe(32)
	if err != nil {
		return pkceRequest{}, err
	}
	state, err := randomURLSafe(24)
	if err != nil {
		return pkceRequest{}, err
	}
	redirectURL, listener, err := m.redirect(manual)
	if err != nil {
		return pkceRequest{}, err
	}
	authURL, err := m.buildAuthorizationURL(authorizationURL, redirectURL, verifier, state)
	if err != nil {
		if listener != nil {
			listener.Close()
		}
		return pkceRequest{}, err
	}
	return pkceRequest{authURL, tokenURL, redirectURL, verifier, state, listener}, nil
}

func (m *Manager) redirect(manual bool) (string, net.Listener, error) {
	if m.cfg.RedirectURL != "" {
		return m.cfg.RedirectURL, nil, nil
	}
	if manual {
		return "http://127.0.0.1/callback", nil, nil
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, fmt.Errorf("start OAuth callback listener: %w", err)
	}
	return "http://" + listener.Addr().String() + "/callback", listener, nil
}

func (m *Manager) buildAuthorizationURL(endpoint, redirectURL, verifier, state string) (string, error) {
	hash := sha256.Sum256([]byte(verifier))
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {m.cfg.ClientID},
		"redirect_uri":          {redirectURL},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(hash[:])},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"resource":              {m.cfg.ResourceURL},
	}
	if m.cfg.Scope != "" {
		query.Set("scope", m.cfg.Scope)
	}
	authURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorization endpoint: %w", err)
	}
	authURL.RawQuery = query.Encode()
	return authURL.String(), nil
}

func (m *Manager) obtainCode(ctx context.Context, request pkceRequest, manual bool) (string, error) {
	if manual {
		fmt.Fprintf(m.cfg.Output, "Open this URL:\n%s\nPaste the authorization code or callback URL: ", request.authorizationURL)
		line, err := bufio.NewReader(m.cfg.Input).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		return codeFromManualInput(strings.TrimSpace(line), request.state)
	}
	if request.listener == nil {
		return "", errors.New("browser login needs a loopback redirect; use --manual with NOISE_MCP_REDIRECT_URL")
	}
	if err := m.cfg.OpenBrowser(request.authorizationURL); err != nil {
		return "", fmt.Errorf("open browser (retry with --manual): %w", err)
	}
	return waitForCallback(ctx, request.listener, request.state)
}

func waitForCallback(ctx context.Context, listener net.Listener, expectedState string) (string, error) {
	result := make(chan callbackResult, 1)
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := validateCallback(r.URL.Query(), expectedState)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			_, _ = io.WriteString(w, "Noise authentication complete. You can close this tab.\n")
		}
		result <- callbackResult{code: code, err: err}
	})
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case value := <-result:
		return value.code, value.err
	}
}

type callbackResult struct {
	code string
	err  error
}

func validateCallback(query url.Values, expectedState string) (string, error) {
	if query.Get("state") != expectedState {
		return "", errors.New("OAuth state mismatch")
	}
	if errorCode := query.Get("error"); errorCode != "" {
		return "", fmt.Errorf("OAuth authorization failed: %s", errorCode)
	}
	if code := query.Get("code"); code != "" {
		return code, nil
	}
	return "", errors.New("OAuth callback has no code")
}

func codeFromManualInput(input, expectedState string) (string, error) {
	if input == "" {
		return "", errors.New("authorization code is empty")
	}
	u, err := url.Parse(input)
	if err == nil && u.Scheme != "" {
		return validateCallback(u.Query(), expectedState)
	}
	return input, nil
}

func randomURLSafe(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
