package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeDomainAPI is an in-memory stand-in for the KrakenKey domain endpoints.
type fakeDomainAPI struct {
	mu      sync.Mutex
	domains map[string]map[string]any
	nextID  int
	// verifyCalls counts POST /domains/:id/verify requests.
	verifyCalls int
	// missingFor makes this many verify calls fail as if the TXT record has not propagated.
	missingFor int
	// verifyStatus, if set, makes every verify call fail with this status.
	verifyStatus int
}

func newFakeDomainAPI(t *testing.T) (*fakeDomainAPI, *httptest.Server) {
	f := &fakeDomainAPI{domains: map[string]map[string]any{}, nextID: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func domainTestID(n int) string {
	return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
}

// add registers a domain directly, as if done outside Terraform.
func (f *fakeDomainAPI) add(hostname string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.addLocked(hostname)
}

func (f *fakeDomainAPI) addLocked(hostname string) string {
	id := domainTestID(f.nextID)
	f.domains[id] = map[string]any{
		"id": id, "hostname": hostname, "isVerified": false, "userId": "user-1",
		"verificationCode": fmt.Sprintf("krakenkey-site-verification=%032d", f.nextID),
		"createdAt":        "2026-10-06T12:00:00.000Z", "updatedAt": "2026-10-06T12:00:00.000Z",
	}
	f.nextID++
	return id
}

func (f *fakeDomainAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(code int, msg string) {
		reply(code, map[string]any{"statusCode": code, "message": msg, "path": r.URL.Path})
	}
	if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
		fail(401, "Unauthorized")
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	if r.URL.Path == "/domains" {
		switch r.Method {
		case http.MethodGet:
			list := []map[string]any{}
			for _, d := range f.domains {
				list = append(list, d)
			}
			reply(200, list)
		case http.MethodPost:
			hostname, _ := body["hostname"].(string)
			// Like the API, a repeat of the same hostname returns the existing domain.
			for _, d := range f.domains {
				if d["hostname"] == hostname {
					reply(201, d)
					return
				}
			}
			reply(201, f.domains[f.addLocked(hostname)])
		default:
			fail(405, "Method not allowed")
		}
		return
	}

	m := regexp.MustCompile(`^/domains/([^/]+)(/verify)?$`).FindStringSubmatch(r.URL.Path)
	if m == nil {
		fail(404, "Cannot "+r.Method+" "+r.URL.Path)
		return
	}
	d, ok := f.domains[m[1]]
	if !ok {
		fail(404, "Domain #"+m[1]+" not found")
		return
	}
	switch {
	case r.Method == http.MethodGet && m[2] == "":
		reply(200, d)
	case r.Method == http.MethodPost && m[2] == "/verify":
		f.verifyCalls++
		switch {
		case f.verifyStatus != 0:
			fail(f.verifyStatus, "This API key needs the domains:write scope for this request.")
		case d["isVerified"] == true:
			reply(201, d)
		case f.missingFor > 0:
			f.missingFor--
			fail(400, "Verification TXT record not found. Please ensure the record has propagated and try again.")
		default:
			d["isVerified"] = true
			reply(201, d)
		}
	case r.Method == http.MethodDelete && m[2] == "":
		delete(f.domains, m[1])
		w.WriteHeader(200)
	default:
		fail(405, "Method not allowed")
	}
}

func (f *fakeDomainAPI) setVerified(id string, v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.domains[id]["isVerified"] = v
}

func (f *fakeDomainAPI) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.domains, id)
}

func (f *fakeDomainAPI) verifies() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.verifyCalls
}

func (f *fakeDomainAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.domains)
}

// domainConfig returns a domain and, if verify is set, its verification.
// providerExtra and verifyExtra go into the provider and verification blocks.
func domainConfig(url, hostname, providerExtra string, verify bool, verifyExtra string) string {
	cfg := fmt.Sprintf(`
provider "krakenkey" {
  api_url = %q
  api_key = %q
  %s
}

resource "krakenkey_domain" "test" {
  hostname = %q
}
`, url, testAPIKey, providerExtra, hostname)
	if verify {
		cfg += fmt.Sprintf(`
resource "krakenkey_domain_verification" "test" {
  domain_id = krakenkey_domain.test.id
  %s
}
`, verifyExtra)
	}
	return cfg
}

const (
	domainRes       = "krakenkey_domain.test"
	domainVerifyRes = "krakenkey_domain_verification.test"
)

func domainCheckVerifies(api *fakeDomainAPI, want int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if got := api.verifies(); got != want {
			return fmt.Errorf("verify called %d times, want %d", got, want)
		}
		return nil
	}
}

func TestDomain_lifecycle(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	id := domainTestID(1)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: domainConfig(srv.URL, "example.com", "", false, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainRes, "id", id),
					resource.TestCheckResourceAttr(domainRes, "hostname", "example.com"),
					resource.TestCheckResourceAttr(domainRes, "verified", "false"),
					resource.TestCheckResourceAttr(domainRes, "verification_code", "krakenkey-site-verification=00000000000000000000000000000001"),
					resource.TestCheckResourceAttr(domainRes, "txt_record_name", "example.com"),
					resource.TestCheckResourceAttr(domainRes, "txt_record_value", "krakenkey-site-verification=00000000000000000000000000000001"),
					resource.TestCheckResourceAttr(domainRes, "cname_record_name", "_acme-challenge.example.com"),
					resource.TestCheckResourceAttr(domainRes, "cname_record_value", "example-com.acme.krakenkey.io"),
					resource.TestCheckResourceAttr(domainRes, "created_at", "2026-10-06T12:00:00Z"),
				),
			},
			{
				Config: domainConfig(srv.URL, "example.com", "", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainVerifyRes, "id", id),
					resource.TestCheckResourceAttr(domainVerifyRes, "domain_id", id),
					resource.TestCheckResourceAttr(domainVerifyRes, "verified", "true"),
					domainCheckVerifies(api, 1),
				),
			},
			{
				// The domain's verified flag catches up on refresh, without a diff.
				Config: domainConfig(srv.URL, "example.com", "", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainRes, "verified", "true"),
					domainCheckVerifies(api, 1),
				),
			},
			{
				ResourceName:      domainRes,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				ResourceName:            domainVerifyRes,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if n := api.count(); n != 0 {
				return fmt.Errorf("%d domains left after destroy", n)
			}
			return nil
		},
	})
}

func TestDomain_acmeZone(t *testing.T) {
	_, srv := newFakeDomainAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: domainConfig(srv.URL, "sub.example.co.uk", `acme_zone = "ACME.Example.NET."`, false, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainRes, "cname_record_name", "_acme-challenge.sub.example.co.uk"),
					resource.TestCheckResourceAttr(domainRes, "cname_record_value", "sub-example-co-uk.acme.example.net"),
				),
			},
		},
	})
}

func TestDomainACMEZoneFor(t *testing.T) {
	cases := []struct{ apiURL, override, want string }{
		{"", "", "acme.krakenkey.io"},
		{"https://api.krakenkey.io", "", "acme.krakenkey.io"},
		{"https://api-dev.krakenkey.io", "", "acme.dev.krakenkey.io"},
		{"https://API-DEV.krakenkey.io/", "", "acme.dev.krakenkey.io"},
		{"https://api-dev.krakenkey.io", "acme.example.net", "acme.example.net"},
		{"http://127.0.0.1:8080", "", "acme.krakenkey.io"},
	}
	for _, tc := range cases {
		if got := acmeZoneFor(tc.apiURL, tc.override); got != tc.want {
			t.Errorf("acmeZoneFor(%q, %q) = %q, want %q", tc.apiURL, tc.override, got, tc.want)
		}
	}
}

func TestDomain_alreadyRegistered(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	id := api.add("example.com")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      domainConfig(srv.URL, "example.com", "", false, ""),
				ExpectError: regexp.MustCompile(`already\s+registered[\s\S]*terraform\s+import\s+<address>\s+` + id),
			},
		},
	})
	if n := api.count(); n != 1 {
		t.Fatalf("%d domains after the failed create, want the existing one kept", n)
	}
}

func TestDomain_invalidHostname(t *testing.T) {
	_, srv := newFakeDomainAPI(t)
	for hostname, want := range map[string]string{
		"Example.com":   "Hostname must be lowercase",
		"*.example.com": "Wildcard hostname",
		"example.com.":  "Trailing dot",
	} {
		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: factories(),
			Steps: []resource.TestStep{
				{Config: domainConfig(srv.URL, hostname, "", false, ""), ExpectError: regexp.MustCompile(want)},
			},
		})
	}
}

func TestDomain_deletedOutsideTerraform(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: domainConfig(srv.URL, "example.com", "", true, "")},
			{
				// A 404 on refresh drops both resources from state, so the plan recreates them.
				PreConfig: func() { api.remove(domainTestID(1)) },
				Config:    domainConfig(srv.URL, "example.com", "", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainRes, "id", domainTestID(2)),
					resource.TestCheckResourceAttr(domainVerifyRes, "domain_id", domainTestID(2)),
					domainCheckVerifies(api, 2),
				),
			},
		},
	})
}

func TestDomainVerification_retriesUntilFound(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	api.missingFor = 3
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: domainConfig(srv.URL, "example.com", "", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainVerifyRes, "verified", "true"),
					domainCheckVerifies(api, 4),
				),
			},
		},
	})
}

func TestDomainVerification_timeout(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	api.missingFor = 1 << 30
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      domainConfig(srv.URL, "example.com", "", true, `timeouts = { create = "500ms" }`),
				ExpectError: regexp.MustCompile(`TXT\s+record\s+example\.com\s+=\s+"krakenkey-site-verification=0+1"[\s\S]*Verification\s+TXT\s+record\s+not\s+found`),
			},
		},
	})
	if n := api.verifies(); n < 2 {
		t.Fatalf("verify called %d times, want retries before the timeout", n)
	}
}

func TestDomainVerification_permanentError(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	api.verifyStatus = http.StatusForbidden
	start := time.Now()
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      domainConfig(srv.URL, "example.com", "", true, `timeouts = { create = "1m" }`),
				ExpectError: regexp.MustCompile(`returned\s+403:\s+This\s+API\s+key\s+needs\s+the\s+domains:write\s+scope`),
			},
		},
	})
	if n := api.verifies(); n != 1 {
		t.Fatalf("verify called %d times, want 1: a 403 should fail without retrying", n)
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Fatalf("took %s; a permanent error should not wait for the timeout", d)
	}
}

func TestDomainVerification_unverifiedOnRefresh(t *testing.T) {
	api, srv := newFakeDomainAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: domainConfig(srv.URL, "example.com", "", true, "")},
			{
				// The daily re-check cleared the flag: refresh drops the
				// verification from state and the apply verifies again.
				PreConfig: func() { api.setVerified(domainTestID(1), false) },
				Config:    domainConfig(srv.URL, "example.com", "", true, ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(domainVerifyRes, "verified", "true"),
					resource.TestCheckResourceAttr(domainRes, "id", domainTestID(1)),
					domainCheckVerifies(api, 2),
				),
			},
		},
	})
}
