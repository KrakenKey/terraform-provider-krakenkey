# Resource Reference

Full schema documentation for all KrakenKey Terraform resources and data sources.

---

## Resources

### `krakenkey_domain`

Represents a DNS zone registered with KrakenKey. Domains are the top-level
grouping for certificates and endpoints.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Fully-qualified domain name (e.g. `example.com`). Immutable after creation. |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey domain UUID |
| `created_at` | string | RFC 3339 creation timestamp |
| `verified` | bool | Whether DNS ownership has been verified |

#### Example

```hcl
resource "krakenkey_domain" "example" {
  name = "example.com"
}
```

---

### `krakenkey_certificate`

Requests an ACME TLS certificate using the DNS-01 challenge. KrakenKey
automatically provisions the required `_acme-challenge` TXT records and
renews the certificate before expiry.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `domain_id` | string | yes | ID of the `krakenkey_domain` this certificate belongs to |
| `sans` | list(string) | no | Additional Subject Alternative Names. The apex domain is always included. Use `["*.example.com"]` for a wildcard. |
| `ca` | string | no | Certificate Authority. Defaults to `letsencrypt`. Allowed values: `letsencrypt`, `letsencrypt_staging`. |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey certificate UUID |
| `status` | string | `pending`, `active`, `expired`, `error` |
| `expires_at` | string | RFC 3339 expiry timestamp |
| `serial` | string | Certificate serial number |
| `chain_pem` | string | Leaf + intermediates (no root), PEM-encoded. Sensitive. |
| `full_chain_pem` | string | Leaf + intermediates + root, PEM-encoded. Sensitive. |

> **Note — LE Merkle Tree Certificates (MTC):** Let's Encrypt announced MTC
> support on 2026-06-03 (staging late 2026, production 2027). MTC replaces
> the X.509 chain model; `chain_pem` and `full_chain_pem` will not apply to
> MTC issuances. Provider support for MTC will be added ahead of the
> production rollout. Track progress in the issue tracker.

#### Example

```hcl
resource "krakenkey_certificate" "wildcard" {
  domain_id = krakenkey_domain.example.id
  sans      = ["*.example.com"]
}
```

---

### `krakenkey_endpoint`

Configures an HTTPS endpoint for continuous monitoring. KrakenKey probes
the URL from one or more regions and alerts on downtime or certificate issues.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `url` | string | yes | Full HTTPS URL to probe (e.g. `https://api.example.com/health`) |
| `domain_id` | string | yes | ID of the parent `krakenkey_domain` |
| `cert_id` | string | no | ID of a `krakenkey_certificate` to associate for expiry tracking |
| `check_interval_seconds` | number | no | Probe frequency in seconds. Default: `60`. Minimum: `30`. |
| `alert_threshold_seconds` | number | no | Seconds of consecutive downtime before alerting. Default: `120`. |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey endpoint UUID |
| `status` | string | `up`, `down`, `unknown` |
| `last_checked_at` | string | RFC 3339 timestamp of most recent probe |

#### Example

```hcl
resource "krakenkey_endpoint" "api" {
  url                    = "https://api.example.com/health"
  domain_id              = krakenkey_domain.example.id
  cert_id                = krakenkey_certificate.wildcard.id
  check_interval_seconds = 60
}
```

---

### `krakenkey_endpoint_region`

Enables probing from an additional geographic region for a given endpoint.
Requires Starter tier or above.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `endpoint_id` | string | yes | ID of the parent `krakenkey_endpoint` |
| `region` | string | yes | Region identifier. See the KrakenKey dashboard for available regions (e.g. `eu-west-1`, `ap-southeast-1`). |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | Composite ID (`endpoint_id/region`) |

#### Example

```hcl
resource "krakenkey_endpoint_region" "eu_west" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "eu-west-1"
}
```

---

### `krakenkey_api_key`

Creates a programmatic API key scoped to the authenticated workspace.
Store the resulting `secret` in a secrets manager — it is only returned
on creation and cannot be retrieved again.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Human-readable label for the key |
| `scopes` | list(string) | no | Permission scopes. Defaults to `["read"]`. Valid values: `read`, `write`, `admin`. |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey API key UUID |
| `prefix` | string | Public prefix of the key (e.g. `kkkey_live_abc123`) |
| `secret` | string | Full API key value. **Sensitive.** Shown only at creation. |
| `created_at` | string | RFC 3339 creation timestamp |

#### Example

```hcl
resource "krakenkey_api_key" "ci" {
  name   = "GitHub Actions"
  scopes = ["read", "write"]
}

output "ci_api_key" {
  value     = krakenkey_api_key.ci.secret
  sensitive = true
}
```

---

## Data Sources

### Data source: `krakenkey_domain`

Look up an existing domain by its fully-qualified name.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Fully-qualified domain name to look up |

#### Attributes (computed)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey domain UUID |
| `verified` | bool | Whether DNS ownership has been verified |
| `created_at` | string | RFC 3339 creation timestamp |

#### Example

```hcl
data "krakenkey_domain" "existing" {
  name = "example.com"
}

resource "krakenkey_endpoint" "api" {
  url       = "https://api.example.com/health"
  domain_id = data.krakenkey_domain.existing.id
}
```

---

### Data source: `krakenkey_certificate`

Look up an existing certificate by its ID.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `id` | string | yes | KrakenKey certificate UUID |

#### Attributes (computed)

Same computed attributes as the [`krakenkey_certificate` resource](#attributes-computed-1),
except `chain_pem` and `full_chain_pem` are always populated (the certificate
already exists).

#### Example

```hcl
data "krakenkey_certificate" "existing" {
  id = var.existing_cert_id
}

output "expires_at" {
  value = data.krakenkey_certificate.existing.expires_at
}
```
