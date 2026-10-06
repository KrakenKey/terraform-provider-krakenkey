resource "krakenkey_endpoint" "api" {
  host = "api.example.com"
}

# Needs the Starter plan or above.
resource "krakenkey_endpoint_region" "us" {
  endpoint_id = krakenkey_endpoint.api.id
  region      = "us-east-1"
}

# Import with <endpoint_id>/<region>:
#   terraform import krakenkey_endpoint_region.us 3f0c9a52-6b1e-4d8a-9c47-2e5f1b7d8a90/us-east-1
