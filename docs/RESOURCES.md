# Resource and Data Source Reference

Full attribute schemas for all resources and data sources in the KrakenKey Terraform provider.

---

## Resources

### `krakenkey_domain`

Registers a domain with KrakenKey and triggers DNS verification.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Fully qualified domain name (e.g., `example.com`) |
| `id` | string | Computed | KrakenKey domain ID |
| `verified` | bool | Computed | Whether DNS verification has passed |
| `created_at` | string | Computed | ISO 8601 creation timestamp |

DNS verification requires two records: a TXT record for ownership and a CNAME to delegate ACME challenges. The provider triggers verification on create; poll the resource until `verified = true`.

---

### `krakenkey_certificate`

Issues and manages a TLS certificate via Let's Encrypt. The CSR is generated client-side; private keys are never sent to the API.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `domain_id` | string | Yes | ID of the verified `krakenkey_domain` |
| `common_name` | string | Yes | Certificate CN (must match or be a subdomain of the domain) |
| `sans` | list(string) | No | Subject Alternative Names |
| `key_type` | string | No | Key type: `EC_P256` (default), `EC_P384`, `RSA2048`, `RSA4096` |
| `auto_renew` | bool | No | Automatically renew before expiry (default: `true`) |
| `id` | string | Computed | KrakenKey certificate ID |
| `status` | string | Computed | Lifecycle status: `pending`, `issuing`, `issued`, `failed`, `revoking`, `revoked` |
| `cert_pem` | string | Computed, Sensitive | Leaf certificate PEM |
| `chain_pem` | string | Computed, Sensitive | Intermediate chain PEM (intermediates only) |
| `full_chain_pem` | string | Computed, Sensitive | Full chain PEM (leaf + intermediates) |
| `expires_at` | string | Computed | Certificate expiry timestamp (ISO 8601) |

`cert_pem`, `chain_pem`, and `full_chain_pem` are populated once `status = "issued"`.

---

### `krakenkey_endpoint`

Creates a monitored TLS endpoint. The probe scans the endpoint on a configurable interval.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `host` | string | Yes | Hostname or IP to monitor |
| `port` | number | Yes | TCP port (typically 443) |
| `label` | string | No | Human-readable label |
| `sni` | string | No | SNI override (defaults to `host`) |
| `is_active` | bool | No | Whether scanning is active (default: `true`) |
| `scan_interval` | number | No | Scan interval in minutes (plan minimum applies) |
| `id` | string | Computed | KrakenKey endpoint ID |
| `created_at` | string | Computed | ISO 8601 creation timestamp |

---

### `krakenkey_endpoint_region`

Adds a hosted probe region to an existing endpoint. Requires Starter tier or higher.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `endpoint_id` | string | Yes | ID of the `krakenkey_endpoint` |
| `region` | string | Yes | Hosted probe region identifier (e.g., `us-east-1`) |
| `id` | string | Computed | Composite ID (`<endpoint_id>/<region>`) |

---

### `krakenkey_api_key`

Creates a KrakenKey API key. The key value is only available at creation time and is not retrievable afterwards.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `name` | string | Yes | Human-readable name for the API key |
| `id` | string | Computed | KrakenKey API key ID |
| `key` | string | Computed, Sensitive | The `kk_...` key value — available only at creation, store in state or secrets manager |
| `created_at` | string | Computed | ISO 8601 creation timestamp |

---

## Data Sources

### `data.krakenkey_certificate`

Looks up an existing certificate by ID.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `id` | string | Yes | KrakenKey certificate ID |
| `status` | string | Computed | Certificate status |
| `common_name` | string | Computed | Certificate CN |
| `sans` | list(string) | Computed | Subject Alternative Names |
| `cert_pem` | string | Computed, Sensitive | Leaf certificate PEM |
| `chain_pem` | string | Computed, Sensitive | Intermediate chain PEM |
| `full_chain_pem` | string | Computed, Sensitive | Full chain PEM |
| `expires_at` | string | Computed | Expiry timestamp |

---

### `data.krakenkey_endpoint`

Looks up an existing monitored endpoint by ID.

| Attribute | Type | Required | Description |
|---|---|---|---|
| `id` | string | Yes | KrakenKey endpoint ID |
| `host` | string | Computed | Monitored hostname or IP |
| `port` | number | Computed | TCP port |
| `label` | string | Computed | Endpoint label |
| `is_active` | bool | Computed | Whether scanning is active |
| `created_at` | string | Computed | ISO 8601 creation timestamp |
