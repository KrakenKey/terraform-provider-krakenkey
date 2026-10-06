package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type HostedRegion struct {
	Region string `json:"region"`
}

type Endpoint struct {
	ID            string         `json:"id"`
	Host          string         `json:"host"`
	Port          int            `json:"port"`
	SNI           *string        `json:"sni"`
	Label         *string        `json:"label"`
	IsActive      bool           `json:"isActive"`
	CreatedAt     time.Time      `json:"createdAt"`
	HostedRegions []HostedRegion `json:"hostedRegions"`
}

// HasRegion reports whether the endpoint is scanned from the hosted region.
func (e *Endpoint) HasRegion(region string) bool {
	for _, r := range e.HostedRegions {
		if r.Region == region {
			return true
		}
	}
	return false
}

// RegionNames returns the endpoint's hosted regions in the order the API gave them.
func (e *Endpoint) RegionNames() []string {
	names := make([]string, 0, len(e.HostedRegions))
	for _, r := range e.HostedRegions {
		names = append(names, r.Region)
	}
	return names
}

// ListEndpoints returns every endpoint the key can see.
func (c *Client) ListEndpoints(ctx context.Context) ([]Endpoint, error) {
	var out []Endpoint
	if err := c.do(ctx, http.MethodGet, "/endpoints", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateEndpoint registers host:port. POST /endpoints is an upsert: when the
// account already has that pair the API updates label and sni on the existing
// endpoint and returns it. Callers that must not adopt an existing endpoint
// have to check with ListEndpoints first.
func (c *Client) CreateEndpoint(ctx context.Context, host string, port int, sni, label *string) (*Endpoint, error) {
	body := map[string]any{"host": host, "port": port}
	if sni != nil {
		body["sni"] = *sni
	}
	if label != nil {
		body["label"] = *label
	}
	var out Endpoint
	if err := c.do(ctx, http.MethodPost, "/endpoints", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) GetEndpoint(ctx context.Context, id string) (*Endpoint, error) {
	var out Endpoint
	if err := c.do(ctx, http.MethodGet, "/endpoints/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateEndpoint sends a partial update. Accepted keys are sni, label and
// isActive; a nil value clears sni or label. host and port cannot be changed.
func (c *Client) UpdateEndpoint(ctx context.Context, id string, fields map[string]any) (*Endpoint, error) {
	var out Endpoint
	if err := c.do(ctx, http.MethodPatch, "/endpoints/"+url.PathEscape(id), fields, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteEndpoint(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/endpoints/"+url.PathEscape(id), nil, nil)
}

// AddEndpointRegion adds a hosted probe region. Adding one that is already
// there succeeds and changes nothing.
func (c *Client) AddEndpointRegion(ctx context.Context, id, region string) error {
	return c.do(ctx, http.MethodPost, "/endpoints/"+url.PathEscape(id)+"/regions", map[string]string{"region": region}, nil)
}

func (c *Client) RemoveEndpointRegion(ctx context.Context, id, region string) error {
	return c.do(ctx, http.MethodDelete, "/endpoints/"+url.PathEscape(id)+"/regions/"+url.PathEscape(region), nil, nil)
}

// addErrorDetails appends the limit, current count and plan from a plan limit
// error body (403 with code "plan_limit_exceeded") to the message.
func addErrorDetails(apiErr *APIError, data []byte) {
	var body struct {
		Code    string `json:"code"`
		Limit   *int   `json:"limit"`
		Current *int   `json:"current"`
		Plan    string `json:"plan"`
	}
	if json.Unmarshal(data, &body) != nil || body.Code != "plan_limit_exceeded" {
		return
	}
	var parts []string
	if body.Limit != nil {
		parts = append(parts, fmt.Sprintf("limit %d", *body.Limit))
	}
	if body.Current != nil {
		parts = append(parts, fmt.Sprintf("in use %d", *body.Current))
	}
	if body.Plan != "" {
		parts = append(parts, "plan "+body.Plan)
	}
	if len(parts) == 0 {
		return
	}
	apiErr.Message += " (" + strings.Join(parts, ", ") + ")"
}
