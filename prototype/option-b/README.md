# Option B prototype: a private key Terraform never stores

This checks the [option B](../../DESIGN.md#private-keys) pattern end to end. Terraform creates the private key, hands it to a secret store, and gets a certificate from KrakenKey. No private key ends up in state, in a saved plan, or in any API request.

It runs against [`mockapi.py`](mockapi.py), a stand-in for the KrakenKey certificate endpoints that signs CSRs with a throwaway CA, and a Vault dev server as the secret store. A real KrakenKey account would need a verified domain with live DNS records, and that adds nothing to what's being tested here: the Terraform side.

## What [`main.tf`](main.tf) does

1. `ephemeral "tls_private_key"` makes a key on every run. It never reaches plan or state.
2. `vault_kv_secret_v2.key` stores the key through the write-only `data_json_wo`, pinned to `var.key_version`.
3. `tls_cert_request` builds the CSR from the same key through `private_key_pem_wo` (hashicorp/tls 4.4.0+), pinned to the same version. The CSR is public, so it stays in state, and later runs reuse it.
4. `krakenkey_certificate` submits the CSR.
5. `vault_kv_secret_v2.deployed` stands in for a consumer that needs the key and certificate together, like `aws_acm_certificate` with `private_key_wo`. It reads the key back from Vault with an ephemeral resource and re-sends it whenever `expires_at_unix` changes.

## Results (2026-10-06, Terraform 1.14.9, tls 4.4.1, vault 5.12.0)

After each step: no `PRIVATE KEY` in `terraform.tfstate`, in `terraform show -json` of the saved plan, or in any request the mock API received. The public key of the key in Vault, the CSR, the issued certificate, and the deployed key and certificate was identical at every step.

| Step | Result |
| --- | --- |
| 1. First apply | 4 created. The CSR in state matches the key in Vault, so the CSR is built at apply time from the same ephemeral key that was stored. |
| 2. Plan again | No changes. The new ephemeral key from this run is never used. |
| 3. KrakenKey renews on the server (same CSR) | Refresh picks up the new certificate. Only the deployed secret changes, with the key read back from Vault. Key secret stays at version 1. |
| 4. `key_version = 2` | New key in Vault, new CSR, certificate replaced, deployed secret updated. The old certificate is kept in KrakenKey with a warning, not revoked. The next plan is clean. |
| 5. Destroy | Everything leaves state; the last certificate is kept in KrakenKey with a warning. The API received no revoke or delete. |

## Real run on dev (2026-10-06)

The same configuration against `https://api-dev.krakenkey.io` with the `ci-test` account, for `ci-test.krakenkey.io`, with `auto_renew = false`. Dev issues from Let's Encrypt staging.

| Step | Result |
| --- | --- |
| Before: key without `certs:issue` | `403: This API key needs the certs:issue scope for this request.` The Vault key and CSR were already created; nothing in KrakenKey. |
| Before: account at its cap | `402: Total active certificate limit reached.` |
| 1. Apply | Certificate 18 issued in 50 seconds, `(STAGING)` Let's Encrypt issuer, `DNS:ci-test.krakenkey.io`. It reused the key and CSR left by the failed attempts. |
| 2. Plan again | No changes. |
| 3. `key_version = 2` | Certificate 19 issued in 50 seconds under the new key; 18 kept with a warning. Next plan clean. |
| 4. Destroy | 19 kept with a warning. Both still `issued`, `autoRenew: false`. |

No private key in state or saved plans at any step; Vault key, CSR, certificate and deployed copy matched each time. The server-side renewal step was only run against the mock: on dev it would have used another issuance from a rate-limited budget.

## What it taught us

- **Pin everything that consumes the ephemeral key to the version argument.** The key is new on every run; only the version decides whether it gets used.
- **Ephemeral reads run at plan time when their arguments are known, and `depends_on` doesn't defer them.** On the first run the stored key doesn't exist yet, so the plan fails with "Unable to Read Resource from Vault". Deriving an argument from a computed attribute of the store resource fixes it:

  ```hcl
  name = vault_kv_secret_v2.key.id != null ? vault_kv_secret_v2.key.name : null
  ```

- **`expires_at_unix` works as a consumer's `*_wo_version`,** but it can go down: the rotated certificate in step 4 expired a day before the renewed one it replaced. Write-only versions only need to change, so this is fine; docs should say "changes", not "increases".
- **Plain `dev_overrides` can't be used with other providers.** `terraform init` still looks `krakenkey/krakenkey` up in the registry. A filesystem mirror works; see [CONTRIBUTING.md](../../CONTRIBUTING.md#local-development-override).

## Run it

Needs Terraform 1.11+, Python 3, `openssl`, `jq` and Docker.

```bash
docker run -d --name kk-proto-vault -p 127.0.0.1:18200:8200 -e VAULT_DEV_ROOT_TOKEN_ID=root hashicorp/vault
python3 mockapi.py /tmp/kk-mock 18080 &

# Install a local build through a filesystem mirror (see CONTRIBUTING.md), then:
export KK_API_URL=http://127.0.0.1:18080 KK_API_KEY=kk_mock_prototype
terraform init && terraform apply

# Simulate a server-side renewal, then apply again:
curl -X POST "http://127.0.0.1:18080/_mock/renew/$(terraform output -raw certificate_id)"
terraform apply

# Rotate the key:
terraform apply -var key_version=2
```
