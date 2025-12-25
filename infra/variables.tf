variable "project_id" {
  description = "The Google Cloud Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region for deployment"
  type        = string
}

variable "gateway_region" {
  description = "Region for API Gateway (must support API Gateway service)"
  type        = string
  default     = "us-central1"
}

variable "delegated_user_email" {
  description = "The Workspace user email address to impersonate"
  type        = string
}

variable "alias_email" {
  description = "The alias email address to appear in the 'From' header"
  type        = string
}

variable "invoker_members" {
  description = "List of IAM members allowed to invoke the function (e.g., 'serviceAccount:...', 'group:...')"
  type        = list(string)
  default     = [] // empty by default for security

  validation {
    condition     = alltrue([for m in var.invoker_members : can(regex("^(user|serviceAccount|group|domain):", m))])
    error_message = "Each member must start with a valid IAM prefix (e.g., 'user:', 'serviceAccount:', 'group:')."
  }
}

variable "sender_display_name" {
  description = "The name to display in the 'From' header (e.g., 'Notification Bot')"
  type        = string
  default     = ""
}