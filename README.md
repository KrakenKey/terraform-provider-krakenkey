# Terraform Provider for KrakenKey

[![Registry](https://img.shields.io/badge/Terraform%20Registry-krakenkey-623CE4)](https://registry.terraform.io/providers/krakenkey/krakenkey)
[![License: AGPL-3.0](https://img.shields.io/badge/license-AGPL--3.0-blue.svg)](LICENSE)

Manage KrakenKey domains, TLS certificates, endpoint monitoring, and API keys with Terraform.

## Overview

The KrakenKey Terraform provider lets you automate the full certificate lifecycle declaratively:

- Register and verify domains
- Issue and renew TLS certificates (leaf, chain, fullchain PEM outputs)
- Configure HTTPS endpoint monitoring with hosted probe regions
- Manage API keys for CI/CD pipelines

## Authentication

Set `KRAKENKEY_API_KEY` in your environment or use the provider `api_key` argument:

```hcl
provider "krakenkey" {
  api_key = var.krakenkey_api_key  # or KRAKENKEY_API_KEY env var
  # api_url = "https://api.krakenkey.io"  # optional; defaults to production
}
```

Create an API key at [app.krakenkey.io](https://app.krakenkey.io) → API Keys. Store it in a secrets manager, not in your Terraform state.

## Quick Example

```hcl
resource "krakenkey_domain" "example" {
  hostname = "example.com"
}

resource "krakenkey_certificate" "example" {
  domain_id = krakenkey_domain.example.id
  key_type  = "ecdsa-p256"  # default
  auto_renew = true
}

output "fullchain_pem" {
  value     = krakenkey_certificate.example.fullchain_pem
  sensitive = true
}
```

See [docs/RESOURCES.md](docs/RESOURCES.md) for the full resource and data source schema.

## Resources

| Resource | Description |
|----------|-------------|
| `krakenkey_domain` | Register a domain for certificate management |
| `krakenkey_certificate` | Issue and renew TLS certificates |
| `krakenkey_endpoint` | Configure HTTPS endpoint monitoring |
| `krakenkey_endpoint_region` | Assign a hosted probe region (Starter tier+) |
| `krakenkey_api_key` | Manage programmatic API keys |

## Data Sources

| Data Source | Description |
|-------------|-------------|
| `data.krakenkey_certificate` | Read certificate metadata and PEM outputs |
| `data.krakenkey_endpoint` | Read current TLS scan status for an endpoint |

## Development

See [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for build, test, and release instructions.

## License

[AGPL-3.0](LICENSE)
