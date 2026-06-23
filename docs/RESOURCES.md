# Resource Reference

Full argument and computed-attribute reference for all KrakenKey Terraform resources and data sources.

---

## Resources

### `krakenkey_domain`

Registers a domain with KrakenKey and provisions ACME DNS-01 credentials.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `fqdn` | string | yes | Fully-qualified domain name (e.g. `example.com`) |

#### Computed Attributes

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey domain UUID |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_domain" "example" {
  fqdn = "example.com"
}

output "domain_id" {
  value = krakenkey_domain.example.id
}
```

---

### `krakenkey_certificate`

Requests a TLS certificate for a registered domain via ACME DNS-01. Automatically renews when fewer than 30 days remain before expiry.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `domain_id` | string | yes | ID of the `krakenkey_domain` resource |
| `sans` | list(string) | no | Additional Subject Alternative Names |
| `auto_renew` | bool | no | Enable automatic renewal (default: `true`) |

#### Computed Attributes

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey certificate UUID |
| `status` | string | `pending`, `active`, `expired`, or `error` |
| `expires_at` | string | ISO 8601 expiry timestamp |
| `chain_pem` | string | Leaf + intermediate chain (excludes root) |
| `fullchain_pem` | string | Leaf + full chain including root |
| `private_key_pem` | string | Private key (sensitive) |

> **Note — LE Merkle Tree Certificates (MTC):** Let's Encrypt announced MTC support on 2026-06-03.
> MTC certificates do not use the traditional X.509 chain.pem / fullchain.pem model.
> `chain_pem` and `fullchain_pem` attributes will require updates before MTC production rollout
> (expected 2027). Staging support is expected in late 2026. Monitor the KrakenKey changelog
> for provider updates before enabling MTC in production.

#### Example

```hcl
resource "krakenkey_certificate" "example" {
  domain_id  = krakenkey_domain.example.id
  sans       = ["www.example.com", "api.example.com"]
  auto_renew = true
}
```

---

### `krakenkey_endpoint`

Attaches a certificate to a named delivery endpoint.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `certificate_id` | string | yes | ID of the `krakenkey_certificate` resource |
| `label` | string | yes | Human-readable endpoint label |

#### Computed Attributes

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey endpoint UUID |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_endpoint" "prod" {
  certificate_id = krakenkey_certificate.example.id
  label          = "prod-nginx"
}
```

---

### `krakenkey_endpoint_region`

Assigns an endpoint to a specific deployment region. Requires Starter tier or above.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `endpoint_id` | string | yes | ID of the `krakenkey_endpoint` resource |
| `region` | string | yes | Deployment region identifier (e.g. `us-east-1`) |

#### Computed Attributes

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey endpoint-region UUID |

#### Example

```hcl
resource "krakenkey_endpoint_region" "us_east" {
  endpoint_id = krakenkey_endpoint.prod.id
  region      = "us-east-1"
}
```

---

### `krakenkey_api_key`

Creates a scoped API key for programmatic access.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `label` | string | yes | Human-readable label |
| `scopes` | list(string) | yes | Permission scopes (e.g. `["certs:read", "domains:write"]`) |
| `expires_at` | string | no | ISO 8601 expiry timestamp; omit for non-expiring key |

#### Computed Attributes

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey API key UUID |
| `key` | string | The raw API key value (sensitive — only returned on creation) |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_api_key" "deploy" {
  label      = "deploy-bot"
  scopes     = ["certs:read"]
  expires_at = "2027-01-01T00:00:00Z"
}

output "deploy_api_key" {
  value     = krakenkey_api_key.deploy.key
  sensitive = true
}
```

---

## Data Sources

### `data.krakenkey_certificate`

Looks up an existing certificate by domain ID.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `domain_id` | string | yes | ID of the domain to look up certificates for |

#### Computed Attributes

Same computed attributes as the `krakenkey_certificate` resource.

#### Example

```hcl
data "krakenkey_certificate" "existing" {
  domain_id = var.domain_id
}
```

---

### `data.krakenkey_domain`

Looks up an existing domain by FQDN.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `fqdn` | string | yes | Fully-qualified domain name to look up |

#### Computed Attributes

Same computed attributes as the `krakenkey_domain` resource.

#### Example

```hcl
data "krakenkey_domain" "existing" {
  fqdn = "example.com"
}
```
