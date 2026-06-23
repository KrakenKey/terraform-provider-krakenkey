# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.io) — automated TLS certificate management via ACME DNS-01.

## Requirements

| Dependency | Minimum version |
|---|---|
| [Terraform](https://developer.hashicorp.com/terraform/downloads) | 1.5 |
| [Go](https://golang.org/doc/install) | 1.22 (for building from source) |

## Authentication

The provider reads your KrakenKey API key from the `KK_API_KEY` environment variable.

```bash
export KK_API_KEY="kkkey_live_..."
```

Alternatively, set it in the provider block (not recommended for production):

```hcl
provider "krakenkey" {
  api_key = "kkkey_live_..."
}
```

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

provider "krakenkey" {}

resource "krakenkey_domain" "example" {
  fqdn = "example.com"
}

resource "krakenkey_certificate" "example" {
  domain_id  = krakenkey_domain.example.id
  sans       = ["www.example.com"]
  auto_renew = true
}

resource "krakenkey_endpoint" "prod" {
  certificate_id = krakenkey_certificate.example.id
  label          = "prod-nginx"
}

resource "krakenkey_endpoint_region" "us_east" {
  endpoint_id = krakenkey_endpoint.prod.id
  region      = "us-east-1"
}

resource "krakenkey_api_key" "deploy" {
  label      = "deploy-bot"
  scopes     = ["certs:read"]
  expires_at = "2027-01-01T00:00:00Z"
}
```

## Resources

| Resource | Description |
|---|---|
| `krakenkey_domain` | Registers a domain and provisions DNS-01 ACME credentials |
| `krakenkey_certificate` | Requests and auto-renews a TLS certificate for a domain |
| `krakenkey_endpoint` | Attaches a certificate to a named delivery endpoint |
| `krakenkey_endpoint_region` | Assigns an endpoint to a deployment region (Starter tier+) |
| `krakenkey_api_key` | Creates a scoped API key |

See [docs/RESOURCES.md](docs/RESOURCES.md) for full argument reference.

## Data Sources

| Data Source | Description |
|---|---|
| `krakenkey_certificate` | Look up an existing certificate by domain ID |
| `krakenkey_domain` | Look up an existing domain by FQDN |

## Local Development Override

To use a locally built provider binary instead of the Terraform Registry version:

1. Build the binary:
   ```bash
   go build -o terraform-provider-krakenkey .
   ```

2. Add a `dev_overrides` block to `~/.terraformrc`:
   ```hcl
   provider_installation {
     dev_overrides {
       "krakenkey/krakenkey" = "/path/to/your/local/build"
     }
     direct {}
   }
   ```

3. Run `terraform plan` or `terraform apply` directly — skip `terraform init` when using dev overrides.

## Related Repositories

| Repo | Description |
|---|---|
| [krakenkey/krakenkey](https://github.com/krakenkey/krakenkey) | Main monorepo (NestJS API, React frontend) |
| [krakenkey/cli](https://github.com/krakenkey/cli) | Go CLI (`kk`) |
| [krakenkey/probe](https://github.com/krakenkey/probe) | Certificate monitoring probe |
| [krakenkey/cert-action](https://github.com/krakenkey/cert-action) | GitHub Actions integration |
