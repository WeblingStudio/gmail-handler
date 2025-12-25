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

# The alias to appear in the "From" header (must be a valid alias for the user above)
alias_email          = "no-reply@example.com"

# The name to display in the "From" header
sender_display_name = "My Service Bot"

# List of IAM members allowed to call this function via HTTP
invoker_members      = [
  "serviceAccount:my-invoker-sa@my-project.iam.gserviceaccount.com",
  "group:engineering-team@example.com"
]
```

### 2. Alias Email
If this is not configured, Google's anti-spoofing security will rewrite the "From" header to the authenticated user's primary email address, ignoring what your code sends.
1. Log in to Gmail as the Delegated User.
1. Go to Settings > Accounts > Send mail as.
1. Add the alias_email address there.
1. Crucial: Uncheck "Treat as an alias" if you want it to behave like a standalone sender, though usually, checking it is fine for this purpose.

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
    terraform -chdir=infra plan
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
  "recipient": "user@target.com",
  "subject": "Hello from Cloud Functions",
  "body_html": "<p>This is a test email.</p>",
  "campaign_id": "welcome-series-001",
  "options": {
    "starred": true,
    "important": false
  }
}
```

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
    export ALIAS_USER_EMAIL="no-reply@example.com"
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