# Provider design proposal

> **In progress.** `krakenkey_certificate` is implemented and tested against a fake API; the other resources are still proposals. Nothing is released yet. See the status notice in [README.md](README.md).

## v1 decisions (2026-10-06)

- **Scope:** `krakenkey_domain`, `krakenkey_domain_verification`, `krakenkey_certificate`, `krakenkey_endpoint`, `krakenkey_endpoint_region`, `krakenkey_alert_channel`, and data sources for a certificate and an endpoint. No DNS provider resource and no private keys: the API has neither.
- **Destroy keeps certificates.** Destroying or replacing `krakenkey_certificate` removes it from state and leaves it valid in KrakenKey, with a warning. `revoke_on_destroy = true` opts in to revoking.
- **The provider never generates or holds private keys.** Keys come from wherever the user makes them; see [Private keys](#private-keys). Option B (Terraform creates the key and hands it to a secret store without writing it to state) was prototyped and works; see [`prototype/option-b`](prototype/option-b/).
- **Terraform is not the renewal scheduler.** KrakenKey renews on the server. How the renewed certificate reaches the thing serving it has to be explicit; see [Renewal delivery](#renewal-delivery).

Each resource below maps to endpoints in the public KrakenKey API (`https://api.krakenkey.io/swagger-json`, rendered at <https://krakenkey.io/docs/api/>). The design only proposes attributes that the API can back. Where the API needs a decision from the provider, it is listed under [Open design questions](#open-design-questions).

## Provider configuration

| Argument | Type | Required | Env var | Description |
|----------|------|----------|---------|-------------|
| `api_key` | string, sensitive | yes, unless the env var is set | `KK_API_KEY` | User API key (`kk_...`) |
| `api_url` | string | no | `KK_API_URL` | API base URL. Default `https://api.krakenkey.io` |
| `acme_zone` | string | no | | Zone the `_acme-challenge` CNAME points into. Derived from `api_url` by default; see [`krakenkey_domain`](#krakenkey_domain). |

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

`krakenkey_domain_verification` (below) handles step 3 and waits for the TXT record. Examples still carry an explicit `depends_on` from the certificate to the CNAME record, because nothing in the graph links them.

## Resources

### `krakenkey_domain`

Registers a domain. API: `POST /domains`, `GET /domains/:id`, `DELETE /domains/:id`.

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `hostname` | string | yes | Domain name, e.g. `example.com`. Lowercase, no wildcard, no trailing dot. Changing it forces a new resource. |

#### Attributes

| Attribute | Type | Source | Description |
|-----------|------|--------|-------------|
| `id` | string | API | Domain UUID |
| `verified` | bool | API `isVerified` | Whether ownership has been verified |
| `verification_code` | string | API `verificationCode` | Full TXT value, `krakenkey-site-verification=<hex>` |
| `txt_record_name` | string | provider | Same as `hostname`; the TXT record goes on the hostname itself |
| `txt_record_value` | string | provider | Same as `verification_code` |
| `cname_record_name` | string | provider | `_acme-challenge.<hostname>` |
| `cname_record_value` | string | provider | `<hostname with dots replaced by dashes>.<acme_zone>`, e.g. `example-com.acme.krakenkey.io` |
| `created_at` | string | API | RFC 3339 timestamp |

The API does not return the record names or the CNAME target, so the provider derives them. The target zone is a server setting (`KK_ACME_AUTH_ZONE_DOMAIN`) that the API does not expose. The provider picks it from `api_url`: `acme.dev.krakenkey.io` for the staging API at `https://api-dev.krakenkey.io`, and `acme.krakenkey.io` for everything else. The provider argument `acme_zone` overrides it for any other KrakenKey server. The CNAME name and target are lowercase, matching the check the issuance job makes.

The API matches hostnames exactly and does not lowercase them, so `Example.com` and `example.com` would be two domains. The provider rejects uppercase, wildcard (`*.example.com`) and trailing-dot hostnames at plan time. A verified domain already covers its subdomains and wildcards.

`POST /domains` returns the existing domain when the account already has the hostname. Create lists the domains first and fails with an import hint if the returned ID was already there, so two configurations cannot share one domain and later delete it from under each other.

Destroy calls `DELETE /domains/:id`, which the API allows at any time. Certificates are not linked to a domain record: they stay valid and keep auto-renewing (renewal checks the CNAME, not the domain). New certificates for those names are refused until the domain is registered and verified again, with a new verification code and therefore a new TXT value. The domain is shared with the user's organization, so deleting it affects every member.

Import: by domain UUID.

```hcl
resource "krakenkey_domain" "example" {
  hostname = "example.com"
}
```

---

### `krakenkey_domain_verification`

Verifies a domain once its TXT record is published. API: `POST /domains/:id/verify`, `GET /domains/:id`.

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `domain_id` | string | yes | `krakenkey_domain.<name>.id`. Forces a new resource. |

Create calls verify and retries until it passes or the create timeout (default 10 minutes) runs out, to cover DNS propagation. The API answers 400 when the TXT record is missing or the DNS lookup fails; those are retried, along with 502, 503 and 504. Any other error (401, 403, 404, 402, 429) fails at once. Verify is in the hourly "expensive" rate limit bucket (5 an hour on Free) even when the domain is already verified, so each attempt reads the domain first and skips verify if it is verified, and the wait between attempts starts at 30 seconds and doubles up to 4 minutes. A 10 minute timeout makes at most 5 verify calls. Read reports `verified` from `GET /domains/:id`; if KrakenKey's daily re-check finds the TXT record gone, the domain becomes unverified and the next plan recreates this resource, which verifies again. Delete only removes it from state.

Import: by domain UUID.

```hcl
resource "krakenkey_domain_verification" "example" {
  domain_id  = krakenkey_domain.example.id
  depends_on = [cloudflare_record.kk_verify]
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
| `revoke_on_destroy` | bool | no | Default `false`: destroy and replacement only remove the certificate from state. `true` revokes it first. |
| `timeouts.create` | string | no | How long create waits for issuance. Default `20m`. |

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
| `expires_at_unix` | number | API `expiresAt` | Expiry as a Unix timestamp. It changes whenever the certificate does (renewal or replacement), so it can drive the `*_wo_version` of a resource that re-sends a write-only private key with the new certificate. It can go down as well as up, for example after a key rotation. |
| `renewal_count` | number | API `renewalCount` | Number of renewals so far |

Create posts the CSR and saves the ID to state straight away, so a timeout or failure can't orphan the certificate. It then polls `GET /certs/tls/:id` until the status is `issued` (success) or `failed` (error that includes `failure_reason`). Issuance usually takes 2 to 5 minutes, so the resource needs a configurable create timeout. Submitting the same CSR again within 15 minutes returns the existing certificate instead of a new one (or 409 while the first request is still being created, which is safe to retry). That makes a retried create safe.

When auto-renew is on, KrakenKey renews on the server and the PEM and expiry attributes change. Those are computed values, so the next refresh picks them up without showing a diff on any argument. The PEM attributes only change while the status is `issued`: during a renewal, or after a failed one, they keep the last issued certificate, so resources that deploy it never see it go empty.

Certificates are public (all Let's Encrypt certificates are logged to Certificate Transparency), so the PEM attributes do not need `Sensitive: true`. The secret is the private key behind the CSR; see [Private keys](#private-keys).

Deploy `fullchain_pem`, not `cert_pem`. Some clients fetch a missing intermediate from the leaf's AIA `caIssuers` URL (Windows, macOS and Chrome do) and some do not (OpenSSL, Go, Firefox, and Java by default). A leaf-only deployment can pass a browser check and still fail in `curl` or a Go service. CA/Browser Forum ballot SC104 (passed 2026-09-03) makes the AIA extension optional in subscriber certificates, so that fallback may disappear.

Let's Encrypt plans to move to Merkle Tree Certificates (staging late 2026, production 2027). MTC does not use the `chain_pem` / `fullchain_pem` model, so these attributes will need a compatibility review before then.

Import: by certificate ID. Read fills `csr_pem` from the API's `rawCsr` when state has none, and otherwise keeps the configured value unless the content differs beyond surrounding whitespace.

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

`POST /endpoints` is an upsert on `host` and `port`: if the account already has that pair, the API returns the existing endpoint, overwrites its `label` and `sni`, and skips the plan limit check. The create response looks like a fresh endpoint, and `createdAt` can't be compared reliably against the local clock. So create lists endpoints first (`GET /endpoints`) and fails with an error that names the existing endpoint's UUID and tells the user to import it, without calling `POST`. Two applies racing on the same pair could still slip past the check; the window is one request.

For an organization member the list covers the whole organization, which matches what the upsert does.

`PATCH` accepts `sni`, `label` and `isActive` (it also takes `probeIds` and `hostedRegions`, which this resource does not send). A `label` or `sni` removed from the configuration is sent as `null`, which clears it. The server never fills in `sni` itself, so both are plain optional arguments.

Import: by endpoint UUID.

```hcl
resource "krakenkey_endpoint" "api" {
  host  = "api.example.com"
  label = "Production API"
}
```

---

### `krakenkey_endpoint_region`

Adds one hosted probe region to an endpoint. API: `POST /endpoints/:id/regions`, `DELETE /endpoints/:id/regions/:region`, with reads from `GET /endpoints/:id`. Hosted monitoring needs the Starter plan or above, and plan limits on regions and hosted endpoints apply (an error body with `code: "plan_limit_exceeded"`).

#### Arguments

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `endpoint_id` | string | yes | Endpoint UUID. Forces a new resource. |
| `region` | string | yes | Region identifier, e.g. `us-east-1`. Forces a new resource. |

The API does not publish a list of valid regions; it accepts any string up to 50 characters. Plan-time validation would need a published list first.

`POST /endpoints/:id/regions` is idempotent: adding a region the endpoint already has returns success. Create therefore reads the endpoint first and refuses with an import hint if the region is there, for the same reason as the endpoint upsert. Read removes the resource from state when the endpoint or the region is gone. Removing a region that is already gone (404) is not an error on destroy.

`id` is `<endpoint_id>/<region>`, which is also the import format.

The endpoint API can also set regions in bulk (`hostedRegions` on create and update, where update replaces the whole list). `krakenkey_endpoint` should not expose that field, or the two resources will undo each other's changes.

```hcl
resource "krakenkey_endpoint_region" "us" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "us-east-1"
}
```

### `krakenkey_alert_channel`

A Slack, Teams or signed-webhook channel for alerts. API: `GET/POST /notifications/channels`, `PATCH/DELETE /notifications/channels/:id`. Needs a key with the `account:read` and `account:write` scopes. Keys limited to specific domains or certificates get a 403: a channel receives alerts for the whole account.

| Argument | Type | Required | Description |
|----------|------|----------|-------------|
| `type` | string | yes | `slack`, `teams` or `webhook`. Forces a new resource. |
| `name` | string | yes | Display name, up to 100 characters. The API trims surrounding whitespace, so the provider rejects it at plan time. |
| `url_wo` | string, write-only | yes | Incoming webhook URL. The URL is a credential and the API only returns it masked, so the provider never stores it. |
| `url_wo_version` | number | yes | Change it to send a new URL. |
| `events` | set of strings | no | Events to send. Defaults to the API's default set (`cert.failed`, `cert.expiring`, `cert.revoked`, `cert.replacement_requested`, `domain.verification_failed`, `endpoint.scan_failed`). An empty set is allowed and sends nothing. |
| `enabled` | bool | no | Default `true`. |

Computed: `id`, `url_masked` (scheme, host and the last 4 characters of the URL). For `webhook` channels the API returns the signing secret once, on create. The provider exposes it as a sensitive `signing_secret` attribute, so it is in state; the receiver needs it, and it can be rotated in the dashboard. Rotating it from Terraform is left out because every run would rotate it, and a secret rotated in the dashboard is not reflected in state.

How the provider maps to the API:

- There is no `GET /notifications/channels/:id`. Read lists the account's channels and looks for the ID; a channel missing from the list is removed from state.
- `PATCH` accepts `url`, so a new URL is an in-place update. The provider sends `url` only when `url_wo_version` changes; changing `url_wo` alone plans nothing, because Terraform does not diff write-only values. `name`, `events` and `enabled` are sent on every update.
- `type` and `events` are checked at plan time against the API's lists. URL rules (Slack must be `https://hooks.slack.com/services/...`, Teams must be a Workflows URL, webhooks must use https and resolve to public addresses) are left to the API, whose 400 message is passed through.
- An account can have at most 10 channels. Creating an eleventh returns 400.

Import: by channel UUID. `url_wo` cannot be imported, and state has no `url_wo_version` afterwards, so the first apply after an import sends the configured URL once. `signing_secret` is null after import.

Write-only arguments need Terraform 1.11 or OpenTofu 1.11.

## Data sources

### Data source `krakenkey_certificate`

Reads a certificate by ID (`GET /certs/tls/:id`). Argument: `id` (string, required). Attributes: the computed attributes of the resource, plus `auto_renew` (and `csr_pem`, from the API's `rawCsr`). The PEM attributes and expiry are only set while the certificate is issued, and a missing certificate is an error.

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

## Private keys

KrakenKey takes a CSR, so the provider never needs a private key. Where the key lives is the user's choice, and the docs should present three options in this order.

**A. The key never touches Terraform (default).** The key and CSR come from the host (the KrakenKey CLI), a key vault, or any other tool, and Terraform gets only `csr_pem`. Nothing secret is in state. Terraform manages the domain, its DNS records, the certificate record and monitoring.

**B. Terraform creates the key once and hands it off.** An ephemeral `tls_private_key` feeds `tls_cert_request.private_key_pem_wo` (hashicorp/tls 4.4.0+) and a write-only secret store argument (`aws_secretsmanager_secret_version.secret_string_wo`, `azurerm_key_vault_secret.value_wo`, `google_secret_manager_secret_version.secret_data_wo`, `vault_kv_secret_v2.data_json_wo`). Both are pinned to the same `*_wo_version`, so the key and CSR only change when the user bumps it. A consumer that needs the key again (for example `aws_acm_certificate.private_key_wo`) reads it back from the store with an ephemeral resource and never regenerates it. Requires Terraform 1.11 or OpenTofu 1.11.

The prototype in [`prototype/option-b`](prototype/option-b/) ran create, a no-op plan, a server-side renewal, a key rotation and destroy against a mock API and Vault. No private key appeared in state, in the saved plan, or in any API request, and the stored key, CSR, certificate and deployed copy matched at every step. Two rules came out of it:

- The ephemeral key is regenerated on every run. Anything that consumes it directly must be pinned to the version argument, or each run would rotate the key.
- An ephemeral read of the stored key runs at plan time whenever its arguments are known, and `depends_on` does not change that. On the first run the secret doesn't exist yet, so the read must take an argument derived from a computed attribute of the store resource (the prototype uses its `id`) to move it to apply.

**C. The key in state.** A managed `tls_private_key` is stored in plain text in state. Acceptable with an encrypted backend or OpenTofu state encryption (1.7+), and simpler, but documented with that warning rather than as the default.

## Renewal delivery

Terraform only acts when it runs, so it is a poor renewal scheduler: the main ACME provider renews only during an apply inside `min_days_remaining`. With KrakenKey the certificate renews on the server regardless, but a renewed certificate still has to reach whatever serves it. Each example says which of these it relies on:

1. **The server pulls.** The host runs the CLI on a timer, or a workflow uses the GitHub Action, and Terraform only sets things up. Most reliable.
2. **Scheduled apply.** A daily plan and apply refreshes `fullchain_pem`, and resources that consume it (an ACM import, a Kubernetes secret, a write-only store keyed on `expires_at_unix`) update. Fails quietly if the schedule stops.
3. **Event-driven.** A `cert.renewed` alert sent to a webhook channel triggers the pipeline. Needs a small relay, because the webhook can't carry a CI token.

Pair any of them with a `krakenkey_endpoint` on the same hostname, so a renewal that never reached the server shows up as an expiring certificate.

## API key scopes

The provider works with a full-access key. For a scoped key, these are the scopes each resource needs:

| Resource | Scopes |
|----------|--------|
| `krakenkey_domain`, `krakenkey_domain_verification` | `domains:read`, `domains:write` |
| `krakenkey_certificate` | `certs:read`, `certs:issue`; `certs:renew` to change `auto_renew`; `certs:revoke` with `revoke_on_destroy` |
| `krakenkey_endpoint`, `krakenkey_endpoint_region` | `endpoints:read`, `endpoints:write` |
| `krakenkey_alert_channel` | `account:read`, `account:write` |

## Errors from the API

Seen against the staging API on 2026-10-06. The provider passes the API's message through, so these reach the user as written:

- **Missing scope:** 403, `This API key needs the certs:issue scope for this request.`
- **Certificate, domain and API key plan limits:** 402, for example `Total active certificate limit reached`. Endpoint and region limits return 403 with a body that has `code: "plan_limit_exceeded"` (plus `limit`, `current` and `plan`, except for plans without hosted monitoring, which omit `limit` and `current`). The provider appends them to the message, for example `Endpoint limit reached (limit 3, in use 3, plan free)`.
- **Rate limits:** 429 with `Retry-After` in seconds. Issuance, renewal, retry, revocation and domain verification share the hourly "expensive" bucket (5 an hour on Free), so a plan that creates and replaces several certificates can run out mid-apply. The provider adds the retry time to the error rather than waiting, since the wait can be close to an hour.

## Open design questions

1. **CNAME delegation failures.** Either the documented examples carry `depends_on` (as above), or the certificate resource retries with `POST /certs/tls/:id/retry` for a while when `failure_reason` reports a delegation problem, to cover DNS propagation delay.
2. **Connected probes.** Endpoints can be assigned to the user's own connected probes (`probeIds`, `POST /endpoints/:id/probes`). That is left out of the first version.
3. **GitHub OIDC.** The API can exchange a GitHub Actions OIDC token for a short-lived key (KrakenKey/app#136). The provider could accept one so CI runs hold no stored key. Not in v1.

Resolved on 2026-10-06: domain verification is a separate resource; destroy keeps certificates unless `revoke_on_destroy` is set.

## Differences from the roadmap issues

Several issues in this repository (#2, #3, #7, #10, #13, #14, #18, #19) were written before this check and describe things the API does not have. They should be updated before implementation starts:

- The API has no DNS provider resource, so `krakenkey_dns_provider` has nothing to call. Notification channels now exist (KrakenKey/app#137) and are covered by `krakenkey_alert_channel`.
- The API never returns a private key (`private_key_pem`), and it supports Let's Encrypt only (no `cert_provider`, no OV or EV `type`).
- Certificate status values are the seven listed above, not `active` or `expired`.
- There is no lookup of a certificate by domain name; that would mean listing all certificates and filtering by the CSR's names.
- The API key env var in #2 is `KRAKENKEY_API_KEY`; this proposal uses `KK_API_KEY` to match the CLI.
