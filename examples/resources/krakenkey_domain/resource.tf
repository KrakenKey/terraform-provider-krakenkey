resource "krakenkey_domain" "example" {
  hostname = "example.com"
}

# Publish the two records with your DNS provider. Cloudflare is shown here.
resource "cloudflare_record" "kk_verify" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.txt_record_name
  type    = "TXT"
  content = krakenkey_domain.example.txt_record_value
}

resource "cloudflare_record" "kk_acme" {
  zone_id = var.cloudflare_zone_id
  name    = krakenkey_domain.example.cname_record_name
  type    = "CNAME"
  content = krakenkey_domain.example.cname_record_value
  proxied = false
}
