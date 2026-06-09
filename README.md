# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.io) — automated TLS certificate management and endpoint monitoring.

> **Status**: Under development. Not yet published to the Terraform Registry.

## Quick Start

```hcl
terraform {
  required_providers {
    krakenkey = {
      source  = "krakenkey/krakenkey"
      version = "~> 0.1"
    }
  }
}

provider "krakenkey" {
  # api_key = var.krakenkey_api_key  # or set KK_API_KEY env var
}

resource "krakenkey_domain" "example" {
  name = "example.com"
}

resource "krakenkey_certificate" "example" {
  domain_id    = krakenkey_domain.example.id
  common_name  = "example.com"
  sans         = ["www.example.com"]
  key_type     = "EC_P256"
  auto_renew   = true
}

resource "krakenkey_endpoint" "example" {
  host  = "example.com"
  port  = 443
  label = "Production"
}
```

## Authentication

The provider authenticates using a KrakenKey API key (`kk_` prefix). Provide it via:

- **Environment variable** (recommended): `export KK_API_KEY=kk_...`
- **Provider block**: `api_key = var.krakenkey_api_key` — use a variable or secret store; never hardcode.

## Resources

| Resource | Description |
|---|---|
| `krakenkey_domain` | Register and verify a domain for certificate issuance |
| `krakenkey_certificate` | Issue and manage a TLS certificate via Let's Encrypt |
| `krakenkey_endpoint` | Create a monitored TLS endpoint |
| `krakenkey_endpoint_region` | Add a hosted probe region to an endpoint (Starter tier+) |
| `krakenkey_api_key` | Create a KrakenKey API key |

## Data Sources

| Data Source | Description |
|---|---|
| `data.krakenkey_certificate` | Look up an existing certificate by ID |
| `data.krakenkey_endpoint` | Look up an existing monitored endpoint by ID |

See [docs/RESOURCES.md](docs/RESOURCES.md) for full attribute schemas.

## Development

See [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for build, test, and release instructions.

## License

[GNU Affero General Public License v3.0](https://github.com/krakenkey/krakenkey/blob/main/LICENSE)
