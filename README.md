# Gmail Handler (Cloud Function)

A secure, serverless Go application deployed on Google Cloud Functions (Gen 2) that sends emails via the Gmail API using **Domain-Wide Delegation**.

This project allows a Service Account ("Robot") to impersonate a specific Google Workspace user to send emails programmatically. It utilizes a **Keyless Architecture** by leveraging the IAM Credentials API for self-signed JWTs, eliminating the need to manage static JSON service account keys.

## 🚀 Features

- **Cloud Functions Gen 2**: Built on Cloud Run for scalability and concurrency.
- **Domain-Wide Delegation**: Impersonates a Workspace user securely.
- **Keyless Authentication**: Uses standard Google Cloud IAM permissions instead of downloaded keys.
- **Safety Brakes**: Prevents infinite loops (sending to self) and limits attachment sizes.
- **API Gateway**: Secure public entry point with API Key authentication.
- **Infrastructure as Code**: Fully provisioned via Terraform.

## 📋 Prerequisites

1.  **Google Cloud Project**: With billing enabled.
2.  **Google Workspace Account**: You must have **Super Admin** access to configure Domain-Wide Delegation.
3.  **Tools**:
    -   [Terraform](https://www.terraform.io/downloads) (>= 1.0)
    -   [Go](https://go.dev/dl/) (>= 1.21)
    -   [Google Cloud CLI](https://cloud.google.com/sdk/docs/install) (`gcloud`)

## 🛠️ Configuration

### 1. Infrastructure Variables

Create a `terraform.tfvars` file in the `infra/` directory. This file is excluded from version control to protect your configuration.

**`infra/terraform.tfvars` Example:**

```hcl
project_id           = "my-gcp-project-id"
region               = "my-gcp-region"

# The Google Workspace user to impersonate (e.g., the actual mailbox owner)
delegated_user_email = "notifications@example.com"

# Region for API Gateway (must be a supported region like us-central1)
gateway_region       = "us-central1"

# List of IAM members allowed to call this function via HTTP
invoker_members      = [
  "serviceAccount:my-invoker-sa@my-project.iam.gserviceaccount.com",
  "group:engineering-team@example.com"
]
```

### 2. Gmail Sender Alias Configuration (CRITICAL)

**IMPORTANT**: Sender addresses are provided per-request rather than configured at deployment time. You MUST configure ALL sender addresses you plan to use as Gmail aliases.

#### Why This Matters
Google's anti-spoofing security will REWRITE the "From" header to the authenticated user's primary email address if the sender address is not configured as a valid alias.

#### Configuration Steps
For EACH sender address you plan to use:

1. Log in to Gmail as the Delegated User (`delegated_user_email`)
2. Go to **Settings** → **Accounts** → **Send mail as**
3. Click **Add another email address**
4. Add the sender email address (e.g., `no-reply@example.com`, `support@example.com`)
5. Follow Gmail's verification process
6. **Tip**: You can check "Treat as an alias" for most use cases

#### Request Validation
- The `sender_address` field in your API request MUST match a configured alias
- Gmail will silently rewrite the From header if not configured
- No error will be returned, but recipients will see the wrong sender address

## 📦 Deployment

1. **Create a GCP Project**:
    - `https://console.cloud.google.com`
    - **Note** the Project ID.

1. **Login to gcloud cli**:
    ```bash
    gcloud auth application-default login

    # use project id for current project
    gcloud config set project <project_id>
    ```

1. **Enable APIs & Services**:
    ```bash
    gcloud services enable cloudfunctions.googleapis.com
    gcloud services enable gmail.googleapis.com
   ```

1.  **Initialize Terraform**:
    ```bash
    cd infra
    terraform -chdir=infra init
    ```

1.  **Deploy**:
    ```bash
    terraform -chdir=infra apply
    ```
    *Note: The API Gateway Managed Service is enabled dynamically. It may take 1-2 minutes after deployment before the API Key is accepted.*

1.  **Note the Outputs**:
    Upon success, Terraform will output values needed for the next step:
    -   `gateway_url`: The secure HTTPS URL for the API Gateway.
    -   `api_key_secret`: The API Key required for authentication (Hidden by default. Run `terraform -chdir=infra output -raw api_key_secret` to view).
    -   `service_account_client_id`: The Unique ID of the robot service account.

## 🔐 Google Workspace Configuration (Critical)

After deployment, you must authorize the Service Account in the Google Workspace Admin Console.

1.  Go to the **Google Admin Console**.
2.  Navigate to **Security** > **Access and data control** > **API controls**.
3.  Click **Manage Domain Wide Delegation** (at the bottom).
4.  Click **Add new**.
5.  **Client ID**: Paste the `service_account_client_id` output from Terraform.
6.  **OAuth Scopes**: Add the following scopes (comma-delimited):
    ```text
    https://www.googleapis.com/auth/gmail.send, https://www.googleapis.com/auth/gmail.modify
    ```
7.  Click **Authorize**.

*Note: It may take a few minutes for these permissions to propagate.*

## 📨 Usage

The service is exposed via Google API Gateway and secured with an API Key.

### Client Configuration
To integrate this service into external applications, configure the following environment variables using the Terraform outputs:

*   `GMAIL_GATEWAY_URL`: `<your-gateway-url>/send` (HTTPS protocol is included in output)
*   `GMAIL_API_KEY`: `<your-api-key>`

### Example Payload

```json
{
  "sender_address": "no-reply@example.com",
  "sender_name": "My Service Bot",
  "recipient_address": "user@target.com",
  "recipient_name": "John Doe",
  "subject": "Hello from Cloud Functions",
  "body_html": "<p>This is a test email.</p>",
  "campaign_id": "welcome-series-001",
  "options": {
    "starred": true,
    "important": false
  }
}
```

**Required Fields**:
- `sender_address`: Must match a configured Gmail alias
- `recipient_address`: Recipient email address
- `subject`: Email subject line
- `body_html`: HTML email body

**Optional Fields**:
- `sender_name`: Display name for sender
- `recipient_name`: Display name for recipient
- `campaign_id`, `cc`, `bcc`, `reply_to`, `custom_headers`, `attachments`, `options.*`

### Example Request

```bash
# Project Root Directory
# 1. Retrieve Config
GATEWAY_URL=$(terraform -chdir=infra output -raw gateway_url)
API_KEY=$(terraform -chdir=infra output -raw api_key_secret)

# 2. Send Request
curl -X POST "$GATEWAY_URL/send" \
  -H "x-api-key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d @payload.json
```

## 💻 Local Development

You can run the function locally using the Go Functions Framework.

1.  **Set Environment Variables**:
    ```bash
    export DELEGATED_USER_EMAIL="notifications@example.com"
    export FUNCTION_IDENTITY_EMAIL="your-sa-email@..." # Required for local auth simulation
    ```

2.  **Run the Server**:
    ```bash
    go run cmd/main.go
    ```

3.  **Test Locally**:
    ```bash
    curl -X POST http://localhost:8080 -d '{"recipient":"..."}'
    ```

*Note: Local execution requires your local `gcloud` credentials to have permission to impersonate the service account if testing the full auth flow.*