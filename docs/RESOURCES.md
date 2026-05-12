# Resource Reference

This document describes all resources and data sources provided by `terraform-provider-krakenkey`.

> **Note**: All resources require the `krakenkey` provider to be configured with a valid API key.

---

## Resources

### `krakenkey_domain`

Registers a domain with KrakenKey for TLS certificate management.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `domain` | string | Yes | The fully-qualified domain name (e.g., `example.com`). |

#### Attributes (Read-Only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | KrakenKey domain ID. |
| `verified` | bool | Whether DNS ownership has been verified. |
| `created_at` | string | ISO 8601 timestamp of domain registration. |

#### Example

```hcl
resource "krakenkey_domain" "example" {
  domain = "example.com"
}
```

---

### `krakenkey_certificate`

Issues or renews a TLS certificate for a registered domain using ACME DNS-01 challenge.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `domain_id` | string | Yes | ID of the `krakenkey_domain` resource. |
| `cert_path` | string | No | Local path to write the leaf certificate PEM (`cert.pem`). |
| `chain_path` | string | No | Local path to write the intermediate CA chain PEM (`chain.pem`). |
| `fullchain_path` | string | No | Local path to write the full chain PEM (`fullchain.pem`, leaf + intermediates). |

#### Attributes (Read-Only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | KrakenKey certificate ID. |
| `cert_pem` | string | Leaf certificate PEM (sensitive). |
| `chain_pem` | string | Intermediate CA chain PEM (sensitive). |
| `fullchain_pem` | string | Full chain PEM — leaf + intermediates (sensitive). |
| `expiry` | string | ISO 8601 certificate expiration timestamp. |
| `days_until_expiry` | number | Days remaining until certificate expiry. |
| `issuer` | string | Common name of the issuing CA. |

#### Notes

- `cert_pem`, `chain_pem`, and `fullchain_pem` are marked sensitive in the Terraform state.
- Use `fullchain_pem` for nginx and HAProxy (requires intermediate chain). Use `cert_pem` alone for Apache when the chain is specified separately.
- Certificates auto-renew approximately 30 days before expiry when using the KrakenKey managed ACME flow.

#### Example

```hcl
resource "krakenkey_certificate" "example" {
  domain_id      = krakenkey_domain.example.id
  cert_path      = "/etc/ssl/example/cert.pem"
  chain_path     = "/etc/ssl/example/chain.pem"
  fullchain_path = "/etc/ssl/example/fullchain.pem"
}

# Use the fullchain output directly in another resource
resource "aws_acm_certificate_import" "example" {
  certificate_body  = krakenkey_certificate.example.cert_pem
  certificate_chain = krakenkey_certificate.example.chain_pem
  private_key       = var.private_key_pem
}
```

---

### `krakenkey_endpoint`

Configures an HTTPS endpoint for TLS certificate monitoring.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `domain_id` | string | Yes | ID of the associated `krakenkey_domain`. |
| `url` | string | Yes | Full HTTPS URL of the endpoint to monitor (e.g., `https://example.com`). |
| `name` | string | Yes | Display name for this endpoint. |
| `port` | number | No | TCP port for TLS checks. Defaults to `443`. |

#### Attributes (Read-Only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | KrakenKey endpoint ID. |
| `status` | string | Current status: `healthy`, `warning`, or `critical`. |
| `last_checked_at` | string | ISO 8601 timestamp of the last probe check. |

---

### `krakenkey_endpoint_region`

Assigns a hosted probe region to an endpoint. Requires **Starter tier or above**.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `endpoint_id` | string | Yes | ID of the `krakenkey_endpoint` to attach the region to. |
| `region` | string | Yes | Hosted probe region identifier (e.g., `us-east-1`, `eu-west-1`). |

#### Attributes (Read-Only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Composite ID: `{endpoint_id}/{region}`. |

---

### `krakenkey_api_key`

Manages API keys for use in CI/CD pipelines and automation.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Display name for this API key. |
| `scopes` | list(string) | No | Permission scopes. Defaults to read-only. |

#### Attributes (Read-Only)

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | API key ID. |
| `key` | string | The API key value (sensitive, only available at creation time). |
| `created_at` | string | ISO 8601 creation timestamp. |

> **Warning**: `key` is only populated when the resource is first created. It cannot be retrieved again — store it in a secrets manager immediately.

---

## Data Sources

### `data.krakenkey_certificate`

Reads metadata and PEM outputs for an existing certificate.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | Yes | Certificate ID to look up. |

#### Attributes

Same read-only attributes as the `krakenkey_certificate` resource.

---

### `data.krakenkey_endpoint`

Reads current TLS status for an existing monitored endpoint.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | Yes | Endpoint ID to look up. |

#### Attributes

Same read-only attributes as the `krakenkey_endpoint` resource.
