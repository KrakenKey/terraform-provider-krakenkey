# The private key is ephemeral, so it never reaches plan or state. Store it with
# your secret store's write-only argument, pinned to the same version, so the
# server can use it later; see DESIGN.md#private-keys.
ephemeral "tls_private_key" "web" {
  algorithm   = "ECDSA"
  ecdsa_curve = "P256"
}

resource "tls_cert_request" "web" {
  private_key_pem_wo         = ephemeral.tls_private_key.web.private_key_pem
  private_key_pem_wo_version = 1

  subject {
    common_name = "example.com"
  }
  dns_names = ["example.com", "www.example.com"]
}

resource "krakenkey_certificate" "web" {
  csr_pem    = tls_cert_request.web.cert_request_pem
  auto_renew = true

  # Every name in the CSR must be on a verified domain, and the
  # _acme-challenge CNAME must be in place. Terraform can't see either
  # dependency, so state it.
  depends_on = [
    krakenkey_domain_verification.example,
    cloudflare_dns_record.kk_acme,
  ]
}

# Deploy fullchain_pem (leaf plus intermediates), not cert_pem.
output "fullchain_pem" {
  value = krakenkey_certificate.web.fullchain_pem
}
