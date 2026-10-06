package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const testAPIKey = "kk_test"

// fakeAPI is an in-memory stand-in for the KrakenKey certificate endpoints.
type fakeAPI struct {
	mu      sync.Mutex
	certs   map[int]map[string]any
	nextID  int
	revoked []int
	// failWith makes new certificates fail with this reason instead of issuing.
	failWith string
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{certs: map[int]map[string]any{}, nextID: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
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

	if r.URL.Path == "/certs/tls" && r.Method == http.MethodPost {
		id := f.nextID
		f.nextID++
		f.certs[id] = map[string]any{
			"id": id, "status": "pending", "rawCsr": body["csrPem"], "crtPem": nil, "chainPem": nil,
			"expiresAt": nil, "autoRenew": true, "renewalCount": 0, "failureReason": nil,
		}
		reply(201, map[string]any{"id": id, "status": "pending"})
		return
	}

	m := regexp.MustCompile(`^/certs/tls/(\d+)(/chain|/revoke)?$`).FindStringSubmatch(r.URL.Path)
	if m == nil {
		reply(404, map[string]any{"statusCode": 404, "message": "Not found"})
		return
	}
	id, _ := strconv.Atoi(m[1])
	cert, ok := f.certs[id]
	if !ok {
		reply(404, map[string]any{"statusCode": 404, "message": "Not found"})
		return
	}
	switch {
	case r.Method == http.MethodGet && m[2] == "":
		if cert["status"] == "pending" {
			if f.failWith != "" {
				cert["status"], cert["failureReason"] = "failed", f.failWith
			} else {
				f.issue(cert, "v1")
			}
		}
		reply(200, cert)
	case r.Method == http.MethodGet && m[2] == "/chain":
		if cert["status"] != "issued" {
			reply(400, map[string]any{"statusCode": 400, "message": "not issued"})
			return
		}
		reply(200, map[string]any{"chainPem": "CHAIN", "fullChainPem": cert["crtPem"].(string) + "CHAIN"})
	case r.Method == http.MethodPatch && m[2] == "":
		cert["autoRenew"] = body["autoRenew"]
		reply(200, cert)
	case r.Method == http.MethodPost && m[2] == "/revoke":
		cert["status"] = "revoked"
		f.revoked = append(f.revoked, id)
		reply(200, map[string]any{"id": id, "status": "revoking"})
	default:
		reply(405, map[string]any{"statusCode": 405, "message": "Method not allowed"})
	}
}

func (f *fakeAPI) issue(cert map[string]any, label string) {
	cert["status"] = "issued"
	cert["crtPem"] = "LEAF-" + label
	cert["chainPem"] = "CHAIN"
	cert["expiresAt"] = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, cert["renewalCount"].(int)).Format(time.RFC3339)
}

// renew simulates a server-side renewal of certificate id.
func (f *fakeAPI) renew(id int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert := f.certs[id]
	cert["renewalCount"] = cert["renewalCount"].(int) + 1
	f.issue(cert, fmt.Sprintf("v%d", cert["renewalCount"].(int)+1))
}

func (f *fakeAPI) set(id int, key string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.certs[id][key] = v
}

func (f *fakeAPI) remove(id int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.certs, id)
}

func (f *fakeAPI) revokedIDs() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.revoked...)
}

func factories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"krakenkey": providerserver.NewProtocol6WithError(&krakenkeyProvider{version: "test", pollInterval: 10 * time.Millisecond}),
	}
}

func config(url, extra string) string {
	return fmt.Sprintf(`
provider "krakenkey" {
  api_url = %q
  api_key = %q
}

resource "krakenkey_certificate" "test" {
  csr_pem = "-----BEGIN CERTIFICATE REQUEST-----\nTEST\n-----END CERTIFICATE REQUEST-----\n"
  %s
}
`, url, testAPIKey, extra)
}

const res = "krakenkey_certificate.test"

func TestCertificate_lifecycle(t *testing.T) {
	api, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: config(srv.URL, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "id", "1"),
					resource.TestCheckResourceAttr(res, "status", "issued"),
					resource.TestCheckResourceAttr(res, "cert_pem", "LEAF-v1"),
					resource.TestCheckResourceAttr(res, "fullchain_pem", "LEAF-v1CHAIN"),
					resource.TestCheckResourceAttr(res, "auto_renew", "true"),
					resource.TestCheckResourceAttr(res, "revoke_on_destroy", "false"),
					resource.TestCheckResourceAttr(res, "expires_at", "2027-01-01T00:00:00Z"),
					resource.TestCheckResourceAttr(res, "expires_at_unix", "1798761600"),
				),
			},
			{
				// A renewal on the server shows up on refresh without a diff on arguments.
				PreConfig: func() { api.renew(1) },
				Config:    config(srv.URL, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "id", "1"),
					resource.TestCheckResourceAttr(res, "cert_pem", "LEAF-v2"),
					resource.TestCheckResourceAttr(res, "renewal_count", "1"),
					resource.TestCheckResourceAttr(res, "expires_at", "2027-01-02T00:00:00Z"),
				),
			},
			{
				// While a renewal runs, the last issued certificate stays in state.
				PreConfig: func() { api.set(1, "status", "renewing") },
				Config:    config(srv.URL, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "status", "renewing"),
					resource.TestCheckResourceAttr(res, "cert_pem", "LEAF-v2"),
					resource.TestCheckResourceAttr(res, "fullchain_pem", "LEAF-v2CHAIN"),
				),
			},
			{
				PreConfig: func() { api.set(1, "status", "issued") },
				// auto_renew updates in place.
				Config: config(srv.URL, "auto_renew = false"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(res, "id", "1"),
					resource.TestCheckResourceAttr(res, "auto_renew", "false"),
				),
			},
			{
				ResourceName:            res,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts", "revoke_on_destroy"},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if got := api.revokedIDs(); len(got) != 0 {
				return fmt.Errorf("destroy revoked %v; certificates should be kept by default", got)
			}
			return nil
		},
	})
}

func TestCertificate_revokeOnDestroy(t *testing.T) {
	api, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: config(srv.URL, "revoke_on_destroy = true")},
		},
		CheckDestroy: func(*terraform.State) error {
			if got := api.revokedIDs(); len(got) != 1 || got[0] != 1 {
				return fmt.Errorf("revoked %v, want [1]", got)
			}
			return nil
		},
	})
}

func TestCertificate_autoRenewOffAtCreate(t *testing.T) {
	_, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: config(srv.URL, "auto_renew = false"),
				Check:  resource.TestCheckResourceAttr(res, "auto_renew", "false"),
			},
		},
	})
}

func TestCertificate_failedIssuance(t *testing.T) {
	api, srv := newFakeAPI(t)
	api.failWith = "ACME challenge delegation missing for example.com"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      config(srv.URL, ""),
				ExpectError: regexp.MustCompile(`ACME challenge delegation missing`),
			},
		},
	})
}

func TestCertificate_deletedOutsideTerraform(t *testing.T) {
	api, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: config(srv.URL, "")},
			{
				// A 404 on refresh drops the resource from state, so the plan recreates it.
				PreConfig: func() { api.remove(1) },
				Config:    config(srv.URL, ""),
				Check:     resource.TestCheckResourceAttr(res, "id", "2"),
			},
		},
	})
}

func TestErrorMessage(t *testing.T) {
	_, srv := newFakeAPI(t)
	cfg := strings.Replace(config(srv.URL, ""), testAPIKey, "kk_wrong", 1)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: cfg, ExpectError: regexp.MustCompile(`returned 401: Unauthorized`)},
		},
	})
}
