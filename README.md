# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.io) — automated TLS certificate management and endpoint monitoring.

> ### ⚠️ Status: planned interface, not yet implemented
>
> This repository currently contains **documentation only** — there is no Go source, no `go.mod`, and no released binary. Everything below describes the *intended* provider interface so that the API surface can be reviewed and agreed before implementation starts.
>
> Concretely, right now:
>
> - The provider is **not** on the Terraform Registry, so `terraform init` cannot resolve `krakenkey/krakenkey`.
> - The build commands in this README and in [docs/CONTRIBUTING.md](CONTRIBUTING.md) will not work — there is nothing to build yet.
> - Resource and attribute names in [docs/RESOURCES.md](DESIGN.md) are a **design proposal**, not a compatibility promise. Expect them to change during implementation.
>
> Until this notice is removed, manage KrakenKey resources with the [CLI](https://github.com/krakenkey/cli), the [GitHub Action](https://github.com/krakenkey/cert-action), or the REST API directly.

## Requirements

| Dependency | Version |
|------------|---------|
| [Terraform](https://developer.hashicorp.com/terraform) | >= 1.5 |
| [Go](https://golang.org/) (to build from source) | >= 1.22 |

## Authentication

The provider authenticates with the KrakenKey API using an API key. The recommended approach is the environment variable:

```bash
export KK_API_KEY=kk_...
```

Or configure it in the provider block:

```hcl
provider "krakenkey" {
  api_key = "kk_..."  # prefer KK_API_KEY env var
}
```

## Quick Start

```hcl
terraform {
  required_providers {
    krakenkey = {
      source = "krakenkey/krakenkey"
    }
  }
}

provider "krakenkey" {}

# Register a domain
resource "krakenkey_domain" "example" {
  name = "example.com"
}

# Issue a certificate
resource "krakenkey_certificate" "example" {
  domain_id   = krakenkey_domain.example.id
  common_name = "example.com"
  san         = ["www.example.com"]
  key_type    = "EC_P256"
}

# Monitor an endpoint
resource "krakenkey_endpoint" "example" {
  host  = "example.com"
  port  = 443
  label = "Main site"
}

# Add a hosted probe region (Starter tier+)
resource "krakenkey_endpoint_region" "us_east" {
  endpoint_id = krakenkey_endpoint.example.id
  region      = "us-east-1"
}
```

## Resources and Data Sources

See [docs/RESOURCES.md](DESIGN.md) for full argument and attribute reference.

**Resources**

| Resource | Description |
|----------|-------------|
| `krakenkey_domain` | Register and verify a domain |
| `krakenkey_certificate` | Issue and manage TLS certificates |
| `krakenkey_endpoint` | Monitor a TLS endpoint |
| `krakenkey_endpoint_region` | Add a hosted probe region to an endpoint (Starter tier+) |
| `krakenkey_api_key` | Create and manage API keys |

**Data Sources**

| Data Source | Description |
|-------------|-------------|
| `data.krakenkey_certificate` | Look up a certificate by ID |
| `data.krakenkey_endpoint` | Look up an endpoint by ID |

## Local Development

```bash
# Build the provider binary
go build -o terraform-provider-krakenkey ./...

# Configure Terraform to use the local binary
cat >> ~/.terraformrc <<'EOF'
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/terraform-provider-krakenkey"
  }
  direct {}
}
EOF
```

See [docs/CONTRIBUTING.md](CONTRIBUTING.md) for full build, test, and release instructions.

## Related Repositories

| Repo | Description |
|------|-------------|
| [krakenkey/krakenkey](https://github.com/krakenkey/krakenkey) | Monorepo (docs, devcontainer) |
| [krakenkey/app](https://github.com/krakenkey/app) | NestJS API + React dashboard |
| [krakenkey/cli](https://github.com/krakenkey/cli) | CLI tool (Go) |
| [krakenkey/probe](https://github.com/krakenkey/probe) | TLS health probe (Go) |

## License

This project is part of [KrakenKey](https://github.com/krakenkey/krakenkey), licensed under the [GNU Affero General Public License v3.0](https://github.com/krakenkey/krakenkey/blob/main/LICENSE).
