# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.io): TLS certificate issuance through Let's Encrypt (ACME DNS-01) and TLS endpoint monitoring.

## Install

Requires Terraform 1.11 or later (or OpenTofu 1.11 or later) for the write-only arguments used in the examples.

```hcl
terraform {
  required_providers {
    krakenkey = {
      source  = "krakenkey/krakenkey"
      version = "~> 0.1"
    }
  }
}
```

Reference docs for every resource and data source are on the [Terraform Registry](https://registry.terraform.io/providers/krakenkey/krakenkey/latest/docs) and in [`docs/`](docs/). Versions before 1.0 can change names, types and behavior between minor releases; check the release notes before upgrading.

## Resources and data sources

The provider wraps the KrakenKey REST API (`https://api.krakenkey.io`).

| Type | Name | Purpose |
|------|------|---------|
| Resource | `krakenkey_domain` | Register a domain and expose the DNS records it needs |
| Resource | `krakenkey_domain_verification` | Verify a domain once its TXT record is published |
| Resource | `krakenkey_certificate` | Submit a CSR and wait for the issued certificate |
| Resource | `krakenkey_endpoint` | Monitor a TLS endpoint |
| Resource | `krakenkey_endpoint_region` | Add a hosted probe region to an endpoint (Starter plan and above) |
| Resource | `krakenkey_alert_channel` | Send certificate and endpoint alerts to a webhook, Slack, Discord or Teams |
| Data source | `krakenkey_certificate` | Read a certificate by ID |
| Data source | `krakenkey_endpoint` | Read an endpoint by ID |

Examples for each are in [`examples/`](examples/).

KrakenKey never generates or stores private keys. The certificate resource takes a CSR, so the private key stays wherever you create it (for example the `hashicorp/tls` provider, or a file generated outside Terraform). See [DESIGN.md](DESIGN.md) for each schema, the DNS ordering rules and the open questions.

## Authentication

The provider authenticates with a KrakenKey user API key (`kk_...`), the same key the CLI uses. Create one in the dashboard or with `krakenkey auth login --web`.

```bash
export KK_API_KEY=kk_...
```

```hcl
provider "krakenkey" {
  # api_key = "kk_..."                    # or KK_API_KEY
  # api_url = "https://api.krakenkey.io"  # or KK_API_URL
}
```

## Example

A domain, its two DNS records, and a certificate for a CSR made with the `hashicorp/tls` provider. The private key is ephemeral and never written to state; [DESIGN.md](DESIGN.md#private-keys) covers where to keep it.

```hcl
resource "krakenkey_domain" "example" {
  hostname = "example.com"
}

# DNS records come from your DNS provider (Cloudflare provider v5 shown here).
resource "cloudflare_dns_record" "kk_verify" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.txt_record_name
  type    = "TXT"
  content = krakenkey_domain.example.txt_record_value
  ttl     = 1
}

resource "cloudflare_dns_record" "kk_acme" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.cname_record_name
  type    = "CNAME"
  content = krakenkey_domain.example.cname_record_value
  ttl     = 1
  proxied = false
}

ephemeral "tls_private_key" "web" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

# Hand the same key to your secret store with its write-only argument and the
# same version; see prototype/option-b.
resource "tls_cert_request" "web" {
  private_key_pem_wo         = ephemeral.tls_private_key.web.private_key_pem
  private_key_pem_wo_version = 1
  subject {
    common_name = "example.com"
  }
  dns_names = ["example.com", "www.example.com"]
}

resource "krakenkey_certificate" "web" {
  csr_pem = tls_cert_request.web.cert_request_pem

  # The API checks domain verification and the _acme-challenge CNAME.
  # Terraform cannot see that dependency, so state it.
  depends_on = [krakenkey_domain_verification.example, cloudflare_dns_record.kk_acme]
}

resource "krakenkey_domain_verification" "example" {
  domain_id  = krakenkey_domain.example.id
  depends_on = [cloudflare_dns_record.kk_verify]
}
```

Destroying a certificate keeps it valid in KrakenKey unless `revoke_on_destroy = true`. KrakenKey renews it on the server; how the renewed certificate reaches your servers is covered in [DESIGN.md](DESIGN.md#renewal-delivery).

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

[Mozilla Public License 2.0](LICENSE), the license used by HashiCorp's provider libraries and most Terraform providers.
