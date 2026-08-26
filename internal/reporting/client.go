package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultBaseURL = "https://reporting.getnoise.com"
	defaultTimeout = 30 * time.Second
	defaultRetries = 3
	maxBodyBytes   = 32 << 20
)

type Config struct {
	BaseURL        string
	Org            string
	APIKey         string
	HTTPClient     *http.Client
	RequestTimeout time.Duration
	MaxRetries     int
}

type Client struct {
	baseURL        *url.URL
	org            string
	apiKey         string
	httpClient     *http.Client
	requestTimeout time.Duration
	maxRetries     int
	sleep          func(context.Context, time.Duration) error
}

type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("reporting API returned HTTP %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("reporting API returned HTTP %d: %s", e.StatusCode, e.Message)
}

func NewClient(config Config) (*Client, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid reporting base URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("reporting base URL must not contain a query or fragment")
	}

	timeout := config.RequestTimeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, fmt.Errorf("request timeout must be positive")
	}
	retries := config.MaxRetries
	if retries == 0 {
		retries = defaultRetries
	}
	if retries < 0 {
		return nil, fmt.Errorf("max retries must not be negative")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &Client{
		baseURL:        parsed,
		org:            strings.TrimSpace(config.Org),
		apiKey:         strings.TrimSpace(config.APIKey),
		httpClient:     httpClient,
		requestTimeout: timeout,
		maxRetries:     retries,
		sleep:          sleepContext,
	}, nil
}

func (c *Client) Health(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/api/health", nil, false)
}

func (c *Client) DailyReport(ctx context.Context, params DailyReportParams) (json.RawMessage, error) {
	query, err := params.Values()
	if err != nil {
		return nil, err
	}
	return c.get(ctx, "/v1/reports/daily", query, true)
}

func (c *Client) ListPosts(ctx context.Context, params PostsParams) (json.RawMessage, error) {
	query, err := params.Values()
	if err != nil {
		return nil, err
	}
	return c.get(ctx, "/v1/posts", query, true)
}

func (c *Client) ListCampaigns(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/v1/campaigns", nil, true)
}

func (c *Client) ListPlaybooks(ctx context.Context) (json.RawMessage, error) {
	return c.get(ctx, "/v1/playbooks", nil, true)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, authenticated bool) (json.RawMessage, error) {
	if authenticated && (c.org == "" || c.apiKey == "") {
		return nil, errors.New("NOISE_ORG and NOISE_API_KEY are required")
	}
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: joinURLPath(c.baseURL.Path, path)})
	requestURL.RawQuery = query.Encode()

	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	for attempt := 0; ; attempt++ {
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create reporting request: %w", err)
		}
		request.Header.Set("Accept", "application/json")
		if authenticated {
			request.Header.Set("x-noise-org", c.org)
			request.Header.Set("x-noise-token", c.apiKey)
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			return nil, fmt.Errorf("request reporting API: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
		closeErr := response.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read reporting API response: %w", readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close reporting API response: %w", closeErr)
		}
		if len(body) > maxBodyBytes {
			return nil, errors.New("reporting API response exceeds 32 MiB")
		}

		if (response.StatusCode == http.StatusTooManyRequests || response.StatusCode == http.StatusServiceUnavailable) && attempt < c.maxRetries {
			delay := retryDelay(response.Header.Get("Retry-After"), attempt, time.Now())
			if err := c.sleep(requestContext, delay); err != nil {
				return nil, fmt.Errorf("wait to retry reporting API: %w", err)
			}
			continue
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return nil, decodeAPIError(response.StatusCode, body)
		}
		if !json.Valid(body) {
			return nil, errors.New("reporting API returned invalid JSON")
		}
		return json.RawMessage(body), nil
	}
}

func joinURLPath(basePath, path string) string {
	return strings.TrimRight(basePath, "/") + "/" + strings.TrimLeft(path, "/")
}

func decodeAPIError(statusCode int, body []byte) error {
	payload := struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}{}
	message := strings.TrimSpace(string(body))
	if json.Unmarshal(body, &payload) == nil && payload.Error != "" {
		message = payload.Error
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	return &APIError{StatusCode: statusCode, Code: payload.Code, Message: message}
}

func retryDelay(value string, attempt int, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		if delay := retryAt.Sub(now); delay > 0 {
			return delay
		}
		return 0
	}
	delay := time.Second << min(attempt, 5)
	return delay
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
