// Package api implements the SubRouter HTTP client.
//
// Two credential surfaces exist and must never be mixed:
//
//	key (sk-...)            relay key  -> /v1/* model calls and billing reads
//	access_token + user_id  management  -> /api/* account administration
//
// Every /api/* response is wrapped in an envelope: {"success":bool,
// "message":string,"data":...}. Some business errors still arrive with
// HTTP 200, so both the status code and the envelope are checked.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to a SubRouter site.
type Client struct {
	BaseURL     string
	AccessToken string
	UserID      int
	APIKey      string
	HTTP        *http.Client
}

// New builds a client from resolved configuration.
func New(baseURL, accessToken string, userID int, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		BaseURL:     strings.TrimRight(baseURL, "/"),
		AccessToken: accessToken,
		UserID:      userID,
		APIKey:      apiKey,
		HTTP:        &http.Client{Timeout: timeout},
	}
}

// APIError describes a failed request.
type APIError struct {
	Status  int
	Path    string
	Message string
	Body    string
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = strings.TrimSpace(e.Body)
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Path, msg, e.Status)
}

// Envelope is the wrapper every /api endpoint returns.
type Envelope struct {
	Success *bool           `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Params is a query string builder that omits empty values.
type Params map[string]any

// Encode renders the params as a query string, skipping empty entries.
func (p Params) Encode() string {
	if len(p) == 0 {
		return ""
	}
	q := url.Values{}
	for k, v := range p {
		switch t := v.(type) {
		case nil:
			continue
		case string:
			if t == "" {
				continue
			}
			q.Set(k, t)
		case bool:
			q.Set(k, strconv.FormatBool(t))
		case int:
			if t == 0 {
				continue
			}
			q.Set(k, strconv.Itoa(t))
		case int64:
			if t == 0 {
				continue
			}
			q.Set(k, strconv.FormatInt(t, 10))
		case float64:
			if t == 0 {
				continue
			}
			q.Set(k, strconv.FormatFloat(t, 'f', -1, 64))
		case []string:
			if len(t) == 0 {
				continue
			}
			q.Set(k, strings.Join(t, ","))
		default:
			q.Set(k, fmt.Sprint(v))
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// AuthMode selects which credential a request carries.
type AuthMode int

const (
	// AuthManagement sends the access token plus the New-Api-User header.
	AuthManagement AuthMode = iota
	// AuthRelay sends the sk- relay key.
	AuthRelay
	// AuthPublic sends no credentials.
	AuthPublic
)

// Do performs a request and decodes the JSON body into out.
//
// When out is non-nil and the endpoint uses the /api envelope, out receives
// the envelope's data field. Use DoRaw to receive the whole envelope.
func (c *Client) Do(ctx context.Context, method, path string, params Params, body any, mode AuthMode, out any) error {
	raw, err := c.DoRaw(ctx, method, path, params, body, mode)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if len(raw) == 0 {
		return nil
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		// Not an envelope (for example a /v1/* response): decode directly.
		return json.Unmarshal(raw, out)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	return json.Unmarshal(env.Data, out)
}

// DoRaw performs a request and returns the raw response body.
func (c *Client) DoRaw(ctx context.Context, method, path string, params Params, body any, mode AuthMode) ([]byte, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request body: %w", err)
		}
		rdr = bytes.NewReader(b)
	}

	target := c.BaseURL + path
	if q := params.Encode(); q != "" {
		target += q
	}

	req, err := http.NewRequestWithContext(ctx, method, target, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	switch mode {
	case AuthManagement:
		if c.AccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+c.AccessToken)
		}
		if c.UserID > 0 {
			req.Header.Set("New-Api-User", strconv.Itoa(c.UserID))
		}
	case AuthRelay:
		if c.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.APIKey)
		}
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response %s: %w", path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &APIError{Status: resp.StatusCode, Path: path, Message: extractMessage(raw), Body: string(raw)}
	}

	// HTTP 200 can still carry a business error inside the envelope.
	var env Envelope
	if err := json.Unmarshal(raw, &env); err == nil {
		if env.Success != nil && !*env.Success {
			return nil, &APIError{Status: resp.StatusCode, Path: path, Message: extractMessage(raw), Body: string(raw)}
		}
		if env.Success == nil && env.Message == "error" {
			return nil, &APIError{Status: resp.StatusCode, Path: path, Message: extractMessage(raw), Body: string(raw)}
		}
	}
	return raw, nil
}

// extractMessage pulls a human readable reason out of an error body.
func extractMessage(raw []byte) string {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err == nil && env.Message != "" && env.Message != "error" {
		return env.Message
	}
	var generic struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &generic); err == nil {
		if generic.Error.Message != "" {
			return generic.Error.Message
		}
		if generic.Message != "" {
			return generic.Message
		}
	}
	return strings.TrimSpace(string(raw))
}

// Get issues an authenticated GET against a management endpoint.
func (c *Client) Get(ctx context.Context, path string, params Params, out any) error {
	return c.Do(ctx, http.MethodGet, path, params, nil, AuthManagement, out)
}

// Post issues an authenticated POST against a management endpoint.
func (c *Client) Post(ctx context.Context, path string, body any, out any) error {
	return c.Do(ctx, http.MethodPost, path, nil, body, AuthManagement, out)
}

// Put issues an authenticated PUT against a management endpoint.
func (c *Client) Put(ctx context.Context, path string, body any, out any) error {
	return c.Do(ctx, http.MethodPut, path, nil, body, AuthManagement, out)
}

// Delete issues an authenticated DELETE against a management endpoint.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodDelete, path, nil, nil, AuthManagement, out)
}

// GetPublic issues an unauthenticated GET.
func (c *Client) GetPublic(ctx context.Context, path string, params Params, out any) error {
	return c.Do(ctx, http.MethodGet, path, params, nil, AuthPublic, out)
}

// GetRelay issues a GET with the sk- relay key.
func (c *Client) GetRelay(ctx context.Context, path string, params Params, out any) error {
	return c.Do(ctx, http.MethodGet, path, params, nil, AuthRelay, out)
}

// List is the generic paginated list shape returned by list endpoints. Some
// endpoints return a bare array instead, so callers use DecodeList.
type List struct {
	Items    json.RawMessage `json:"items"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Total    int             `json:"total"`
	HasMore  bool            `json:"has_more"`
	NextCur  string          `json:"next_cursor"`
}

// DecodeList decodes either a bare JSON array or an object with an "items"
// array into a typed slice.
func DecodeList(raw json.RawMessage, out any) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '[' {
		return json.Unmarshal(trimmed, out)
	}
	var l List
	if err := json.Unmarshal(trimmed, &l); err != nil {
		return err
	}
	if len(l.Items) == 0 {
		return nil
	}
	return json.Unmarshal(l.Items, out)
}
