# The Slack incoming webhook URL is a credential. Pass it in as a variable
# (for example TF_VAR_slack_webhook_url) and never commit it.
variable "slack_webhook_url" {
  type      = string
  sensitive = true
  ephemeral = true
}

# url_wo is write-only (Terraform 1.11+): it is never stored in state or plan.
# Increment url_wo_version to send a new URL.
resource "krakenkey_alert_channel" "slack" {
  type           = "slack"
  name           = "Ops alerts"
  url_wo         = var.slack_webhook_url
  url_wo_version = 1
}

# A signed webhook. KrakenKey returns the signing secret once, on create, and
# the provider keeps it in state as signing_secret.
resource "krakenkey_alert_channel" "deploy" {
  type           = "webhook"
  name           = "Certificate deploy hook"
  url_wo         = "https://deploy.example.com/hooks/krakenkey"
  url_wo_version = 1
  events         = ["cert.issued", "cert.renewed"]
}

output "deploy_hook_signing_secret" {
  value     = krakenkey_alert_channel.deploy.signing_secret
  sensitive = true
}
