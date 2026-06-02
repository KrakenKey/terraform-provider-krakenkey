# Resource Reference

Full schema documentation for all KrakenKey Terraform resources and data sources.

---

## Resources

### `krakenkey_domain`

Registers a domain with KrakenKey. KrakenKey returns two DNS records that must be configured before the domain can be used for certificate issuance.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Domain name (e.g. `example.com`) |

#### Attributes (read-only)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey domain UUID |
| `status` | string | Verification status: `pending`, `verified`, `failed` |
| `txt_record_name` | string | TXT record name to create for ownership proof |
| `txt_record_value` | string | TXT record value |
| `cname_record_name` | string | CNAME record name to delegate ACME DNS-01 challenges |
| `cname_record_value` | string | CNAME record value (KrakenKey ACME delegator) |

---

### `krakenkey_certificate`

Issues and manages a TLS certificate for a registered domain. The private key is generated client-side and never leaves the provider.

#### Arguments

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `domain_id` | string | Yes | | KrakenKey domain UUID |
| `key_type` | string | No | `ecdsa-p256` | Key type: `ecdsa-p256`, `ecdsa-p384`, `rsa-2048`, `rsa-4096` |
| `auto_renew` | bool | No | `true` | Automatically renew before expiry |
| `sans` | list(string) | No | `[]` | Additional Subject Alternative Names |

#### Attributes (read-only, sensitive)

| Attribute | Type | Sensitive | Description |
|---|---|---|---|
| `id` | string | No | KrakenKey certificate UUID |
| `status` | string | No | Certificate state: `pending`, `issuing`, `issued`, `failed`, `revoked` |
| `cert_pem` | string | Yes | Leaf certificate PEM |
| `chain_pem` | string | Yes | Intermediate CA chain PEM (without leaf) |
| `fullchain_pem` | string | Yes | Leaf + intermediate chain PEM (use for nginx `ssl_certificate`) |
| `private_key_pem` | string | Yes | Private key PEM |
| `expires_at` | string | No | Certificate expiry (RFC 3339) |

> **Note:** `cert_pem`, `chain_pem`, `fullchain_pem`, and `private_key_pem` are marked sensitive and will not appear in plan output.

---

### `krakenkey_endpoint`

Configures an HTTPS endpoint for TLS health monitoring.

#### Arguments

| Argument | Type | Required | Default | Description |
|---|---|---|---|---|
| `host` | string | Yes | | Hostname to monitor |
| `port` | number | No | `443` | Port to connect on |
| `sni` | string | No | same as `host` | SNI hostname for TLS handshake |
| `label` | string | No | `""` | Human-readable label |
| `is_active` | bool | No | `true` | Enable or disable monitoring |
| `scan_interval` | number | No | plan minimum | Scan interval in minutes |

#### Attributes (read-only)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | KrakenKey endpoint UUID |
| `status` | string | Latest TLS status from most recent scan |

---

### `krakenkey_endpoint_region`

Assigns a KrakenKey-hosted probe region to an endpoint. Requires **Starter tier or above**.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `endpoint_id` | string | Yes | KrakenKey endpoint UUID |
| `region` | string | Yes | Region identifier (e.g. `us-east-1`, `eu-west-1`) |

#### Attributes (read-only)

| Attribute | Type | Description |
|---|---|---|
| `id` | string | Composite ID (`<endpoint_id>/<region>`) |

---

### `krakenkey_api_key`

Creates an API key for use in CI/CD pipelines or automation. The key value is only available at creation time.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Human-readable label for the key |

#### Attributes (read-only)

| Attribute | Type | Sensitive | Description |
|---|---|---|---|
| `id` | string | No | KrakenKey API key UUID |
| `key` | string | Yes | API key value (`kk_...`). **Only returned at creation time.** Store securely immediately. |
| `created_at` | string | No | Creation timestamp (RFC 3339) |

> **Warning:** The `key` attribute is only populated when the resource is first created. After the initial `apply`, Terraform will show the value as known but it cannot be retrieved from the API again. Store it in a secrets manager (e.g., AWS Secrets Manager, HashiCorp Vault) on first apply.

---

## Data Sources

### `data.krakenkey_certificate`

Reads metadata and PEM outputs for an existing KrakenKey certificate.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `id` | string | Yes | KrakenKey certificate UUID |

#### Attributes

Same as the `krakenkey_certificate` resource attributes.

---

### `data.krakenkey_endpoint`

Reads the current configuration and latest TLS scan status for an existing endpoint.

#### Arguments

| Argument | Type | Required | Description |
|---|---|---|---|
| `id` | string | Yes | KrakenKey endpoint UUID |

#### Attributes

Same as the `krakenkey_endpoint` resource attributes, plus `latest_scan_at` (RFC 3339 timestamp of most recent scan).
