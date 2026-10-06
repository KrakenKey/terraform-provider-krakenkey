data "krakenkey_endpoint" "existing" {
  id = "3f0c9a52-6b1e-4d8a-9c47-2e5f1b7d8a90"
}

output "regions" {
  value = data.krakenkey_endpoint.existing.hosted_regions
}
