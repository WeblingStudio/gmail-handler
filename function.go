package function

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/iamcredentials/v1"
	"google.golang.org/api/option"

	// Replace with your actual module path
	"github.com/vinm0/gmail-handler/pkg/auth"
	"github.com/vinm0/gmail-handler/pkg/constants"
	"github.com/vinm0/gmail-handler/pkg/email"
)

// Environment Variable Keys
const (
	EnvDelegatedUser = "DELEGATED_USER_EMAIL"    // e.g. admin@ or notifications@
	EnvFunctionSA    = "FUNCTION_IDENTITY_EMAIL" // The Cloud Function's Service Account

	// Safety Limits
	MaxTotalSizeMB = 20
)

func init() {
	functions.HTTP("HandleEmail", HandleEmail)
}

// Global service client to reuse across warm starts
var gmailService *gmail.Service

// initGmailService performs the Keyless Domain-Wide Delegation
func initGmailService(ctx context.Context) error {
	if gmailService != nil {
		return nil
	}

	// 1. Load Configuration from Environment
	delegatedUser := os.Getenv(EnvDelegatedUser)
	functionSA := os.Getenv(EnvFunctionSA)

	if delegatedUser == "" || functionSA == "" {
		return fmt.Errorf("missing required env vars: %s or %s", EnvDelegatedUser, EnvFunctionSA)
	}

	// 2. Initialize IAM Credentials Client (Standard ADC)
	// This client authenticates as the Cloud Function itself
	iamClient, err := iamcredentials.NewService(ctx)
	if err != nil {
		return fmt.Errorf("failed to create iam client: %v", err)
	}

	// 3. Create the Keyless Token Source
	// This asks the IAM API to sign a JWT claiming we are the 'delegatedUser'
	ts := &auth.KeylessTokenSource{
		ServiceAccountEmail: functionSA,
		DelegateEmail:       delegatedUser,
		Scopes:              []string{gmail.GmailSendScope, gmail.GmailModifyScope},
		IamClient:           iamClient,
	}

	// 4. Create Gmail Service using the custom TokenSource
	srv, err := gmail.NewService(ctx, option.WithTokenSource(oauth2.ReuseTokenSource(nil, ts)))
	if err != nil {
		return err
	}
	gmailService = srv
	return nil
}

func HandleEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// --- 0. Routing / Path Validation ---
	if r.URL.Path == "/health" {
		w.Header().Set(constants.HTTPContentType, constants.HTTPAppJSON)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"healthy","service":"gmail-handler"}`)
		return
	}

	if r.URL.Path != "/send" && r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// --- 1. Load Config & Validate ---
	delegatedUser := os.Getenv(EnvDelegatedUser)

	// --- 2. Parse Payload ---
	var req email.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid json payload", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// --- 3. SAFETY BRAKES ---

	// A. Loop Protection: Prevent sending TO the delegated user account
	if req.RecipientAddress == delegatedUser {
		logger.Warn("safety brake: blocked attempt to send to delegated user",
			"recipient_address", req.RecipientAddress,
			"delegated_user", delegatedUser,
		)
		http.Error(w, "Safety Block: Cannot send to delegated user account", http.StatusBadRequest)
		return
	}

	// B. Attachment Size Check (Approximate)
	totalSize := 0
	for _, att := range req.Attachments {
		totalSize += len(att.ContentB64)
	}
	// Check if size > ~26MB (allow some buffer for encoding overhead)
	if float64(totalSize) > (float64(MaxTotalSizeMB) * 1024 * 1024 * 1.33) {
		logger.Warn("safety brake: attachments too large", "size_bytes", totalSize)
		http.Error(w, "Attachments exceed size limit", http.StatusBadRequest)
		return
	}

	// --- 4. Initialize Service ---
	if err := initGmailService(ctx); err != nil {
		logger.Error("failed to init auth", "error", err)
		http.Error(w, "Auth Configuration Error", http.StatusInternalServerError)
		return
	}

	// --- 5. Construct MIME Message ---
	logger.Info("preparing email",
		"sender_address", req.SenderAddress,
		"sender_name", req.SenderName,
		"recipient_address", req.RecipientAddress,
		"recipient_name", req.RecipientName,
	)

	rawMime, err := email.BuildMime(req)
	if err != nil {
		logger.Error("mime build failed", "error", err)
		http.Error(w, "Message Build Error", http.StatusInternalServerError)
		return
	}

	// --- 6. Send Email ---
	msg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(rawMime),
	}

	sentMsg, err := gmailService.Users.Messages.Send("me", msg).Do()
	if err != nil {
		logger.Error("upstream send failed", "recipient_address", req.RecipientAddress, "error", err)
		http.Error(w, fmt.Sprintf("Upstream API Error: %v", err), http.StatusBadGateway)
		return
	}

	// --- 7. Post-Process: Labels ---
	labelsToAdd := req.Options.LabelIDs
	if labelsToAdd == nil {
		labelsToAdd = []string{}
	}

	if req.Options.Starred {
		labelsToAdd = append(labelsToAdd, constants.LabelStarred)
	}
	if req.Options.Important {
		labelsToAdd = append(labelsToAdd, constants.LabelImportant)
	}

	if len(labelsToAdd) > 0 {
		_, err := gmailService.Users.Messages.Modify("me", sentMsg.Id, &gmail.ModifyMessageRequest{
			AddLabelIds: labelsToAdd,
		}).Do()
		if err != nil {
			logger.Warn("failed to apply labels", "id", sentMsg.Id, "labels", labelsToAdd, "error", err)
		}
	}

	// --- 8. Success Log ---
	logger.Info("email sent successfully",
		"id", sentMsg.Id,
		"recipient_address", req.RecipientAddress,
		"sender_address", req.SenderAddress,
		"campaign", req.CampaignID,
	)

	w.Header().Set(constants.HTTPContentType, constants.HTTPAppJSON)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"sent", "id":"%s"}`, sentMsg.Id)
}
