# Provider design proposal

> **Not implemented.** No provider code exists yet. This is a proposed schema for review before implementation. Everything here can change. See the status notice in [README.md](README.md).

Each resource below maps to endpoints in the public KrakenKey API (`https://api.krakenkey.io/swagger-json`, rendered at <https://krakenkey.io/docs/api/>). The design only proposes attributes that the API can back. Where the API needs a decision from the provider, it is listed under [Open design questions](#open-design-questions).

## Provider configuration

| Argument | Type | Required | Env var | Description |
|----------|------|----------|---------|-------------|
| `api_key` | string, sensitive | yes, unless the env var is set | `KK_API_KEY` | User API key (`kk_...`) |
| `api_url` | string | no | `KK_API_URL` | API base URL. Default `https://api.krakenkey.io` |

The env var names match the KrakenKey CLI. Requests use `Authorization: Bearer <api_key>`.

API keys cannot manage API keys: `POST /auth/api-keys` and `DELETE /auth/api-keys/:id` accept dashboard sessions only. A `krakenkey_api_key` resource is therefore not possible with API key auth and is left out.

## Setup order

KrakenKey checks two DNS records, and Terraform cannot see either dependency because it runs through DNS rather than the resource graph:

1. Create `krakenkey_domain` (`POST /domains`). This returns the verification code.
2. Publish the TXT record (`txt_record_name` / `txt_record_value`) with your DNS provider.
3. Verify the domain (`POST /domains/:id/verify`). This reads the TXT record and fails if it has not propagated. KrakenKey re-checks it daily, so the record must stay in place.
4. Publish the `_acme-challenge` CNAME (`cname_record_name` / `cname_record_value`).
5. Create `krakenkey_certificate` (`POST /certs/tls`).

Step 5 fails in two ways if the order is wrong:

- If any name in the CSR is not on a verified domain, `POST /certs/tls` returns 400 and nothing is created.
- If the `_acme-challenge` CNAME is missing or points somewhere else, the request is accepted, but the issuance job checks the delegation before it creates an ACME order. It then marks the certificate `failed` with a `failureReason` starting `ACME challenge delegation missing` or `ACME challenge delegation mismatch`. That failure is treated as permanent and is not retried. Before KrakenKey/app#108 (merged 2026-09-24) the same mistake only showed up after Let's Encrypt validation failed.

Examples should carry an explicit `depends_on` on the DNS records until the provider handles this itself.

## Resources

### `krakenkey_domain`

Registers a domain. API: `POST /domains`, `GET /domains/:id`, `DELETE /domains/:id`.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `hostname` | string | yes | Domain name, e.g. `example.com`. Changing it forces a new resource. |

#### Attributes

| Attribute | Type | Source | Description |
|-----------|------|--------|-------------|
| `id` | string | API | Domain UUID |
| `verified` | bool | API `isVerified` | Whether ownership has been verified |
| `verification_code` | string | API `verificationCode` | Full TXT value, `krakenkey-site-verification=<hex>` |
| `txt_record_name` | string | provider | Same as `hostname`; the TXT record goes on the hostname itself |
| `txt_record_value` | string | provider | Same as `verification_code` |
| `cname_record_name` | string | provider | `_acme-challenge.<hostname>` |
| `cname_record_value` | string | provider | `<hostname with dots replaced by dashes>.acme.krakenkey.io`, e.g. `example-com.acme.krakenkey.io` |
| `created_at` | string | API | RFC 3339 timestamp |

The API does not return the record names or the CNAME target. The provider would derive them. The `acme.krakenkey.io` zone is a server setting and differs on non-production API URLs, so the implementation needs a way to get the right zone when `api_url` is not the default.

Import: by domain UUID.

```hcl
resource "krakenkey_domain" "example" {
  hostname = "example.com"
}
```

---

### `krakenkey_certificate`

Submits a CSR and waits for issuance. API: `POST /certs/tls`, `GET /certs/tls/:id`, `GET /certs/tls/:id/chain`, `PATCH /certs/tls/:id`, `POST /certs/tls/:id/revoke`, `DELETE /certs/tls/:id`.

KrakenKey takes a CSR, not key parameters, and never sees the private key. The names on the certificate come from the CSR, and every one of them must be on a verified domain in the account. There is no `domain_id` argument because the API has no such field.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `csr_pem` | string | yes | PEM CSR, e.g. from `tls_cert_request`. Changing it forces a new resource. |
| `auto_renew` | bool | no | Server-side automatic renewal. Default `true` (the API default). Updated in place with `PATCH`. |

#### Attributes

| Attribute | Type | Source | Description |
|-----------|------|--------|-------------|
| `id` | string | API | Certificate ID. The API uses a number; the provider would store it as a string. |
| `status` | string | API | `pending`, `issuing`, `issued`, `failed`, `renewing`, `revoking` or `revoked` |
| `failure_reason` | string | API `failureReason` | Why the last issuance or renewal failed, if it did |
| `cert_pem` | string | API `crtPem` | Leaf certificate |
| `chain_pem` | string | API `chainPem` | Intermediates only |
| `fullchain_pem` | string | `GET .../chain` `fullChainPem` | Leaf plus intermediates. This is what most servers (nginx, Caddy, HAProxy) want. |
| `expires_at` | string | API `expiresAt` | RFC 3339 expiry |
| `renewal_count` | number | API `renewalCount` | Number of renewals so far |

Create posts the CSR, then polls `GET /certs/tls/:id` until the status is `issued` (success) or `failed` (error that includes `failure_reason`). Issuance usually takes 2 to 5 minutes, so the resource needs a configurable create timeout. Submitting the same CSR again within 15 minutes returns the existing certificate instead of a new one (or 409 while the first request is still being created, which is safe to retry). That makes a retried create safe.

When auto-renew is on, KrakenKey renews on the server and the PEM and expiry attributes change. Those are computed values, so the next refresh picks them up without showing a diff on any argument.

Certificates are public (all Let's Encrypt certificates are logged to Certificate Transparency), so the PEM attributes do not need `Sensitive: true`. The secret is the private key behind the CSR. If you create it with `tls_private_key`, it is stored in plain text in Terraform state. Use a state backend with encryption at rest, or generate the key and CSR outside Terraform and pass in only `csr_pem`.

Deploy `fullchain_pem`, not `cert_pem`. Some clients fetch a missing intermediate from the leaf's AIA `caIssuers` URL (Windows, macOS and Chrome do) and some do not (OpenSSL, Go, Firefox, and Java by default). A leaf-only deployment can pass a browser check and still fail in `curl` or a Go service. CA/Browser Forum ballot SC104 (passed 2026-09-03) makes the AIA extension optional in subscriber certificates, so that fallback may disappear.

Let's Encrypt plans to move to Merkle Tree Certificates (staging late 2026, production 2027). MTC does not use the `chain_pem` / `fullchain_pem` model, so these attributes will need a compatibility review before then.

Import: by certificate ID. `csr_pem` cannot be read back exactly as written, so imported resources would need it set in configuration with changes ignored, or the provider would need to compare CSRs by content.

```hcl
resource "krakenkey_certificate" "web" {
  csr_pem    = tls_cert_request.web.cert_request_pem
  auto_renew = true

  depends_on = [cloudflare_record.kk_verify, cloudflare_record.kk_acme]
}
```

---

### `krakenkey_endpoint`

Monitors a TLS endpoint. API: `POST /endpoints`, `GET /endpoints/:id`, `PATCH /endpoints/:id`, `DELETE /endpoints/:id`.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `host` | string | yes | Hostname to scan. Changing it forces a new resource. |
| `port` | number | no | Port, 1 to 65535. Default `443`. Changing it forces a new resource. |
| `sni` | string | no | SNI override. Defaults to `host` on the server. |
| `label` | string | no | Display label |
| `is_active` | bool | no | Whether scanning is on. Default `true`. The create call does not accept it, so a `false` value is applied with a `PATCH` after create. |

`host` and `port` cannot be changed through `PATCH`, which is why they force replacement.

#### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `id` | string | Endpoint UUID |
| `created_at` | string | RFC 3339 timestamp |

`POST /endpoints` is an upsert on `host` and `port`: if the account already has that pair, the API returns the existing endpoint instead of creating one. The resource should detect that (or check first) so two configurations do not silently share and then delete the same endpoint.

Import: by endpoint UUID.

```hcl
resource "krakenkey_endpoint" "api" {
  host  = "api.example.com"
  label = "Production API"
}
```

---

### `krakenkey_endpoint_region`

Adds one hosted probe region to an endpoint. API: `POST /endpoints/:id/regions`, `DELETE /endpoints/:id/regions/:region`, with reads from `GET /endpoints/:id`. Hosted monitoring needs the Starter plan or above, and plan limits on regions and hosted endpoints apply (403 with `code: "plan_limit_exceeded"`).

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `endpoint_id` | string | yes | Endpoint UUID. Forces a new resource. |
| `region` | string | yes | Region identifier, e.g. `us-east-1`. Forces a new resource. |

The API does not publish a list of valid regions; it accepts any string up to 50 characters. Plan-time validation would need a published list first.

`id` is `<endpoint_id>/<region>`, which is also the import format.

The endpoint API can also set regions in bulk (`hostedRegions` on create and update, where update replaces the whole list). `krakenkey_endpoint` should not expose that field, or the two resources will undo each other's changes.

```hcl
resource "krakenkey_endpoint_region" "us" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "us-east-1"
}
```

## Data sources

### Data source `krakenkey_certificate`

Reads a certificate by ID (`GET /certs/tls/:id`). Argument: `id` (string, required). Attributes: the computed attributes of the resource, plus `auto_renew`.

```hcl
data "krakenkey_certificate" "existing" {
  id = "42"
}
```

### Data source `krakenkey_endpoint`

Reads an endpoint by UUID (`GET /endpoints/:id`). Argument: `id` (string, required). Attributes: `host`, `port`, `sni`, `label`, `is_active`, `created_at`, and `hosted_regions` (list of strings).

```hcl
data "krakenkey_endpoint" "existing" {
  id = "3f0c9a52-6b1e-4d8a-9c47-2e5f1b7d8a90"
}
```

## Open design questions

1. **Domain verification.** Nothing above calls `POST /domains/:id/verify`, and the domain cannot be verified when it is created because the TXT record does not exist yet. Options: a separate `krakenkey_domain_verification` resource that depends on the TXT record and retries until it passes, or a `verify` flag on `krakenkey_domain` that is applied on a later update.
2. **CNAME delegation failures.** Either the documented examples carry `depends_on` (as above), or the certificate resource retries with `POST /certs/tls/:id/retry` for a while when `failure_reason` reports a delegation problem, to cover DNS propagation delay.
3. **Destroying an issued certificate.** `DELETE /certs/tls/:id` only works on `failed` or `revoked` certificates. Destroy could revoke first and then delete, or only remove the resource from state. Revoking on every destroy (including replacement after a CSR change) is a large side effect, so this needs an explicit choice, probably with a flag.
4. **Connected probes.** Endpoints can be assigned to the user's own connected probes (`probeIds`, `POST /endpoints/:id/probes`). That is left out of the first version.

## Differences from the roadmap issues

Several issues in this repository (#2, #3, #7, #10, #13, #14, #18, #19) were written before this check and describe things the API does not have. They should be updated before implementation starts:

- The API has no DNS provider resource and no notification channels (email notification preferences sit on the user profile), so `krakenkey_dns_provider` and `krakenkey_notification` have nothing to call.
- The API never returns a private key (`private_key_pem`), and it supports Let's Encrypt only (no `cert_provider`, no OV or EV `type`).
- Certificate status values are the seven listed above, not `active` or `expired`.
- There is no lookup of a certificate by domain name; that would mean listing all certificates and filtering by the CSR's names.
- The API key env var in #2 is `KRAKENKEY_API_KEY`; this proposal uses `KK_API_KEY` to match the CLI.
