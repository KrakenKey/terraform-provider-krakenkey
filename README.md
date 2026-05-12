# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.com) — automated TLS certificate management and endpoint monitoring.

> **Status**: Under development. Not yet published to the Terraform Registry.

---

## Overview

This provider allows you to manage KrakenKey resources using Terraform:

- Issue and renew TLS certificates via ACME DNS-01 challenges
- Register and configure monitored endpoints
- Manage API credentials
- Configure hosted probe regions (Starter tier and above)

---

## Requirements

| Requirement | Version |
|-------------|--------|
| Terraform | >= 1.5 |
| Go (for development) | >= 1.22 |

---

## Authentication

The provider authenticates using a KrakenKey API key. Set it via environment variable (recommended) or directly in the provider block.

```hcl
provider "krakenkey" {
  api_key = var.krakenkey_api_key   # or set KRAKENKEY_API_KEY env var
}
```

| Environment Variable | Description |
|----------------------|-------------|
| `KRAKENKEY_API_KEY` | API key from your KrakenKey account settings |
| `KRAKENKEY_API_BASE_URL` | Override the API base URL (default: `https://api.krakenkey.com`) |

---

## Resources

See [docs/RESOURCES.md](./docs/RESOURCES.md) for full resource and data source schemas.

### Quick Reference

| Resource | Description |
|----------|-------------|
| `krakenkey_domain` | Register a domain for certificate management |
| `krakenkey_certificate` | Issue or renew a TLS certificate for a domain |
| `krakenkey_endpoint` | Configure an HTTPS endpoint for monitoring |
| `krakenkey_endpoint_region` | Assign a hosted probe region to an endpoint (Starter+) |
| `krakenkey_api_key` | Manage API keys for CI/CD pipelines |

### Data Sources

| Data Source | Description |
|-------------|-------------|
| `krakenkey_certificate` | Read an existing certificate's metadata and PEM outputs |
| `krakenkey_endpoint` | Read an existing endpoint's current TLS status |

---

## Example Usage

```hcl
provider "krakenkey" {}

# Register a domain
resource "krakenkey_domain" "example" {
  domain = "example.com"
}

# Issue a certificate (DNS-01 via ACME)
resource "krakenkey_certificate" "example" {
  domain_id  = krakenkey_domain.example.id
  cert_path       = "/etc/ssl/example/cert.pem"
  chain_path      = "/etc/ssl/example/chain.pem"
  fullchain_path  = "/etc/ssl/example/fullchain.pem"
}

# Monitor the endpoint
resource "krakenkey_endpoint" "example" {
  domain_id = krakenkey_domain.example.id
  url       = "https://example.com"
  name      = "Production"
}

# Add a hosted probe region (requires Starter tier or above)
resource "krakenkey_endpoint_region" "us_east" {
  endpoint_id = krakenkey_endpoint.example.id
  region      = "us-east-1"
}
```

---

## Development

See [docs/CONTRIBUTING.md](./docs/CONTRIBUTING.md) for build, test, and release instructions.

---

## License

MIT. See [LICENSE](./LICENSE).
