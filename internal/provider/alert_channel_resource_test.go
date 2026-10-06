package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

const (
	channelRes      = "krakenkey_alert_channel.test"
	channelSlackURL = "https://hooks.slack.com/services/T000/B000/firstAAAA"
	channelSlackNew = "https://hooks.slack.com/services/T000/B000/secondBBBB"
)

// fakeChannelAPI is an in-memory stand-in for the notification channel endpoints.
type fakeChannelAPI struct {
	mu       sync.Mutex
	channels map[string]map[string]any
	order    []string
	urls     map[string]string
	nextID   int
	// patches records every PATCH body, so tests can check when a URL was sent.
	patches []map[string]any
	deleted []string
}

func newFakeChannelAPI(t *testing.T) (*fakeChannelAPI, *httptest.Server) {
	f := &fakeChannelAPI{channels: map[string]map[string]any{}, urls: map[string]string{}, nextID: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func channelMask(url string) string {
	host := strings.SplitN(strings.TrimPrefix(url, "https://"), "/", 2)[0]
	return "https://" + host + "/…" + url[len(url)-4:]
}

// channelCheckURL applies the API's syntax rules for Slack and Teams URLs.
func channelCheckURL(typ, url string) string {
	switch {
	case !strings.HasPrefix(url, "https://"):
		return "URL must use https"
	case typ == "slack" && !strings.HasPrefix(url, "https://hooks.slack.com/services/"):
		return "Slack URL must be an incoming webhook URL (https://hooks.slack.com/services/...)"
	}
	return ""
}

func (f *fakeChannelAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
		reply(401, map[string]any{"statusCode": 401, "message": "Unauthorized"})
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	if r.URL.Path == "/notifications/channels" {
		switch r.Method {
		case http.MethodGet:
			out := []map[string]any{}
			for _, id := range f.order {
				out = append(out, f.channels[id])
			}
			reply(200, out)
		case http.MethodPost:
			typ, url := body["type"].(string), body["url"].(string)
			if msg := channelCheckURL(typ, url); msg != "" {
				reply(400, map[string]any{"statusCode": 400, "message": msg})
				return
			}
			id := fmt.Sprintf("00000000-0000-4000-8000-%012d", f.nextID)
			f.nextID++
			events, ok := body["events"]
			if !ok {
				events = client.DefaultAlertEvents
			}
			enabled, ok := body["enabled"]
			if !ok {
				enabled = true
			}
			ch := map[string]any{
				"id": id, "type": typ, "name": body["name"], "urlMasked": channelMask(url),
				"events": events, "enabled": enabled, "hasSecret": typ == "webhook",
			}
			f.channels[id], f.urls[id] = ch, url
			f.order = append(f.order, id)
			out := map[string]any{}
			for k, v := range ch {
				out[k] = v
			}
			if typ == "webhook" {
				out["secret"] = "whsec_" + id[len(id)-4:]
			}
			reply(201, out)
		default:
			reply(405, map[string]any{"statusCode": 405, "message": "Method not allowed"})
		}
		return
	}

	id, ok := strings.CutPrefix(r.URL.Path, "/notifications/channels/")
	ch, found := f.channels[id]
	if !ok || !found {
		reply(404, map[string]any{"statusCode": 404, "message": "Notification channel not found"})
		return
	}
	switch r.Method {
	case http.MethodPatch:
		f.patches = append(f.patches, body)
		if url, ok := body["url"].(string); ok {
			if msg := channelCheckURL(ch["type"].(string), url); msg != "" {
				reply(400, map[string]any{"statusCode": 400, "message": msg})
				return
			}
			f.urls[id], ch["urlMasked"] = url, channelMask(url)
		}
		for _, k := range []string{"name", "events", "enabled"} {
			if v, ok := body[k]; ok {
				ch[k] = v
			}
		}
		reply(200, ch)
	case http.MethodDelete:
		delete(f.channels, id)
		f.order = slices.DeleteFunc(f.order, func(s string) bool { return s == id })
		f.deleted = append(f.deleted, id)
		w.WriteHeader(204)
	default:
		reply(405, map[string]any{"statusCode": 405, "message": "Method not allowed"})
	}
}

// channelURL returns the URL the fake holds for channel id.
func (f *fakeChannelAPI) channelURL(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.urls[id]
}

// patchURLs returns, for each PATCH so far, the URL it carried ("" for none).
func (f *fakeChannelAPI) patchURLs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, p := range f.patches {
		url, _ := p["url"].(string)
		out = append(out, url)
	}
	return out
}

func (f *fakeChannelAPI) deletedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.deleted...)
}

func (f *fakeChannelAPI) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.channels, id)
	f.order = slices.DeleteFunc(f.order, func(s string) bool { return s == id })
}

func channelConfig(url, body string) string {
	return fmt.Sprintf(`
provider "krakenkey" {
  api_url = %q
  api_key = %q
}

resource "krakenkey_alert_channel" "test" {
%s
}
`, url, testAPIKey, body)
}

func channelSlack(name, url string, version int, extra string) string {
	return fmt.Sprintf(`
  type           = "slack"
  name           = %q
  url_wo         = %q
  url_wo_version = %d
  %s`, name, url, version, extra)
}

const channelID1 = "00000000-0000-4000-8000-000000000001"

// channelNoURLInState fails if any attribute in state holds one of the URLs.
func channelNoURLInState(urls ...string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[channelRes]
		if !ok {
			return fmt.Errorf("%s not in state", channelRes)
		}
		for k, v := range rs.Primary.Attributes {
			for _, u := range urls {
				if strings.Contains(v, u) {
					return fmt.Errorf("state attribute %s holds the channel URL", k)
				}
			}
		}
		return nil
	}
}

func channelCheck(f func() error) resource.TestCheckFunc {
	return func(*terraform.State) error { return f() }
}

func TestAlertChannel_slack(t *testing.T) {
	api, srv := newFakeChannelAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: channelConfig(srv.URL, channelSlack("Ops", channelSlackURL, 1, "")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "id", channelID1),
					resource.TestCheckResourceAttr(channelRes, "type", "slack"),
					resource.TestCheckResourceAttr(channelRes, "name", "Ops"),
					resource.TestCheckResourceAttr(channelRes, "enabled", "true"),
					resource.TestCheckResourceAttr(channelRes, "events.#", fmt.Sprint(len(client.DefaultAlertEvents))),
					resource.TestCheckTypeSetElemAttr(channelRes, "events.*", "cert.expiring"),
					resource.TestCheckResourceAttr(channelRes, "url_masked", "https://hooks.slack.com/…AAAA"),
					resource.TestCheckNoResourceAttr(channelRes, "url_wo"),
					resource.TestCheckNoResourceAttr(channelRes, "signing_secret"),
					channelNoURLInState(channelSlackURL),
					channelCheck(func() error {
						if got := api.channelURL(channelID1); got != channelSlackURL {
							return fmt.Errorf("API has URL %q, want %q", got, channelSlackURL)
						}
						return nil
					}),
				),
			},
			{
				// Name, events and enabled update in place without sending the URL.
				Config: channelConfig(srv.URL, channelSlack("Ops alerts", channelSlackURL, 1,
					`events = ["cert.renewed", "cert.failed"]
  enabled = false`)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(channelRes, plancheck.ResourceActionUpdate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "id", channelID1),
					resource.TestCheckResourceAttr(channelRes, "name", "Ops alerts"),
					resource.TestCheckResourceAttr(channelRes, "enabled", "false"),
					resource.TestCheckResourceAttr(channelRes, "events.#", "2"),
					resource.TestCheckTypeSetElemAttr(channelRes, "events.*", "cert.renewed"),
					channelCheck(func() error {
						if got := api.patchURLs(); len(got) != 1 || got[0] != "" {
							return fmt.Errorf("PATCH URLs %q, want one PATCH without a URL", got)
						}
						return nil
					}),
				),
			},
			{
				// A new URL alone plans nothing: write-only values don't show up in a diff.
				Config: channelConfig(srv.URL, channelSlack("Ops alerts", channelSlackNew, 1,
					`events = ["cert.renewed", "cert.failed"]
  enabled = false`)),
				PlanOnly: true,
			},
			{
				// Bumping url_wo_version sends it.
				Config: channelConfig(srv.URL, channelSlack("Ops alerts", channelSlackNew, 2,
					`events = ["cert.renewed", "cert.failed"]
  enabled = false`)),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "url_masked", "https://hooks.slack.com/…BBBB"),
					resource.TestCheckResourceAttr(channelRes, "url_wo_version", "2"),
					channelNoURLInState(channelSlackURL, channelSlackNew),
					channelCheck(func() error {
						if got := api.channelURL(channelID1); got != channelSlackNew {
							return fmt.Errorf("API has URL %q, want %q", got, channelSlackNew)
						}
						if got := api.patchURLs(); len(got) != 2 || got[1] != channelSlackNew {
							return fmt.Errorf("PATCH URLs %q, want the new URL on the second PATCH", got)
						}
						return nil
					}),
				),
			},
			{
				// Removing events from the configuration goes back to the API defaults.
				Config: channelConfig(srv.URL, channelSlack("Ops alerts", channelSlackNew, 2, "")),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "events.#", fmt.Sprint(len(client.DefaultAlertEvents))),
					resource.TestCheckResourceAttr(channelRes, "enabled", "true"),
					channelCheck(func() error {
						if got := api.patchURLs(); len(got) != 3 || got[2] != "" {
							return fmt.Errorf("PATCH URLs %q, want no URL on the third PATCH", got)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestAlertChannel_webhookSecret(t *testing.T) {
	_, srv := newFakeChannelAPI(t)
	webhook := func(name string) string {
		return channelConfig(srv.URL, fmt.Sprintf(`
  type           = "webhook"
  name           = %q
  url_wo         = "https://hooks.example.com/krakenkey"
  url_wo_version = 1
  events         = ["cert.renewed"]`, name))
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: webhook("Deploy hook"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "signing_secret", "whsec_0001"),
					resource.TestCheckResourceAttr(channelRes, "url_masked", "https://hooks.example.com/…nkey"),
				),
			},
			{
				// The API never returns the secret again; refreshes and updates keep it.
				Config: webhook("Deploy hook v2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "name", "Deploy hook v2"),
					resource.TestCheckResourceAttr(channelRes, "signing_secret", "whsec_0001"),
				),
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckResourceAttr(channelRes, "signing_secret", "whsec_0001"),
			},
		},
	})
}

func TestAlertChannel_typeChangeReplaces(t *testing.T) {
	api, srv := newFakeChannelAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: channelConfig(srv.URL, channelSlack("Ops", channelSlackURL, 1, ""))},
			{
				Config: channelConfig(srv.URL, `
  type           = "teams"
  name           = "Ops"
  url_wo         = "https://prod-00.westus.logic.azure.com/workflows/abc"
  url_wo_version = 1`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(channelRes, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(channelRes, "id", "00000000-0000-4000-8000-000000000002"),
					resource.TestCheckResourceAttr(channelRes, "type", "teams"),
					channelCheck(func() error {
						if got := api.deletedIDs(); !slices.Contains(got, channelID1) {
							return fmt.Errorf("old channel was not deleted (deleted: %v)", got)
						}
						return nil
					}),
				),
			},
		},
	})
}

func TestAlertChannel_invalidValues(t *testing.T) {
	_, srv := newFakeChannelAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: channelConfig(srv.URL, `
  type           = "discord"
  name           = "Ops"
  url_wo         = "https://example.com/hook"
  url_wo_version = 1`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`"discord" is not supported`),
			},
			{
				Config:      channelConfig(srv.URL, channelSlack("Ops", channelSlackURL, 1, `events = ["cert.issued", "cert.exploded"]`)),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`"cert.exploded" is not an alert event`),
			},
			{
				Config:      channelConfig(srv.URL, channelSlack(" Ops", channelSlackURL, 1, "")),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`Invalid channel name`),
			},
			{
				// URL rules are the API's; its message is passed through.
				Config:      channelConfig(srv.URL, channelSlack("Ops", "https://example.com/hook", 1, "")),
				ExpectError: regexp.MustCompile(`Slack URL must be an incoming webhook URL`),
			},
		},
	})
}

func TestAlertChannel_deletedOutsideTerraform(t *testing.T) {
	api, srv := newFakeChannelAPI(t)
	cfg := channelConfig(srv.URL, channelSlack("Ops", channelSlackURL, 1, ""))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				// A channel missing from the list drops out of state, so the plan recreates it.
				PreConfig: func() { api.remove(channelID1) },
				Config:    cfg,
				Check:     resource.TestCheckResourceAttr(channelRes, "id", "00000000-0000-4000-8000-000000000002"),
			},
		},
	})
}

func TestAlertChannel_import(t *testing.T) {
	_, srv := newFakeChannelAPI(t)
	cfg := channelConfig(srv.URL, channelSlack("Ops", channelSlackURL, 1, `events = ["cert.failed"]`))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: cfg},
			{
				Config:            cfg,
				ResourceName:      channelRes,
				ImportState:       true,
				ImportStateVerify: true,
				// The URL and its version can't be read back from the API.
				ImportStateVerifyIgnore: []string{"url_wo_version"},
			},
		},
	})
}
