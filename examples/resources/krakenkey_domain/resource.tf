resource "krakenkey_domain" "example" {
  hostname = "example.com"
}

# Publish the two records with your DNS provider. Cloudflare provider v5 is shown here.
resource "cloudflare_dns_record" "kk_verify" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.txt_record_name
  type    = "TXT"
  content = krakenkey_domain.example.txt_record_value
  ttl     = 1
}

resource "cloudflare_dns_record" "kk_acme" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.cname_record_name
  type    = "CNAME"
  content = krakenkey_domain.example.cname_record_value
  ttl     = 1
  proxied = false
}
