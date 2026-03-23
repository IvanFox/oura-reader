package oura

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// HTTPClientProvider returns an authenticated HTTP client for a user.
type HTTPClientProvider interface {
	HTTPClientForUser(ctx context.Context, userID int64) (*http.Client, error)
}

type Client struct {
	provider HTTPClientProvider
	limiter  *rate.Limiter
}

// listResponse is the generic paginated wrapper from Oura API.
type listResponse struct {
	Data      []json.RawMessage `json:"data"`
	NextToken *string           `json:"next_token"`
}

func NewClient(provider HTTPClientProvider) *Client {
	// 5000 requests per 300 seconds ≈ 16.67/s, burst of 10
	return &Client{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Every(300*time.Second/5000), 10),
	}
}

// Fetch retrieves all pages for an endpoint within the date range.
// Returns raw JSON objects for storage.
func (c *Client) Fetch(ctx context.Context, userID int64, spec EndpointSpec, startDate, endDate string) ([]json.RawMessage, error) {
	httpClient, err := c.provider.HTTPClientForUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	if !spec.IsList {
		raw, err := c.fetchSingle(ctx, httpClient, spec)
		if err != nil {
			return nil, err
		}
		return []json.RawMessage{raw}, nil
	}

	var all []json.RawMessage
	var nextToken string

	for {
		u := c.buildURL(spec, startDate, endDate, nextToken)

		if err := c.limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limiter: %w", err)
		}

		body, err := c.doWithRetry(ctx, httpClient, u)
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", spec.Name, err)
		}

		var resp listResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode %s response: %w", spec.Name, err)
		}

		all = append(all, resp.Data...)

		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		nextToken = *resp.NextToken
	}

	return all, nil
}

func (c *Client) fetchSingle(ctx context.Context, httpClient *http.Client, spec EndpointSpec) (json.RawMessage, error) {
	u := baseURL + spec.Path

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	body, err := c.doWithRetry(ctx, httpClient, u)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", spec.Name, err)
	}
	return body, nil
}

func (c *Client) buildURL(spec EndpointSpec, startDate, endDate, nextToken string) string {
	u := baseURL + spec.Path
	params := url.Values{}
	if spec.HasDates {
		if startDate != "" {
			params.Set("start_date", startDate)
		}
		if endDate != "" {
			params.Set("end_date", endDate)
		}
	}
	if nextToken != "" {
		params.Set("next_token", nextToken)
	}
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

func (c *Client) doWithRetry(ctx context.Context, httpClient *http.Client, url string) ([]byte, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := range maxRetries {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			backoff(attempt)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read body: %w", err)
			backoff(attempt)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil

		case resp.StatusCode == http.StatusTooManyRequests:
			retryAfter := resp.Header.Get("Retry-After")
			wait := 60 * time.Second
			if seconds, err := strconv.Atoi(retryAfter); err == nil {
				wait = time.Duration(seconds) * time.Second
			}
			slog.Warn("rate limited by Oura API", "retry_after", wait, "attempt", attempt+1)
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			lastErr = fmt.Errorf("rate limited (429)")
			continue

		case resp.StatusCode >= 500:
			lastErr = fmt.Errorf("server error %d: %s", resp.StatusCode, truncate(string(body), 200))
			slog.Warn("Oura API server error", "status", resp.StatusCode, "attempt", attempt+1)
			backoff(attempt)
			continue

		default:
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func backoff(attempt int) {
	d := time.Duration(1<<uint(attempt)) * time.Second
	time.Sleep(d)
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

// ExtractField extracts a string field from raw JSON.
func ExtractField(raw json.RawMessage, field string) string {
	if field == "" {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return strings.Trim(string(v), `"`)
	}
	return s
}

// ExtractDay extracts the day portion from a field value.
// For "timestamp" fields, truncates to date (YYYY-MM-DD).
func ExtractDay(raw json.RawMessage, spec EndpointSpec) string {
	val := ExtractField(raw, spec.DayField)
	if val == "" {
		return ""
	}
	// If the field is "timestamp", extract just the date part.
	if spec.DayField == "timestamp" && len(val) >= 10 {
		return val[:10]
	}
	return val
}
