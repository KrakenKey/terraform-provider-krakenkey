terraform {
  required_providers {
    krakenkey = {
      source  = "krakenkey/krakenkey"
      version = "~> 0.1"
    }
  }
}

# Reads the API key from KK_API_KEY when api_key is not set.
provider "krakenkey" {}
