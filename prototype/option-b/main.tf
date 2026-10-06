# Option B prototype: Terraform creates the private key once, hands it to a
# secret store, and never writes it to plan or state. KrakenKey only sees the CSR.
#
# Vault stands in for any store with write-only arguments (AWS Secrets Manager
# secret_string_wo, Azure Key Vault value_wo, ...). The "deployed" secret stands
# in for a consumer that needs the key and certificate together, like
# aws_acm_certificate with private_key_wo.

terraform {
  required_version = ">= 1.11"
  required_providers {
    krakenkey = {
      source = "krakenkey/krakenkey"
    }
    tls = {
      source  = "hashicorp/tls"
      version = ">= 4.4.0"
    }
    vault = {
      source = "hashicorp/vault"
    }
  }
}

variable "key_version" {
  description = "Bump to rotate the private key. Nothing about the key changes until this does."
  type        = number
  default     = 1
}

variable "vault_address" {
  type    = string
  default = "http://127.0.0.1:18200"
}

provider "vault" {
  address = var.vault_address
  # Dev-mode token for the prototype only.
  token = "root"
}

# KK_API_URL and KK_API_KEY come from the environment.
provider "krakenkey" {}

# 1. A fresh key on every run. Ephemeral: it never reaches plan or state, and
#    it's only used when key_version changes.
ephemeral "tls_private_key" "web" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

# 2. Hand the key to the store. Write-only, so state keeps no copy.
resource "vault_kv_secret_v2" "key" {
  mount                = "secret"
  name                 = "tls/example.com/key"
  data_json_wo         = jsonencode({ private_key_pem = ephemeral.tls_private_key.web.private_key_pem })
  data_json_wo_version = var.key_version
}

# 3. The CSR, from the same key in the same run. The CSR is public and stays in
#    state, so later runs reuse it without the key.
resource "tls_cert_request" "web" {
  private_key_pem_wo         = ephemeral.tls_private_key.web.private_key_pem
  private_key_pem_wo_version = var.key_version

  subject {
    common_name = "example.com"
  }
  dns_names = ["example.com", "www.example.com"]
}

# 4. KrakenKey signs the CSR and renews it on its own, reusing the CSR.
resource "krakenkey_certificate" "web" {
  csr_pem = tls_cert_request.web.cert_request_pem
}

# 5. A consumer that needs the key again: read it back from the store, never
#    regenerate it. expires_at_unix changes on every renewal, which re-sends
#    the write-only value with the new certificate.
ephemeral "vault_kv_secret_v2" "key" {
  mount = vault_kv_secret_v2.key.mount
  # Ephemeral resources are read during plan whenever their arguments are
  # known, and depends_on doesn't change that. On the first run the secret
  # doesn't exist yet, so tie the name to the computed id: it's unknown until
  # the secret is created, which moves the read to apply.
  name = vault_kv_secret_v2.key.id != null ? vault_kv_secret_v2.key.name : null
}

resource "vault_kv_secret_v2" "deployed" {
  mount = "secret"
  name  = "deploy/example.com"
  data_json_wo = jsonencode({
    certificate = krakenkey_certificate.web.fullchain_pem
    private_key = ephemeral.vault_kv_secret_v2.key.data.private_key_pem
  })
  data_json_wo_version = krakenkey_certificate.web.expires_at_unix
}

output "certificate_id" {
  value = krakenkey_certificate.web.id
}

output "expires_at" {
  value = krakenkey_certificate.web.expires_at
}
