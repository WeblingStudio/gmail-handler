# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A serverless Gmail API handler deployed as a Google Cloud Function (Gen 2) that sends emails using **Domain-Wide Delegation** with a **Keyless Architecture**. The service account impersonates a Google Workspace user to send emails programmatically without managing static JSON keys.

## Commands

### Local Development
```bash
# Set required environment variables
export DELEGATED_USER_EMAIL="notifications@example.com"
export FUNCTION_IDENTITY_EMAIL="your-sa-email@..."

# Run locally using Functions Framework
go run cmd/main.go

# Test locally (server runs on port 8080 by default)
curl -X POST http://localhost:8080/send \
  -H "Content-Type: application/json" \
  -d '{"sender_address":"no-reply@example.com", "recipient_address":"test@example.com", "subject":"Test", "body_html":"<p>Test</p>"}'
```

### Deployment & Infrastructure
```bash
# All commands run from project root
# Initialize Terraform
terraform -chdir=infra init

# Preview changes
terraform -chdir=infra plan

# Deploy to GCP
terraform -chdir=infra apply

# View sensitive outputs (like API key)
terraform -chdir=infra output -raw api_key_secret
```

### Testing Deployed Function
```bash
# Retrieve gateway URL and API key from Terraform outputs
GATEWAY_URL=$(terraform -chdir=infra output -raw gateway_url)
API_KEY=$(terraform -chdir=infra output -raw api_key_secret)

# Send test email
curl -X POST "$GATEWAY_URL/send" \
  -H "x-api-key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d @payload.json
```

## Architecture

### Core Components

1. **Entry Point** (`function.go`): The HTTP handler registered with the Functions Framework. Handles routing, validation, safety checks, and orchestrates the email sending flow.

2. **Keyless Authentication** (`pkg/auth/dwd.go`): Implements `oauth2.TokenSource` using the IAM Credentials API to sign JWTs without local private keys. The service account self-signs a JWT with claims to impersonate the delegated user, then exchanges it for an OAuth access token.

3. **Email Builder** (`pkg/email/builder.go`): Constructs RFC 2822 MIME messages with HTML sanitization (using bluemonday), attachments, and custom headers. Handles multipart/mixed encoding.

4. **Infrastructure as Code** (`infra/main.tf`): Terraform manages the complete stack including Cloud Function, API Gateway, service accounts, IAM bindings, and API key restrictions.

### Authentication Flow

The keyless Domain-Wide Delegation works as follows:
1. Cloud Function runs with its service account identity (via Application Default Credentials)
2. Function calls IAM Credentials API `SignJwt` to create a signed JWT claiming to be the delegated user
3. JWT includes `sub` claim (delegated user email) and required OAuth scopes
4. Signed JWT is exchanged at Google OAuth2 endpoint for an access token
5. Access token used to call Gmail API as the delegated user

**Critical**: The service account must be authorized in Google Workspace Admin Console → Security → API Controls → Domain-Wide Delegation with the Client ID and scopes: `https://www.googleapis.com/auth/gmail.send, https://www.googleapis.com/auth/gmail.modify`

### Request Flow

```
External Client
  → API Gateway (x-api-key auth)
  → Cloud Function (OIDC auth from gateway)
  → HandleEmail handler
  → Route: /health (returns status) OR /send (processes email)
  → Safety checks (loop prevention, size limits)
  → KeylessTokenSource.Token() (gets access token)
  → Gmail API (sends email using client-provided sender_address)
  → Label modifications (starred, important, custom labels)
```

### Safety Mechanisms

Located in `function.go:106-128`:
- **Loop Prevention**: Blocks sending to `delegated_user_email` to prevent infinite loops
- **Attachment Size Limit**: 20MB total (with base64 encoding overhead buffer)
- **HTML Sanitization**: All HTML content sanitized with bluemonday UGCPolicy

### Environment Variables

Required at runtime:
- `DELEGATED_USER_EMAIL`: The Google Workspace user to impersonate (mailbox owner)
- `FUNCTION_IDENTITY_EMAIL`: The service account email (robot identity)

**Note**: Sender information (`sender_address` and `sender_name`) is provided per-request in the JSON payload, not via environment variables.

### Important Implementation Details

1. **Warm Starts**: The Gmail service client is cached in a global variable (`gmailService`) and reused across invocations for performance.

2. **Client-Controlled Sender**: The handler uses client-provided `req.SenderAddress` without modification. Clients must ensure the address is configured as a Gmail alias. Invalid aliases will be rewritten by Gmail's anti-spoofing.

3. **Health Check Endpoint**: `/health` endpoint provides lightweight status check without Gmail service initialization. Returns JSON: `{"status":"healthy","service":"gmail-handler"}`.

4. **Label Application**: Labels are applied post-send via `Messages.Modify` rather than during send. Failures are logged but don't fail the request since the primary email send succeeded.

5. **Terraform Source Packaging**: The function source is zipped from the parent directory with exclusions for `infra/`, `.git/`, and `cmd/` directories. The zip hash is used in the GCS object name to trigger redeployment on code changes.

6. **API Gateway Authentication**: Uses a two-layer approach:
   - External clients authenticate with API Key (x-api-key header)
   - Gateway authenticates to Cloud Function with OIDC token (generated via gateway service account)

7. **Adding New Endpoints**: When adding new routes to `function.go`, you MUST also update the API Gateway OpenAPI specification:
   - Edit `infra/api_config.yaml` to add the new path definition
   - Include the `x-google-backend` configuration pointing to `${function_url}`
   - Add `security: - api_key_header: []` to require API key authentication
   - Deploy with `terraform -chdir=infra apply` to update the gateway
   - Example: The `/health` endpoint required adding a GET path definition to the OpenAPI spec

### Module Structure

```
pkg/
├── auth/         - Keyless Domain-Wide Delegation token source
├── constants/    - Shared constants for headers, auth, MIME types
└── email/        - MIME message construction and sanitization

cmd/main.go            - Local development entry point (Functions Framework)
function.go            - Cloud Function HTTP handler
infra/
├── main.tf            - Terraform resources (Function, Gateway, IAM)
├── api_config.yaml    - OpenAPI spec for API Gateway (defines endpoints)
├── variables.tf       - Input variables
├── outputs.tf         - Output values
└── terraform.tfvars   - Terraform variables (.gitignored)
```

### Deployment Prerequisites

Before deploying, ensure:
1. Service Account Client ID from Terraform output is authorized in Google Workspace Admin Console
2. All sender addresses you plan to use are configured as Gmail aliases for the `delegated_user_email` (Settings → Accounts → Send mail as)
3. All required APIs are enabled (done automatically by Terraform)

### Terraform Configuration

Required variables in `infra/terraform.tfvars`:
- `project_id`: GCP project ID
- `region`: GCP region for Cloud Function
- `gateway_region`: API Gateway region (must be supported, e.g., us-central1)
- `delegated_user_email`: Workspace user to impersonate
- `invoker_members`: List of IAM members allowed to invoke function directly (separate from API Gateway access)

**Note**: Sender information is provided per-request via the API payload, not as Terraform variables.
