terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = ">= 4.0"
    }
    google-beta = {
      source  = "hashicorp/google-beta"
      version = ">= 4.0"
    }
  }
}

provider "google" {
  project = var.project_id
  region  = var.region
  user_project_override = true
  billing_project       = var.project_id
}

provider "google-beta" {
  project = var.project_id
  region  = var.region
  user_project_override = true
  billing_project       = var.project_id
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
    "iam.googleapis.com",            # Required for Service Account creation
    "apigateway.googleapis.com",     # Required for API Gateway
    "servicecontrol.googleapis.com", # Required for API Gateway management
    "servicemanagement.googleapis.com", # Required for API Gateway management
    "apikeys.googleapis.com",        # Required for API Key creation and validation
    "logging.googleapis.com"         # Required for Cloud Logging
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

# Grant the Runtime Service Account permission to write logs
resource "google_project_iam_member" "runtime_sa_logging_writer" {
  project = data.google_project.current.id
  role    = "roles/logging.logWriter"
  member  = "serviceAccount:${google_service_account.email_sender.email}"
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
      FUNCTION_IDENTITY_EMAIL = google_service_account.email_sender.email
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
resource "google_cloud_run_service_iam_member" "invoker" {
  for_each = toset(var.invoker_members)
  location = google_cloudfunctions2_function.email_handler.location
  service  = google_cloudfunctions2_function.email_handler.service_config[0].service
  role     = "roles/run.invoker"
  member   = each.value
}

# ==============================================================================
# 6. API GATEWAY (The Public Facade)
# ==============================================================================

# A. The Gateway's Identity
# This robot talks to the Cloud Function. We do NOT download a key for it.
resource "google_service_account" "gateway_identity" {
  account_id   = "api-gateway-sa"
  display_name = "API Gateway Service Account"
}

# B. Grant Gateway permission to Invoke the Function
resource "google_cloud_run_service_iam_member" "gateway_invoker" {
  location = google_cloudfunctions2_function.email_handler.location
  service  = google_cloudfunctions2_function.email_handler.service_config[0].service
  role     = "roles/run.invoker"
  member   = "serviceAccount:${google_service_account.gateway_identity.email}"
}

# C. Grant Gateway permission to create OIDC tokens (Required for x-google-backend authentication)
resource "google_service_account_iam_member" "gateway_token_creator" {
  service_account_id = google_service_account.gateway_identity.name
  role               = "roles/iam.serviceAccountTokenCreator"
  member             = "serviceAccount:${google_service_account.gateway_identity.email}"
}

# D. The API Definition
resource "google_api_gateway_api" "email_api" {
  provider = google-beta
  api_id   = "email-handler-api"

  depends_on = [
    google_project_service.apis
  ]
}

# Enable the Managed Service generated by the Gateway API
# This allows the project to consume its own API (required for API Keys to work)
resource "google_project_service" "email_api_managed_service" {
  project            = var.project_id
  service            = google_api_gateway_api.email_api.managed_service
  disable_on_destroy = false
}

# E. The API Config (Uploads the OpenAPI Spec)
resource "google_api_gateway_api_config" "email_cfg" {
  provider      = google-beta
  api           = google_api_gateway_api.email_api.api_id
  api_config_id_prefix = "email-cfg-"

  openapi_documents {
    document {
      path     = "api_config.yaml"
      # Inject the actual Function URL into the YAML
      contents = base64encode(templatefile("${path.module}/api_config.yaml", {
        function_url = google_cloudfunctions2_function.email_handler.service_config[0].uri
      }))
    }
  }

  gateway_config {
    backend_config {
      google_service_account = google_service_account.gateway_identity.email
    }
  }

  lifecycle {
    create_before_destroy = true
  }
}

# F. The Gateway Instance (The actual load balancer)
resource "google_api_gateway_gateway" "gw" {
  provider   = google-beta
  api_config = google_api_gateway_api_config.email_cfg.id
  gateway_id = "email-gateway"
  region     = var.gateway_region
}

# ==============================================================================
# 7. THE API KEY (For Request Server)
# ==============================================================================

# Create a restricted API Key
resource "google_apikeys_key" "remote_key" {
  name         = "email-gateway-key"
  display_name = "Email Gateway API Key"

  restrictions {
    api_targets {
      # Restrict to the specific Managed Service created for this API
      service = google_api_gateway_api.email_api.managed_service
    }
    
    # SECURITY: If Remote Servers has a static IP, we would list it here.
  }
}