# Resource and Data Source Reference

> **Design proposal — not implemented.** No provider code exists in this repository yet. This document defines the intended schema so it can be reviewed before implementation; names, types and defaults may change. See the status notice in [README.md](README.md).

## Resources

### `krakenkey_domain`

Register a domain with KrakenKey and track DNS verification status.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | yes | Domain name (e.g. `example.com`) |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Domain ID |
| `verified` | bool | Whether DNS ownership is verified |
| `txt_record_name` | string | TXT record name for domain ownership verification |
| `txt_record_value` | string | TXT record value for domain ownership verification |
| `cname_record_name` | string | CNAME record name for ACME DNS-01 delegation |
| `cname_record_value` | string | CNAME record value for ACME DNS-01 delegation |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_domain" "example" {
  name = "example.com"
}

output "verification_txt" {
  value = krakenkey_domain.example.txt_record_value
}

# The ACME delegation record, created with your DNS provider.
# krakenkey_certificate must depend on this — see the note there.
resource "cloudflare_record" "acme_delegation" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.cname_record_name
  type    = "CNAME"
  content = krakenkey_domain.example.cname_record_value
  proxied = false
}
```

---

### `krakenkey_certificate`

Issue and manage a TLS certificate via Let's Encrypt ACME DNS-01.

> **Ordering matters — the API now rejects issuance before the CNAME exists.** KrakenKey verifies the `_acme-challenge` delegation *before* creating an ACME order, and a missing or misdirected CNAME is a permanent failure, not a retry. In Terraform that record is usually created by a different provider (`cloudflare_record`, `aws_route53_record`) from `krakenkey_domain`'s `cname_record_name` / `cname_record_value`. Terraform has no way to infer that `krakenkey_certificate` depends on that record, so without an explicit `depends_on` it may create the certificate first and fail the apply outright. Previously the same ordering bug surfaced as a slow timeout; now it fails immediately.
>
> The implementation should either make this ordering explicit in the documented examples (as below) or have the resource retry on the delegation error rather than failing the apply. Worth deciding before the resource is built.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `domain_id` | string | yes | ID of the `krakenkey_domain` to issue for |
| `common_name` | string | yes | Certificate common name |
| `san` | list(string) | no | Subject alternative names |
| `key_type` | string | no | Key type: `EC_P256` (default), `EC_P384`, `RSA_2048`, `RSA_4096` |
| `auto_renew` | bool | no | Enable automatic renewal (default: `true`) |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Certificate ID |
| `status` | string | Certificate status: `pending`, `issuing`, `issued`, `failed`, `revoked` |
| `cert_pem` | string | Leaf certificate PEM (sensitive) |
| `chain_pem` | string | Intermediate CA chain PEM (sensitive) |
| `fullchain_pem` | string | Leaf + intermediates PEM (sensitive) |
| `expires_at` | string | ISO 8601 expiry timestamp |
| `fingerprint` | string | SHA-256 fingerprint |

> **Note — LE Merkle Tree Certificates (2027)**: Let's Encrypt announced MTC on 2026-06-03. MTC breaks the traditional `chain_pem`/`fullchain_pem` model. Both attributes will be empty for MTC certificates pending provider updates before the LE production rollout (target: 2027).

> **Note — deploy `fullchain_pem`, not `cert_pem`**: write `fullchain_pem` wherever a server expects a certificate file. Clients split on whether they will repair an incomplete chain by fetching the issuing intermediate from the leaf's AIA `caIssuers` URL — Windows Schannel, macOS Security.framework and Chrome do; OpenSSL, Go, Firefox and Java PKIX (by default) do not — so a `cert_pem`-only deployment can pass a browser check and fail in `curl` or a Go service. CA/Browser Forum ballot SC104 (passed 2026-09-03) relaxed AIA from MUST to SHOULD in subscriber certificates, so leaves may eventually carry no `caIssuers` URL at all and chain repair becomes unavailable everywhere.

All three PEM attributes are `Sensitive: true`, so they are redacted in plan output but stored in plain text in Terraform state. Use a remote backend with encryption at rest, or pass the certificate to its destination out of band rather than through state.

#### Example

```hcl
resource "krakenkey_certificate" "web" {
  domain_id   = krakenkey_domain.example.id
  common_name = "example.com"
  san         = ["www.example.com", "api.example.com"]
  key_type    = "EC_P256"
  auto_renew  = true
}
```

---

### `krakenkey_endpoint`

Monitor a TLS endpoint for certificate expiry, chain validity, and connection health.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `host` | string | yes | Hostname to monitor |
| `port` | number | no | Port (default: `443`) |
| `label` | string | no | Human-readable label |
| `sni` | string | no | SNI override (defaults to `host`) |
| `is_active` | bool | no | Whether scanning is active (default: `true`) |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Endpoint ID |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_endpoint" "api" {
  host  = "api.example.com"
  port  = 443
  label = "Production API"
}
```

---

### `krakenkey_endpoint_region`

Add a hosted probe region to an endpoint. Requires Starter tier or higher.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `endpoint_id` | string | yes | ID of the `krakenkey_endpoint` |
| `region` | string | yes | Region identifier (e.g. `us-east-1`, `eu-west-1`) |

#### Example

```hcl
resource "krakenkey_endpoint_region" "us" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "us-east-1"
}

resource "krakenkey_endpoint_region" "eu" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "eu-west-1"
}
```

---

### `krakenkey_api_key`

Create an API key. The key value (`token`) is only available at creation time and is stored in state as sensitive.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | yes | Display name for the key |

#### Computed Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Key ID |
| `token` | string | `kk_...` token value (sensitive; only available at creation) |
| `created_at` | string | ISO 8601 creation timestamp |

#### Example

```hcl
resource "krakenkey_api_key" "ci" {
  name = "ci-pipeline"
}

output "ci_api_key" {
  value     = krakenkey_api_key.ci.token
  sensitive = true
}
```

---

## Data Sources

### `data.krakenkey_certificate`

Look up a certificate by ID.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | yes | Certificate ID |

#### Attributes

Same computed attributes as the `krakenkey_certificate` resource.

#### Example

```hcl
data "krakenkey_certificate" "existing" {
  id = "cert_abc123"
}
```

---

### `data.krakenkey_endpoint`

Look up an endpoint by ID.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | yes | Endpoint ID |

#### Attributes

Same computed attributes as the `krakenkey_endpoint` resource.

#### Example

```hcl
data "krakenkey_endpoint" "existing" {
  id = "ep_abc123"
}
```
