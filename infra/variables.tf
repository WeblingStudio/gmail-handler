variable "project_id" {
  description = "The Google Cloud Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region for deployment"
  type        = string
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
}

variable "sender_display_name" {
  description = "The name to display in the 'From' header (e.g., 'Notification Bot')"
  type        = string
  default     = ""
}