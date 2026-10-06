package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const certDS = "data.krakenkey_certificate.test"

const certDSCSR = "-----BEGIN CERTIFICATE REQUEST-----\nDS\n-----END CERTIFICATE REQUEST-----\n"

func certDSProvider(url string) string {
	return fmt.Sprintf(`
provider "krakenkey" {
  api_url = %q
  api_key = %q
}
`, url, testAPIKey)
}

func certDSConfig(url, id string) string {
	return certDSProvider(url) + fmt.Sprintf(`
data "krakenkey_certificate" "test" {
  id = %q
}
`, id)
}

// certDSSeed adds an issued certificate with ID 1 straight to the fake API.
func certDSSeed(api *fakeAPI) {
	api.mu.Lock()
	defer api.mu.Unlock()
	cert := map[string]any{
		"id": 1, "status": "pending", "rawCsr": certDSCSR, "crtPem": nil, "chainPem": nil,
		"expiresAt": nil, "autoRenew": false, "renewalCount": 2, "failureReason": nil,
	}
	api.issue(cert, "v3")
	api.certs[1] = cert
	api.nextID = 2
}

func TestCertificateDataSource_issued(t *testing.T) {
	api, srv := newFakeAPI(t)
	certDSSeed(api)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: certDSConfig(srv.URL, "1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(certDS, "id", "1"),
					resource.TestCheckResourceAttr(certDS, "status", "issued"),
					resource.TestCheckResourceAttr(certDS, "csr_pem", certDSCSR),
					resource.TestCheckResourceAttr(certDS, "auto_renew", "false"),
					resource.TestCheckResourceAttr(certDS, "renewal_count", "2"),
					resource.TestCheckResourceAttr(certDS, "cert_pem", "LEAF-v3"),
					resource.TestCheckResourceAttr(certDS, "chain_pem", "CHAIN"),
					resource.TestCheckResourceAttr(certDS, "fullchain_pem", "LEAF-v3CHAIN"),
					resource.TestCheckResourceAttr(certDS, "expires_at", "2027-01-03T00:00:00Z"),
					resource.TestCheckResourceAttr(certDS, "expires_at_unix", "1798934400"),
					resource.TestCheckNoResourceAttr(certDS, "failure_reason"),
				),
			},
		},
	})
}

func TestCertificateDataSource_notIssued(t *testing.T) {
	api, srv := newFakeAPI(t)
	certDSSeed(api)
	for name, status := range map[string]string{"renewing": "renewing", "failed": "failed"} {
		t.Run(name, func(t *testing.T) {
			api.set(1, "status", status)
			api.set(1, "failureReason", "ACME challenge delegation missing")
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: factories(),
				Steps: []resource.TestStep{
					{
						Config: certDSConfig(srv.URL, "1"),
						Check: resource.ComposeAggregateTestCheckFunc(
							resource.TestCheckResourceAttr(certDS, "status", status),
							resource.TestCheckResourceAttr(certDS, "failure_reason", "ACME challenge delegation missing"),
							resource.TestCheckResourceAttr(certDS, "csr_pem", certDSCSR),
							resource.TestCheckNoResourceAttr(certDS, "cert_pem"),
							resource.TestCheckNoResourceAttr(certDS, "chain_pem"),
							resource.TestCheckNoResourceAttr(certDS, "fullchain_pem"),
							resource.TestCheckNoResourceAttr(certDS, "expires_at"),
							resource.TestCheckNoResourceAttr(certDS, "expires_at_unix"),
						),
					},
				},
			})
		})
	}
}

func TestCertificateDataSource_notFound(t *testing.T) {
	_, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      certDSConfig(srv.URL, "99"),
				ExpectError: regexp.MustCompile(`Certificate 99 not found`),
			},
		},
	})
}

func TestCertificateDataSource_invalidID(t *testing.T) {
	_, srv := newFakeAPI(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config:      certDSConfig(srv.URL, "abc"),
				ExpectError: regexp.MustCompile(`Invalid certificate ID`),
			},
		},
	})
}

func TestCertificateDataSource_nextToResource(t *testing.T) {
	_, srv := newFakeAPI(t)
	cfg := config(srv.URL, "") + `
data "krakenkey_certificate" "test" {
  id = krakenkey_certificate.test.id
}
`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories(),
		Steps: []resource.TestStep{
			{
				Config: cfg,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(certDS, "id", res, "id"),
					resource.TestCheckResourceAttrPair(certDS, "status", res, "status"),
					resource.TestCheckResourceAttrPair(certDS, "csr_pem", res, "csr_pem"),
					resource.TestCheckResourceAttrPair(certDS, "auto_renew", res, "auto_renew"),
					resource.TestCheckResourceAttrPair(certDS, "cert_pem", res, "cert_pem"),
					resource.TestCheckResourceAttrPair(certDS, "chain_pem", res, "chain_pem"),
					resource.TestCheckResourceAttrPair(certDS, "fullchain_pem", res, "fullchain_pem"),
					resource.TestCheckResourceAttrPair(certDS, "expires_at", res, "expires_at"),
					resource.TestCheckResourceAttrPair(certDS, "expires_at_unix", res, "expires_at_unix"),
					resource.TestCheckResourceAttrPair(certDS, "renewal_count", res, "renewal_count"),
				),
			},
		},
	})
}
