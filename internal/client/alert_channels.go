package client

import (
	"context"
	"net/http"
	"net/url"
)

// Channel types and alert events the API accepts. They mirror the API's
// shared types (NOTIFICATION_CHANNEL_TYPES, ALERT_EVENTS and
// DEFAULT_ALERT_EVENTS); update them together when the API adds one.
var (
	AlertChannelTypes = []string{"slack", "teams", "webhook"}

	AlertEvents = []string{
		"cert.issued",
		"cert.renewed",
		"cert.failed",
		"cert.expiring",
		"cert.revoked",
		"cert.replacement_requested",
		"domain.verification_failed",
		"endpoint.scan_failed",
	}

	// DefaultAlertEvents is what the API subscribes a new channel to when the
	// create request has no events.
	DefaultAlertEvents = []string{
		"cert.failed",
		"cert.expiring",
		"cert.revoked",
		"cert.replacement_requested",
		"domain.verification_failed",
		"endpoint.scan_failed",
	}
)

// AlertChannel is a notification channel as the API returns it. The URL is
// never returned in full, only URLMasked (scheme, host and last 4 characters).
type AlertChannel struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Name      string   `json:"name"`
	URLMasked string   `json:"urlMasked"`
	Events    []string `json:"events"`
	Enabled   bool     `json:"enabled"`
	HasSecret bool     `json:"hasSecret"`
	// Secret is the webhook signing secret. Only the create response has it.
	Secret string `json:"secret,omitempty"`
}

type CreateAlertChannelRequest struct {
	Type    string   `json:"type"`
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
}

// UpdateAlertChannelRequest is a partial update: nil fields are left as they are.
type UpdateAlertChannelRequest struct {
	Name    *string   `json:"name,omitempty"`
	URL     *string   `json:"url,omitempty"`
	Events  *[]string `json:"events,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
}

func (c *Client) CreateAlertChannel(ctx context.Context, in CreateAlertChannelRequest) (*AlertChannel, error) {
	if in.Events == nil {
		in.Events = []string{}
	}
	var out AlertChannel
	if err := c.do(ctx, http.MethodPost, "/notifications/channels", in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetAlertChannel finds one channel. The API has no GET by ID, so this lists
// the account's channels (at most 10) and returns a 404 APIError when the ID
// is not among them.
func (c *Client) GetAlertChannel(ctx context.Context, id string) (*AlertChannel, error) {
	var all []AlertChannel
	if err := c.do(ctx, http.MethodGet, "/notifications/channels", nil, &all); err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Message: "Notification channel not found"}
}

func (c *Client) UpdateAlertChannel(ctx context.Context, id string, in UpdateAlertChannelRequest) (*AlertChannel, error) {
	var out AlertChannel
	if err := c.do(ctx, http.MethodPatch, "/notifications/channels/"+url.PathEscape(id), in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteAlertChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/notifications/channels/"+url.PathEscape(id), nil, nil)
}
