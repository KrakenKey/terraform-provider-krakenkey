# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.io): TLS certificate issuance through Let's Encrypt (ACME DNS-01) and TLS endpoint monitoring.

> **Status: not implemented.** This repository holds documentation only. There is no Go source, no `go.mod`, no release, and nothing on the Terraform Registry, so `terraform init` cannot install `krakenkey/krakenkey` and the build steps in [CONTRIBUTING.md](CONTRIBUTING.md) have nothing to build.
>
> [DESIGN.md](DESIGN.md) is a proposed schema, checked against the public API. Names, types and behavior can change during implementation and are not a compatibility promise.
>
> Until there is a release, use the [CLI](https://github.com/KrakenKey/cli), the [GitHub Action](https://github.com/KrakenKey/cert-action), or the [REST API](https://krakenkey.io/docs/api/).

## Planned scope

The provider wraps the KrakenKey REST API (`https://api.krakenkey.io`). The proposed first version covers:

| Type | Name | Purpose |
|------|------|---------|
| Resource | `krakenkey_domain` | Register a domain and expose the DNS records it needs |
| Resource | `krakenkey_certificate` | Submit a CSR and wait for the issued certificate |
| Resource | `krakenkey_endpoint` | Monitor a TLS endpoint |
| Resource | `krakenkey_endpoint_region` | Add a hosted probe region to an endpoint (Starter plan and above) |
| Data source | `krakenkey_certificate` | Read a certificate by ID |
| Data source | `krakenkey_endpoint` | Read an endpoint by ID |

KrakenKey never generates or stores private keys. The certificate resource takes a CSR, so the private key stays wherever you create it (for example the `hashicorp/tls` provider, or a file generated outside Terraform). See [DESIGN.md](DESIGN.md) for each schema, the DNS ordering rules and the open questions.

## Planned authentication

The provider would authenticate with a KrakenKey user API key (`kk_...`), the same key the CLI uses. Create one in the dashboard or with `krakenkey auth login --web`.

```bash
export KK_API_KEY=kk_...
```

```hcl
provider "krakenkey" {
  # api_key = "kk_..."                    # or KK_API_KEY
  # api_url = "https://api.krakenkey.io"  # or KK_API_URL
}
```

## Example of the proposed interface

This does not run yet. It shows the intended shape: a domain, its two DNS records, and a certificate for a CSR made with the `hashicorp/tls` provider.

```hcl
resource "krakenkey_domain" "example" {
  hostname = "example.com"
}

# DNS records come from your DNS provider (Cloudflare shown here).
resource "cloudflare_record" "kk_verify" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.txt_record_name
  type    = "TXT"
  content = krakenkey_domain.example.txt_record_value
}

resource "cloudflare_record" "kk_acme" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.cname_record_name
  type    = "CNAME"
  content = krakenkey_domain.example.cname_record_value
  proxied = false
}

resource "tls_private_key" "web" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

resource "tls_cert_request" "web" {
  private_key_pem = tls_private_key.web.private_key_pem
  subject {
    common_name = "example.com"
  }
  dns_names = ["example.com", "www.example.com"]
}

resource "krakenkey_certificate" "web" {
  csr_pem = tls_cert_request.web.cert_request_pem

  # The API checks domain verification and the _acme-challenge CNAME.
  # Terraform cannot see that dependency, so state it.
  depends_on = [cloudflare_record.kk_verify, cloudflare_record.kk_acme]
}
```

Domain verification also has to happen between the TXT record and the certificate. How the provider triggers it is an open question in [DESIGN.md](DESIGN.md#open-design-questions).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Implementation work is tracked in this repository's issues.

## Related projects

| Repository | Description |
|------------|-------------|
| [KrakenKey/KrakenKey](https://github.com/KrakenKey/KrakenKey) | Monorepo with docs and dev environment |
| [KrakenKey/app](https://github.com/KrakenKey/app) | NestJS API and React dashboard |
| [KrakenKey/cli](https://github.com/KrakenKey/cli) | Command line client (Go) |
| [KrakenKey/cert-action](https://github.com/KrakenKey/cert-action) | GitHub Action for certificates |
| [KrakenKey/probe](https://github.com/KrakenKey/probe) | TLS endpoint probe (Go) |

Documentation: <https://krakenkey.io/docs/>

## License

No license has been chosen for this repository yet. One will be added before the first release.
