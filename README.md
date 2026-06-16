# terraform-provider-krakenkey

Terraform provider for [KrakenKey](https://krakenkey.com) — automated TLS
certificate management and endpoint monitoring.

> **Status**: Under active development. Not yet published to the Terraform
> Registry. Install from source or via the local dev override below.

## Table of Contents

- [Requirements](#requirements)
- [Authentication](#authentication)
- [Quick Start](#quick-start)
- [Resources](#resources)
- [Data Sources](#data-sources)
- [Local Development Override](#local-development-override)

---

## Requirements

| Tool | Version |
|---|---|
| [Terraform](https://developer.hashicorp.com/terraform/downloads) | ≥ 1.5 |
| [Go](https://golang.org/doc/install) | ≥ 1.22 (to build from source) |

---

## Authentication

The provider authenticates to the KrakenKey API using an API key. You can
supply the key in one of two ways (environment variable is recommended):

### Environment variable (recommended)

```bash
export KK_API_KEY="kkkey_live_..."
```

### Provider block

```hcl
provider "krakenkey" {
  api_key = "kkkey_live_..."  # avoid hard-coding in committed files
}
```

API keys are scoped per workspace and can be rotated from the KrakenKey
dashboard under **Settings → API Keys**.

---

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
  # api_key is read from KK_API_KEY env var
}

# Declare a domain (DNS zone) that KrakenKey will manage
resource "krakenkey_domain" "example" {
  name = "example.com"
}

# Issue a wildcard TLS certificate via ACME DNS-01
resource "krakenkey_certificate" "wildcard" {
  domain_id = krakenkey_domain.example.id
  sans      = ["*.example.com"]
}

# Monitor an HTTPS endpoint
resource "krakenkey_endpoint" "api" {
  url         = "https://api.example.com/health"
  domain_id   = krakenkey_domain.example.id
  cert_id     = krakenkey_certificate.wildcard.id
  check_interval_seconds = 60
}

# Enable multi-region probing (Starter tier+)
resource "krakenkey_endpoint_region" "eu_west" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "eu-west-1"
}

output "cert_expires_at" {
  value = krakenkey_certificate.wildcard.expires_at
}
```

---

## Resources

| Resource | Description |
|---|---|
| [`krakenkey_domain`](docs/RESOURCES.md#krakenkey_domain) | DNS zone registered with KrakenKey |
| [`krakenkey_certificate`](docs/RESOURCES.md#krakenkey_certificate) | ACME TLS certificate (DNS-01 challenge) |
| [`krakenkey_endpoint`](docs/RESOURCES.md#krakenkey_endpoint) | HTTPS endpoint under monitoring |
| [`krakenkey_endpoint_region`](docs/RESOURCES.md#krakenkey_endpoint_region) | Additional probe region for an endpoint (Starter tier+) |
| [`krakenkey_api_key`](docs/RESOURCES.md#krakenkey_api_key) | Programmatic API key for a workspace |

---

## Data Sources

| Data Source | Description |
|---|---|
| [`krakenkey_domain`](docs/RESOURCES.md#data-source-krakenkey_domain) | Look up an existing domain by name |
| [`krakenkey_certificate`](docs/RESOURCES.md#data-source-krakenkey_certificate) | Look up an existing certificate by ID |

---

## Local Development Override

To test a locally built provider binary without publishing to a registry,
add the following to `~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "krakenkey/krakenkey" = "/path/to/terraform-provider-krakenkey"
  }
  direct {}
}
```

Then build the binary into that path:

```bash
go build -o /path/to/terraform-provider-krakenkey .
```

See [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md) for the full development
workflow.
