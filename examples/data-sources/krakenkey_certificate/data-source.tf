data "krakenkey_certificate" "existing" {
  id = "42"
}

output "certificate_status" {
  value = data.krakenkey_certificate.existing.status
}

output "certificate_expires_at" {
  value = data.krakenkey_certificate.existing.expires_at
}
