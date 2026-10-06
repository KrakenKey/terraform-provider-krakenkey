package client

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

type Domain struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	// VerificationCode is the full TXT value, krakenkey-site-verification=<hex>.
	VerificationCode string    `json:"verificationCode"`
	IsVerified       bool      `json:"isVerified"`
	CreatedAt        time.Time `json:"createdAt"`
}

// CreateDomain registers a hostname. If the account already has it, the API
// returns the existing domain instead of creating a new one.
func (c *Client) CreateDomain(ctx context.Context, hostname string) (*Domain, error) {
	var out Domain
	if err := c.do(ctx, http.MethodPost, "/domains", map[string]string{"hostname": hostname}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListDomains returns every domain the key can see, including those of other
// members of the user's organization.
func (c *Client) ListDomains(ctx context.Context) ([]Domain, error) {
	var out []Domain
	if err := c.do(ctx, http.MethodGet, "/domains", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetDomain(ctx context.Context, id string) (*Domain, error) {
	var out Domain
	if err := c.do(ctx, http.MethodGet, "/domains/"+url.PathEscape(id), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// VerifyDomain asks KrakenKey to look up the TXT record. It returns a 400 when
// the record is missing or the lookup fails, and counts against the hourly
// rate limit for expensive operations even when the domain is already verified.
func (c *Client) VerifyDomain(ctx context.Context, id string) (*Domain, error) {
	var out Domain
	if err := c.do(ctx, http.MethodPost, "/domains/"+url.PathEscape(id)+"/verify", map[string]any{}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteDomain(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/domains/"+url.PathEscape(id), nil, nil)
}
