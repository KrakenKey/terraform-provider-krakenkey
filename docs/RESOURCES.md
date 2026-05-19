# KrakenKey Provider — Resource Reference

Full schema reference for all resources and data sources.

---

## `krakenkey_domain`

Registers a domain for certificate management. KrakenKey returns two DNS records to set up: a TXT record for domain ownership and a CNAME to delegate ACME DNS-01 challenges.

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `hostname` | string | Yes | Fully qualified domain name (e.g., `example.com`) |

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Domain UUID |
| `status` | string | `unverified` or `verified` |
| `txt_record_name` | string | DNS TXT record name for ownership verification |
| `txt_record_value` | string | DNS TXT record value |
| `cname_record_name` | string | DNS CNAME name to delegate ACME challenges |
| `cname_record_value` | string | DNS CNAME target |

---

## `krakenkey_certificate`

Issues and manages a TLS certificate. Certificate issuance is asynchronous; the provider waits for completion (default up to 10 minutes).

### Arguments

| Argument | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `domain_id` | string | Yes | — | ID of a `krakenkey_domain` resource |
| `key_type` | string | No | `ecdsa-p256` | Key algorithm: `ecdsa-p256`, `ecdsa-p384`, `rsa-2048`, `rsa-4096` |
| `subject_alt_names` | list(string) | No | `[]` | Additional SANs beyond the primary domain |
| `auto_renew` | bool | No | `true` | Enable automatic server-side renewal |
| `poll_timeout` | string | No | `10m` | How long to wait for issuance (Go duration string) |

### Attributes

| Attribute | Type | Sensitive | Description |
|-----------|------|-----------|-------------|
| `id` | string | No | Certificate UUID |
| `status` | string | No | `pending`, `issuing`, `issued`, `failed` |
| `domain` | string | No | Primary domain (CN) |
| `expires` | string | No | Expiry timestamp (ISO 8601) |
| `fingerprint` | string | No | SHA-256 fingerprint |
| `issuer` | string | No | Issuer DN (e.g., `CN=R11,O=Let's Encrypt`) |
| `serial_number` | string | No | Certificate serial number |
| `cert_pem` | string | Yes | Leaf certificate PEM |
| `chain_pem` | string | Yes | Intermediate CA chain PEM (intermediates only) |
| `fullchain_pem` | string | Yes | Full chain PEM (leaf + intermediates) — use this for most web servers |
| `private_key_pem` | string | Yes | Private key PEM — stored in Terraform state; use a remote backend with encryption at rest |

> **Security note:** `cert_pem`, `chain_pem`, `fullchain_pem`, and `private_key_pem` are marked sensitive and will not appear in plan output. Ensure your Terraform state backend encrypts data at rest.

---

## `krakenkey_endpoint`

Configures a monitored HTTPS endpoint. KrakenKey probes scan the endpoint on a configurable interval and report TLS certificate health, expiry, chain validity, and connection metrics.

### Arguments

| Argument | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `host` | string | Yes | — | Hostname or IP to monitor |
| `port` | number | No | `443` | Port to connect to |
| `sni` | string | No | `host` | SNI override (leave blank to use `host`) |
| `label` | string | No | `""` | Human-readable label for the dashboard |
| `is_active` | bool | No | `true` | Whether the endpoint is actively monitored |

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Endpoint UUID |
| `status` | string | Latest TLS scan status (`valid`, `expiring_soon`, `expired`, `invalid`) |
| `last_scanned_at` | string | ISO 8601 timestamp of the last scan |

---

## `krakenkey_endpoint_region`

Assigns a hosted probe region to a monitored endpoint. Requires **Starter tier or above**.

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `endpoint_id` | string | Yes | ID of a `krakenkey_endpoint` resource |
| `region` | string | Yes | Hosted probe region identifier (e.g., `us-east`, `eu-west`) |

---

## `krakenkey_api_key`

Creates a KrakenKey API key for use in CI/CD pipelines and infrastructure automation.

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `name` | string | Yes | Display name for the key |
| `expires_at` | string | No | Expiry date (ISO 8601); omit for non-expiring |

### Attributes

| Attribute | Type | Sensitive | Description |
|-----------|------|-----------|-------------|
| `id` | string | No | Key UUID |
| `key` | string | Yes | The `kk_...` key value — **only available at creation time**; store in a secrets manager immediately |
| `created_at` | string | No | Creation timestamp |

> **Warning:** The `key` attribute value is returned only once at creation. If you lose it, delete the resource and create a new one.

---

## `data.krakenkey_certificate`

Reads metadata and PEM outputs for an existing certificate.

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | Yes | Certificate UUID |

### Attributes

Same as `krakenkey_certificate` computed attributes (`status`, `domain`, `expires`, `fingerprint`, `cert_pem`, `chain_pem`, `fullchain_pem`).

---

## `data.krakenkey_endpoint`

Reads the current TLS scan status for an existing monitored endpoint.

### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `id` | string | Yes | Endpoint UUID |

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | string | Monitored hostname |
| `port` | number | Port |
| `status` | string | Latest scan status |
| `last_scanned_at` | string | Timestamp of last scan |
