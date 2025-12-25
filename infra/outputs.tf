output "service_account_client_id" {
  description = "The Client ID (Unique ID) needed for Domain-Wide Delegation"
  value       = google_service_account.email_sender.unique_id
}

output "gateway_url" {
  value = "https://${google_api_gateway_gateway.gw.default_hostname}"
}

output "api_key_secret" {
  # This creates a dependency so we can see the key
  value     = google_apikeys_key.remote_key.key_string
  sensitive = true # Terraform will hide this in logs unless you ask explicitly
}