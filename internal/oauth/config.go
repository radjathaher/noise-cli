package oauth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

var ErrLoginRequired = errors.New("authentication required: run `noise auth login`")

type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}

func (t Token) Valid(now time.Time) bool {
	return t.AccessToken != "" && (t.ExpiresAt.IsZero() || now.Add(30*time.Second).Before(t.ExpiresAt))
}

type Status struct {
	Authenticated bool      `json:"authenticated"`
	Source        string    `json:"source,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Refreshable   bool      `json:"refreshable,omitempty"`
}

type Config struct {
	ResourceURL         string
	ResourceMetadataURL string
	AuthorizationURL    string
	TokenURL            string
	ClientID            string
	ClientSecret        string
	RefreshToken        string
	Scope               string
	TokenPath           string
	RedirectURL         string
	HTTPClient          *http.Client
	Getenv              func(string) string
	OpenBrowser         func(string) error
	Input               io.Reader
	Output              io.Writer
	Now                 func() time.Time
}

type Manager struct {
	cfg Config
}

func NewManager(cfg Config) *Manager {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.Getenv == nil {
		cfg.Getenv = os.Getenv
	}
	if cfg.OpenBrowser == nil {
		cfg.OpenBrowser = openBrowser
	}
	if cfg.Input == nil {
		cfg.Input = os.Stdin
	}
	if cfg.Output == nil {
		cfg.Output = os.Stdout
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Manager{cfg: cfg}
}

func (m *Manager) Status() Status {
	if m.cfg.Getenv("NOISE_MCP_ACCESS_TOKEN") != "" {
		return Status{Authenticated: true, Source: "environment"}
	}
	if m.cfg.ClientID != "" && m.cfg.ClientSecret != "" {
		return Status{Authenticated: true, Source: "client_credentials"}
	}
	if m.refreshToken() != "" {
		return Status{Authenticated: true, Source: "refresh_token", Refreshable: true}
	}
	token, err := m.load()
	if err != nil || token.AccessToken == "" {
		return Status{}
	}
	return Status{
		Authenticated: token.Valid(m.cfg.Now()) || token.RefreshToken != "",
		Source:        "token_file",
		ExpiresAt:     token.ExpiresAt,
		Refreshable:   token.RefreshToken != "",
	}
}

// AccessToken never starts an interactive flow. Precedence is direct access
// token, complete client credentials, explicit refresh token, then token file.
func (m *Manager) AccessToken(ctx context.Context) (string, error) {
	if token := m.cfg.Getenv("NOISE_MCP_ACCESS_TOKEN"); token != "" {
		return token, nil
	}
	if m.cfg.ClientID != "" && m.cfg.ClientSecret != "" {
		return m.accessWithClientCredentials(ctx)
	}
	if refreshToken := m.refreshToken(); refreshToken != "" {
		return m.accessWithRefreshToken(ctx, refreshToken, false)
	}
	token, err := m.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if token.Valid(m.cfg.Now()) {
		return token.AccessToken, nil
	}
	if token.RefreshToken == "" {
		return "", ErrLoginRequired
	}
	return m.accessWithRefreshToken(ctx, token.RefreshToken, true)
}

func (m *Manager) accessWithClientCredentials(ctx context.Context) (string, error) {
	token, err := m.clientCredentials(ctx)
	if err != nil {
		return "", err
	}
	return token.AccessToken, nil
}

func (m *Manager) accessWithRefreshToken(ctx context.Context, refreshToken string, save bool) (string, error) {
	token, err := m.refresh(ctx, refreshToken)
	if err != nil {
		return "", err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}
	if save {
		if err := m.save(token); err != nil {
			return "", err
		}
	}
	return token.AccessToken, nil
}

func (m *Manager) refreshToken() string {
	if token := m.cfg.Getenv("NOISE_MCP_REFRESH_TOKEN"); token != "" {
		return token
	}
	return m.cfg.RefreshToken
}

func (m *Manager) Login(ctx context.Context, manual bool) (Token, error) {
	if m.cfg.ClientID == "" {
		return Token{}, errors.New("NOISE_MCP_CLIENT_ID is required for login; dynamic client registration is not supported")
	}
	if m.cfg.ClientSecret != "" {
		token, err := m.clientCredentials(ctx)
		if err != nil {
			return Token{}, err
		}
		return token, m.save(token)
	}
	return m.loginPKCE(ctx, manual)
}

func (m *Manager) Logout() error {
	path, err := m.tokenPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
