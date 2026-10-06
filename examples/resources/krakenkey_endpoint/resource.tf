resource "krakenkey_endpoint" "api" {
  host  = "api.example.com"
  label = "Production API"
}

# A non-default port, created with scanning switched off.
resource "krakenkey_endpoint" "staging" {
  host      = "staging.example.com"
  port      = 8443
  is_active = false
}

# An endpoint that already exists in KrakenKey must be imported, not created:
#   terraform import krakenkey_endpoint.api 3f0c9a52-6b1e-4d8a-9c47-2e5f1b7d8a90
