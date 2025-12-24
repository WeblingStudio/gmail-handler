# Gmail Handler (Cloud Function)

A secure, serverless Go application deployed on Google Cloud Functions (Gen 2) that sends emails via the Gmail API using **Domain-Wide Delegation**.

This project allows a Service Account ("Robot") to impersonate a specific Google Workspace user to send emails programmatically. It utilizes a **Keyless Architecture** by leveraging the IAM Credentials API for self-signed JWTs, eliminating the need to manage static JSON service account keys.

## 🚀 Features

- **Cloud Functions Gen 2**: Built on Cloud Run for scalability and concurrency.
- **Domain-Wide Delegation**: Impersonates a Workspace user securely.
- **Keyless Authentication**: Uses standard Google Cloud IAM permissions instead of downloaded keys.
- **Safety Brakes**: Prevents infinite loops (sending to self) and limits attachment sizes.
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
    terraform init
    ```

1.  **Deploy**:
    ```bash
    terraform apply
    ```

1.  **Note the Outputs**:
    Upon success, Terraform will output values needed for the next step:
    -   `service_account_client_id`: The Unique ID of the robot service account.
    -   `service_account_email`: The email of the robot service account.
    -   `function_uri`: The public URL of your deployed function.

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

You can invoke the function using `curl` or any HTTP client. The caller must have the `roles/run.invoker` permission (configured via `invoker_members` in Terraform).

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
{"recipient": "user@target.com","subject": "Hello from Cloud Functions","body_html": "<p>This is a test email.</p>","campaign_id": "welcome-series-001","options": {"starred": true,"important": false}}
```

### Example Request

```bash
curl -X POST $(terraform output -raw function_uri) \
  -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
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