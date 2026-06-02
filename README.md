# Terraform Provider for KrakenKey

The KrakenKey Terraform provider manages TLS certificates, domains, endpoint monitoring, and API keys through the [KrakenKey](https://krakenkey.io) platform.

[![Terraform Registry](https://img.shields.io/badge/registry-krakenkey%2Fkrakenkey-7B42BC.svg)](https://registry.terraform.io/providers/krakenkey/krakenkey)
[![Go Reference](https://pkg.go.dev/badge/github.com/krakenkey/terraform-provider-krakenkey.svg)](https://pkg.go.dev/github.com/krakenkey/terraform-provider-krakenkey)

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) v1.5+
- Go 1.21+ (to build from source)
- A KrakenKey account and API key

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
  # Set via KRAKENKEY_API_KEY environment variable, or:
  # api_key = var.krakenkey_api_key
}

resource "krakenkey_domain" "example" {
  name = "example.com"
}

resource "krakenkey_certificate" "example" {
  domain_id  = krakenkey_domain.example.id
  key_type   = "ecdsa-p256"
  auto_renew = true

  lifecycle {
    ignore_changes = [cert_pem, chain_pem, fullchain_pem, private_key_pem]
  }
}

output "fullchain_pem" {
  value     = krakenkey_certificate.example.fullchain_pem
  sensitive = true
}
```

## Authentication

Set the `KRAKENKEY_API_KEY` environment variable or the `api_key` provider argument:

```bash
export KRAKENKEY_API_KEY="kk_your_api_key"
```

API keys are created in the KrakenKey dashboard or with `krakenkey auth keys create`.

## Resources

| Resource | Description |
|---|---|
| `krakenkey_domain` | Register a domain for certificate management |
| `krakenkey_certificate` | Issue and renew TLS certificates |
| `krakenkey_endpoint` | Configure HTTPS endpoint monitoring |
| `krakenkey_endpoint_region` | Assign a hosted probe region to an endpoint (Starter tier+) |
| `krakenkey_api_key` | Create and manage API keys for CI/CD automation |

## Data Sources

| Data Source | Description |
|---|---|
| `data.krakenkey_certificate` | Read an existing certificate's metadata and PEM outputs |
| `data.krakenkey_endpoint` | Read the current TLS scan status for an existing endpoint |

See [`docs/RESOURCES.md`](docs/RESOURCES.md) for full schema documentation.

## Development

See [`docs/CONTRIBUTING.md`](docs/CONTRIBUTING.md) for build, test, and release instructions.

## Related Repositories

| Repo | Description |
|---|---|
| [krakenkey/krakenkey](https://github.com/krakenkey/krakenkey) | Main monorepo (devcontainer, docs, orchestration) |
| [krakenkey/app](https://github.com/krakenkey/app) | NestJS API + React dashboard |
| [krakenkey/cli](https://github.com/krakenkey/cli) | CLI tool (Go) |

## License

[Mozilla Public License 2.0](LICENSE)
