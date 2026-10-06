# Waits until KrakenKey can see the TXT record from krakenkey_domain.
# Nothing in the graph links the DNS record to the verification, so depends_on
# is needed.
resource "krakenkey_domain_verification" "example" {
  domain_id = krakenkey_domain.example.id

  timeouts = {
    create = "15m"
  }

  depends_on = [cloudflare_record.kk_verify]
}
