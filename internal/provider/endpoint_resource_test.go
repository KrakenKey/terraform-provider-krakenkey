package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeEndpointAPI is an in-memory stand-in for the KrakenKey endpoint routes.
// It mirrors the real API's upsert on host and port and its plan limit errors.
type fakeEndpointAPI struct {
	mu        sync.Mutex
	endpoints map[string]*fakeEndpoint
	order     []string
	nextID    int
	// maxEndpoints and maxRegions of 0 mean no limit.
	maxEndpoints int
	maxRegions   int
	// posts counts POST /endpoints calls, to show a refused create never reached the API.
	posts int
}

type fakeEndpoint struct {
	ID        string
	Host      string
	Port      int
	SNI       *string
	Label     *string
	IsActive  bool
	CreatedAt time.Time
	Regions   []string
}

func newFakeEndpointAPI(t *testing.T) (*fakeEndpointAPI, *httptest.Server) {
	f := &fakeEndpointAPI{endpoints: map[string]*fakeEndpoint{}, nextID: 1}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (e *fakeEndpoint) json() map[string]any {
	regions := []map[string]any{}
	for _, r := range e.Regions {
		regions = append(regions, map[string]any{"region": r})
	}
	return map[string]any{
		"id": e.ID, "host": e.Host, "port": e.Port, "sni": e.SNI, "label": e.Label,
		"isActive": e.IsActive, "createdAt": e.CreatedAt.Format(time.RFC3339), "hostedRegions": regions,
	}
}

func (f *fakeEndpointAPI) add(host string, port int) *fakeEndpoint {
	e := &fakeEndpoint{
		ID: fmt.Sprintf("ep-%d", f.nextID), Host: host, Port: port, IsActive: true,
		CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
	}
	f.nextID++
	f.endpoints[e.ID] = e
	f.order = append(f.order, e.ID)
	return e
}

func (f *fakeEndpointAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reply := func(code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	notFound := func(msg string) { reply(404, map[string]any{"statusCode": 404, "message": msg}) }
	if r.Header.Get("Authorization") != "Bearer "+testAPIKey {
		reply(401, map[string]any{"statusCode": 401, "message": "Unauthorized"})
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	str := func(k string) *string {
		if s, ok := body[k].(string); ok {
			return &s
		}
		return nil
	}

	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if parts[0] != "endpoints" {
		notFound("Not found")
		return
	}
	switch {
	case len(parts) == 1 && r.Method == http.MethodGet:
		list := []map[string]any{}
		for _, id := range f.order {
			if e, ok := f.endpoints[id]; ok {
				list = append(list, e.json())
			}
		}
		reply(200, list)
		return
	case len(parts) == 1 && r.Method == http.MethodPost:
		f.posts++
		host, _ := body["host"].(string)
		port := 443
		if p, ok := body["port"].(float64); ok {
			port = int(p)
		}
		for _, e := range f.endpoints {
			if e.Host == host && e.Port == port {
				// Upsert: the real API returns the existing endpoint and overwrites label and sni.
				e.SNI, e.Label = str("sni"), str("label")
				reply(201, e.json())
				return
			}
		}
		if f.maxEndpoints > 0 && len(f.endpoints) >= f.maxEndpoints {
			reply(403, map[string]any{
				"statusCode": 403, "message": "Endpoint limit reached", "code": "plan_limit_exceeded",
				"limit": f.maxEndpoints, "current": len(f.endpoints), "plan": "free",
			})
			return
		}
		e := f.add(host, port)
		e.SNI, e.Label = str("sni"), str("label")
		reply(201, e.json())
		return
	}

	ep, ok := f.endpoints[parts[1]]
	if !ok {
		notFound("Endpoint #" + parts[1] + " not found")
		return
	}
	switch {
	case len(parts) == 2 && r.Method == http.MethodGet:
		reply(200, ep.json())
	case len(parts) == 2 && r.Method == http.MethodPatch:
		if _, ok := body["host"]; ok {
			reply(400, map[string]any{"statusCode": 400, "message": []string{"property host should not exist"}})
			return
		}
		if v, ok := body["sni"]; ok {
			ep.SNI = str("sni")
			_ = v
		}
		if _, ok := body["label"]; ok {
			ep.Label = str("label")
		}
		if v, ok := body["isActive"].(bool); ok {
			ep.IsActive = v
		}
		reply(200, ep.json())
	case len(parts) == 2 && r.Method == http.MethodDelete:
		delete(f.endpoints, ep.ID)
		reply(200, map[string]any{})
	case len(parts) == 3 && parts[2] == "regions" && r.Method == http.MethodPost:
		region, _ := body["region"].(string)
		if f.maxRegions > 0 && f.regionCount() >= f.maxRegions {
			reply(403, map[string]any{
				"statusCode": 403, "message": "Hosted region limit reached", "code": "plan_limit_exceeded",
				"limit": f.maxRegions, "current": f.regionCount(), "plan": "starter",
			})
			return
		}
		for _, have := range ep.Regions {
			if have == region {
				reply(201, map[string]any{"region": region})
				return
			}
		}
		ep.Regions = append(ep.Regions, region)
		reply(201, map[string]any{"region": region})
	case len(parts) == 4 && parts[2] == "regions" && r.Method == http.MethodDelete:
		for i, have := range ep.Regions {
			if have == parts[3] {
				ep.Regions = append(ep.Regions[:i], ep.Regions[i+1:]...)
				reply(200, map[string]any{})
				return
			}
		}
		notFound("Region '" + parts[3] + "' not found for endpoint #" + ep.ID)
	default:
		reply(405, map[string]any{"statusCode": 405, "message": "Method not allowed"})
	}
}

func (f *fakeEndpointAPI) regionCount() int {
	n := 0
	for _, e := range f.endpoints {
		n += len(e.Regions)
	}
	return n
}

func (f *fakeEndpointAPI) seed(host string, port int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.add(host, port).ID
}

func (f *fakeEndpointAPI) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.endpoints, id)
}

func (f *fakeEndpointAPI) addRegion(id, region string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.endpoints[id].Regions = append(f.endpoints[id].Regions, region)
}

func (f *fakeEndpointAPI) removeRegion(id, region string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	e := f.endpoints[id]
	for i, have := range e.Regions {
		if have == region {
			e.Regions = append(e.Regions[:i], e.Regions[i+1:]...)
			return
		}
	}
}

func (f *fakeEndpointAPI) endpoint(id string) *fakeEndpoint {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.endpoints[id]; ok {
		c := *e
		c.Regions = append([]string(nil), e.Regions...)
		return &c
	}
	return nil
}

func (f *fakeEndpointAPI) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.endpoints)
}

func (f *fakeEndpointAPI) postCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.posts
}

func endpointProvider(url string) string {
	return fmt.Sprintf("provider \"krakenkey\" {\n  api_url = %q\n  api_key = %q\n}\n", url, testAPIKey)
}

// endpointConfig renders one krakenkey_endpoint.test with the given arguments.
func endpointConfig(url, args string) string {
	return endpointProvider(url) + fmt.Sprintf("\nresource \"krakenkey_endpoint\" \"test\" {\n  %s\n}\n", args)
}

func endpointRegionConfig(url, endpointID, region string) string {
	return endpointProvider(url) + fmt.Sprintf("\nresource \"krakenkey_endpoint_region\" \"test\" {\n  endpoint_id = %q\n  region      = %q\n}\n", endpointID, region)
}

const (
	endpointRes       = "krakenkey_endpoint.test"
	endpointRegionRes = "krakenkey_endpoint_region.test"
)

func TestEndpoint_lifecycle(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: endpointConfig(srv.URL, `host = "example.com"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "id", "ep-1"),
					resource.TestCheckResourceAttr(endpointRes, "port", "443"),
					resource.TestCheckResourceAttr(endpointRes, "is_active", "true"),
					resource.TestCheckNoResourceAttr(endpointRes, "label"),
					resource.TestCheckNoResourceAttr(endpointRes, "sni"),
					resource.TestCheckResourceAttr(endpointRes, "created_at", "2026-10-01T00:00:00Z"),
				),
			},
			{
				// sni, label and is_active change in place, with one PATCH.
				Config: endpointConfig(srv.URL, `host = "example.com"
  sni       = "www.example.com"
  label     = "Public site"
  is_active = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "id", "ep-1"),
					resource.TestCheckResourceAttr(endpointRes, "sni", "www.example.com"),
					resource.TestCheckResourceAttr(endpointRes, "label", "Public site"),
					resource.TestCheckResourceAttr(endpointRes, "is_active", "false"),
				),
			},
			{
				// Dropping the label clears it on the server.
				Config: endpointConfig(srv.URL, `host = "example.com"
  sni       = "www.example.com"
  is_active = true`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "id", "ep-1"),
					resource.TestCheckNoResourceAttr(endpointRes, "label"),
					resource.TestCheckResourceAttr(endpointRes, "is_active", "true"),
					func(*terraform.State) error {
						if e := api.endpoint("ep-1"); e == nil || e.Label != nil {
							return fmt.Errorf("label should be cleared, got %+v", e)
						}
						return nil
					},
				),
			},
			{
				// Removing sni sends null and clears it on the server.
				Config: endpointConfig(srv.URL, `host = "example.com"`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "id", "ep-1"),
					resource.TestCheckNoResourceAttr(endpointRes, "sni"),
					func(*terraform.State) error {
						if e := api.endpoint("ep-1"); e == nil || e.SNI != nil {
							return fmt.Errorf("sni should be cleared, got %+v", e)
						}
						return nil
					},
				),
			},
			{
				// A change to host or port replaces the endpoint.
				Config: endpointConfig(srv.URL, `host = "example.com"
  port = 8443`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "id", "ep-2"),
					resource.TestCheckResourceAttr(endpointRes, "port", "8443"),
				),
			},
			{
				Config: endpointConfig(srv.URL, `host = "other.example.com"
  port = 8443`),
				Check: resource.TestCheckResourceAttr(endpointRes, "id", "ep-3"),
			},
			{
				ResourceName:      endpointRes,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if n := api.count(); n != 0 {
				return fmt.Errorf("%d endpoints left after destroy", n)
			}
			return nil
		},
	})
}

func TestEndpoint_inactiveAtCreate(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: endpointConfig(srv.URL, `host = "example.com"
  is_active = false`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRes, "is_active", "false"),
					func(*terraform.State) error {
						if e := api.endpoint("ep-1"); e == nil || e.IsActive {
							return fmt.Errorf("endpoint should be inactive on the server, got %+v", e)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestEndpoint_existingHostAndPortRefused(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      endpointConfig(srv.URL, `host = "example.com"`),
				ExpectError: regexp.MustCompile(`(?s)already exists.*terraform import.*` + id),
			},
			{
				// Another port on the same host is a different endpoint.
				Config: endpointConfig(srv.URL, `host = "example.com"
  port = 8443`),
				Check: resource.TestCheckResourceAttr(endpointRes, "port", "8443"),
			},
		},
	})
	if api.postCount() != 1 {
		t.Errorf("POST /endpoints was called %d times; the refused create must not reach it", api.postCount())
	}
	if e := api.endpoint(id); e == nil || e.Label != nil {
		t.Errorf("the existing endpoint was modified: %+v", e)
	}
}

func TestEndpoint_planLimit(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	api.maxEndpoints = 1
	api.seed("taken.example.com", 443)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      endpointConfig(srv.URL, `host = "example.com"`),
				ExpectError: regexp.MustCompile(`Endpoint\s+limit\s+reached\s+\(limit\s+1,\s+in\s+use\s+1,\s+plan\s+free\)`),
			},
		},
	})
}

func TestEndpoint_deletedOutsideTerraform(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: endpointConfig(srv.URL, `host = "example.com"`)},
			{
				// A 404 on refresh drops the resource from state, so the plan recreates it.
				PreConfig: func() { api.remove("ep-1") },
				Config:    endpointConfig(srv.URL, `host = "example.com"`),
				Check:     resource.TestCheckResourceAttr(endpointRes, "id", "ep-2"),
			},
		},
	})
}

func TestEndpointRegion_lifecycle(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: endpointRegionConfig(srv.URL, id, "us-east-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRegionRes, "id", id+"/us-east-1"),
					func(*terraform.State) error {
						if e := api.endpoint(id); e == nil || len(e.Regions) != 1 || e.Regions[0] != "us-east-1" {
							return fmt.Errorf("regions on the server: %+v", e)
						}
						return nil
					},
				),
			},
			{
				// Changing the region replaces the assignment.
				Config: endpointRegionConfig(srv.URL, id, "eu-west-1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(endpointRegionRes, "id", id+"/eu-west-1"),
					func(*terraform.State) error {
						if e := api.endpoint(id); e == nil || len(e.Regions) != 1 || e.Regions[0] != "eu-west-1" {
							return fmt.Errorf("regions on the server: %+v", e)
						}
						return nil
					},
				),
			},
			{
				ResourceName:      endpointRegionRes,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if e := api.endpoint(id); e == nil || len(e.Regions) != 0 {
				return fmt.Errorf("regions left after destroy: %+v", e)
			}
			return nil
		},
	})
}

func TestEndpointRegion_importByID(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	api.addRegion(id, "us-east-1")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:             endpointRegionConfig(srv.URL, id, "us-east-1"),
				ResourceName:       endpointRegionRes,
				ImportState:        true,
				ImportStateId:      id + "/us-east-1",
				ImportStatePersist: true,
			},
			{
				// After the import the configuration matches, so there is nothing to do.
				Config:   endpointRegionConfig(srv.URL, id, "us-east-1"),
				PlanOnly: true,
			},
		},
	})
}

func TestEndpointRegion_alreadyOnEndpointRefused(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	api.addRegion(id, "us-east-1")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      endpointRegionConfig(srv.URL, id, "us-east-1"),
				ExpectError: regexp.MustCompile(`(?s)already on endpoint.*terraform import.*` + id + `/us-east-1`),
			},
		},
	})
}

func TestEndpointRegion_removedOutsideTerraform(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: endpointRegionConfig(srv.URL, id, "us-east-1")},
			{
				// Read drops the region from state, so the plan adds it back.
				PreConfig: func() { api.removeRegion(id, "us-east-1") },
				Config:    endpointRegionConfig(srv.URL, id, "us-east-1"),
				Check: func(*terraform.State) error {
					if e := api.endpoint(id); e == nil || len(e.Regions) != 1 {
						return fmt.Errorf("region was not added back: %+v", e)
					}
					return nil
				},
			},
		},
	})
}

func TestEndpointRegion_endpointDeletedOutsideTerraform(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 443)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{Config: endpointRegionConfig(srv.URL, id, "us-east-1")},
			{
				// The endpoint is gone, so the region leaves state and the re-add fails with a 404.
				PreConfig:   func() { api.remove(id) },
				Config:      endpointRegionConfig(srv.URL, id, "us-east-1"),
				ExpectError: regexp.MustCompile(`not found`),
			},
		},
	})
}

func TestEndpointRegion_planLimit(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	api.maxRegions = 1
	id := api.seed("example.com", 443)
	api.addRegion(id, "us-east-1")
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      endpointRegionConfig(srv.URL, id, "eu-west-1"),
				ExpectError: regexp.MustCompile(`Hosted\s+region\s+limit\s+reached\s+\(limit\s+1,\s+in\s+use\s+1,\s+plan\s+starter\)`),
			},
		},
	})
}

func TestEndpointDataSource(t *testing.T) {
	api, srv := newFakeEndpointAPI(t)
	id := api.seed("example.com", 8443)
	api.addRegion(id, "us-east-1")
	api.addRegion(id, "eu-west-1")
	cfg := endpointProvider(srv.URL) + fmt.Sprintf("\ndata \"krakenkey_endpoint\" \"test\" {\n  id = %q\n}\n", id)
	const ds = "data.krakenkey_endpoint.test"
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(ds, "host", "example.com"),
					resource.TestCheckResourceAttr(ds, "port", "8443"),
					resource.TestCheckResourceAttr(ds, "is_active", "true"),
					resource.TestCheckNoResourceAttr(ds, "label"),
					resource.TestCheckResourceAttr(ds, "created_at", "2026-10-01T00:00:00Z"),
					resource.TestCheckResourceAttr(ds, "hosted_regions.#", "2"),
					resource.TestCheckResourceAttr(ds, "hosted_regions.0", "us-east-1"),
					resource.TestCheckResourceAttr(ds, "hosted_regions.1", "eu-west-1"),
				),
			},
		},
	})
}
