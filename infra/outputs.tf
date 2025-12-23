output "function_uri" {
  description = "The public URL of your Cloud Function"
  value       = google_cloudfunctions2_function.email_handler.service_config[0].uri
}

output "service_account_email" {
  description = "The email of the robot account"
  value       = google_service_account.email_sender.email
}

output "service_account_client_id" {
  description = "The Client ID (Unique ID) needed for Domain-Wide Delegation"
  value       = google_service_account.email_sender.unique_id
}