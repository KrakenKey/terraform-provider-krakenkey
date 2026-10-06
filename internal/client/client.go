// Package client is a small client for the parts of the KrakenKey API the
// provider uses.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultAPIURL = "https://api.krakenkey.io"

// Certificate statuses returned by the API.
const (
	StatusPending  = "pending"
	StatusIssuing  = "issuing"
	StatusIssued   = "issued"
	StatusFailed   = "failed"
	StatusRenewing = "renewing"
	StatusRevoking = "revoking"
	StatusRevoked  = "revoked"
)

type Client struct {
	baseURL   string
	apiKey    string
	userAgent string
	http      *http.Client
}

func New(baseURL, apiKey, userAgent string) *Client {
	if baseURL == "" {
		baseURL = DefaultAPIURL
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		apiKey:    apiKey,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// APIError is a non-2xx response. Message comes from the API's standard error body.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("KrakenKey API returned %d: %s", e.StatusCode, e.Message)
}

// IsNotFound reports whether err is a 404 from the API.
func IsNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

type Certificate struct {
	ID            int        `json:"id"`
	Status        string     `json:"status"`
	RawCSR        string     `json:"rawCsr"`
	CrtPEM        *string    `json:"crtPem"`
	ChainPEM      *string    `json:"chainPem"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	AutoRenew     bool       `json:"autoRenew"`
	RenewalCount  int        `json:"renewalCount"`
	FailureReason *string    `json:"failureReason"`
}

type Chain struct {
	ChainPEM     string `json:"chainPem"`
	FullChainPEM string `json:"fullChainPem"`
}

type createResponse struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

// CreateCertificate submits a CSR. The API returns the existing certificate
// for a repeat of the same CSR within 15 minutes, so retrying is safe.
func (c *Client) CreateCertificate(ctx context.Context, csrPEM string) (int, error) {
	var out createResponse
	if err := c.do(ctx, http.MethodPost, "/certs/tls", map[string]string{"csrPem": csrPEM}, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

func (c *Client) GetCertificate(ctx context.Context, id int) (*Certificate, error) {
	var out Certificate
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/certs/tls/%d", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetChain(ctx context.Context, id int) (*Chain, error) {
	var out Chain
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/certs/tls/%d/chain", id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) SetAutoRenew(ctx context.Context, id int, autoRenew bool) error {
	return c.do(ctx, http.MethodPatch, fmt.Sprintf("/certs/tls/%d", id), map[string]bool{"autoRenew": autoRenew}, nil)
}

func (c *Client) RevokeCertificate(ctx context.Context, id int) error {
	return c.do(ctx, http.MethodPost, fmt.Sprintf("/certs/tls/%d/revoke", id), map[string]any{}, nil)
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &APIError{StatusCode: resp.StatusCode, Message: errorMessage(data, resp.Status)}
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

// errorMessage pulls "message" out of the API error body. Validation errors
// return it as an array of strings.
func errorMessage(data []byte, fallback string) string {
	var body struct {
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(data, &body) != nil || len(body.Message) == 0 {
		return fallback
	}
	var one string
	if json.Unmarshal(body.Message, &one) == nil {
		return one
	}
	var many []string
	if json.Unmarshal(body.Message, &many) == nil {
		return strings.Join(many, "; ")
	}
	return fallback
}
