terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
}

data "google_project" "current" {}

# ==============================================================================
# 1. ENABLE REQUIRED APIS
# ==============================================================================
resource "google_project_service" "apis" {
  for_each = toset([
    "cloudfunctions.googleapis.com",
    "run.googleapis.com",
    "artifactregistry.googleapis.com",
    "cloudbuild.googleapis.com",
    "gmail.googleapis.com",
    "iamcredentials.googleapis.com", # Required for Keyless Signing
    "storage.googleapis.com",        # Required for Bucket creation
    "iam.googleapis.com"             # Required for Service Account creation
  ])
  service            = each.key
  disable_on_destroy = false
}

# ==============================================================================
# 2. IDENTITY & PERMISSIONS (KEYLESS ARCHITECTURE)
# ==============================================================================

# A. The Service Account (The "Robot")
resource "google_service_account" "email_sender" {
  account_id   = "notifications-robot"
  display_name = "Email Notification Service Account"
  description  = "Identity for the Gmail Handler Cloud Function"
}

# B. Allow the Robot to "Sign" itself (Self-Impersonation)
# This is REQUIRED for the IAM Credentials API flow (Keyless Domain-Wide Delegation)
resource "google_service_account_iam_member" "self_token_creator" {
  service_account_id = google_service_account.email_sender.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.email_sender.email}"
}

# C. Fix for Cloud Functions Gen 2 Build Permission Error
# Grants the Default Compute Service Account access to read storage buckets (required for build artifacts)
resource "google_project_iam_member" "compute_sa_storage_viewer" {
  project = data.google_project.current.id
  role    = "roles/storage.objectViewer"
  member  = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

# D. Fix for Artifact Registry Permission Error
# Grants the Default Compute Service Account access to read/write artifacts (required for Cloud Build)
resource "google_project_iam_member" "compute_sa_artifact_registry" {
  project = data.google_project.current.id
  role    = "roles/artifactregistry.writer"
  member  = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

# E. Fix for Cloud Logging Permission Error
# Grants the Default Compute Service Account access to write logs (required for Cloud Build)
resource "google_project_iam_member" "compute_sa_logging_writer" {
  project = data.google_project.current.id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${data.google_project.current.number}-compute@developer.gserviceaccount.com"
}

# ==============================================================================
# 3. SOURCE CODE MANAGEMENT
# ==============================================================================

# A. Zip the Go Code
data "archive_file" "function_zip" {
  type        = "zip"
  source_dir  = "${path.module}/.." # Assumes main.tf is in a subfolder (e.g., /terraform)
  output_path = "/tmp/function.zip"
  
  # Exclude infrastructure and git files from the build artifact
  excludes    = [".git", "terraform", "infra", "local_bin", ".DS_Store", "cmd"]
}

# B. Create Bucket for Source Code
resource "google_storage_bucket" "source_bucket" {
  name                        = "${var.project_id}-gcf-source"
  location                    = "US"
  uniform_bucket_level_access = true
  
  # Clean up old source zips automatically after 30 days
  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }
}

# C. Upload Zip to Bucket
resource "google_storage_bucket_object" "archive" {
  # Use MD5 in name to force function redeploy on code changes
  name   = "source-${data.archive_file.function_zip.output_md5}.zip"
  bucket = google_storage_bucket.source_bucket.name
  source = data.archive_file.function_zip.output_path
}

# ==============================================================================
# 4. CLOUD FUNCTION (GEN 2)
# ==============================================================================

resource "google_cloudfunctions2_function" "email_handler" {
  name        = "gmail-handler"
  location    = var.region
  description = "Secure Gmail API Handler via Domain-Wide Delegation"

  build_config {
    runtime     = "go125"
    entry_point = "HandleEmail" # Must match function.go
    
    source {
      storage_source {
        bucket = google_storage_bucket.source_bucket.name
        object = google_storage_bucket_object.archive.name
      }
    }
  }

  service_config {
    max_instance_count = 10
    min_instance_count = 0
    available_memory   = "256Mi"
    timeout_seconds    = 60
    
    # Run as the Robot
    service_account_email = google_service_account.email_sender.email
    
    # Environment Variables for Application Logic
    environment_variables = {
      LOG_LEVEL               = "INFO"
      DELEGATED_USER_EMAIL    = var.delegated_user_email
      ALIAS_USER_EMAIL        = var.alias_email
      FUNCTION_IDENTITY_EMAIL = google_service_account.email_sender.email
      SENDER_DISPLAY_NAME     = var.sender_display_name
    }
  }

  depends_on = [
    google_project_service.apis
  ]
}

# ==============================================================================
# 5. SECURITY: INVOKER PERMISSIONS
# ==============================================================================

# Restrict HTTP access to specific members (OIDC principals, other projects, etc.)
resource "google_cloud_run_service_iam_binding" "invoker" {
  location = google_cloudfunctions2_function.email_handler.location
  service  = google_cloudfunctions2_function.email_handler.name
  role     = "roles/run.invoker"
  
  members = var.invoker_members
}