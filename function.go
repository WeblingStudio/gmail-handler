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
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"

	"github.com/vinm0/gmail-handler/pkg/constants"
	"github.com/vinm0/gmail-handler/pkg/email"
)

func init() {
	functions.HTTP("HandleEmail", HandleEmail)
}

var gmailService *gmail.Service

func initGmailService(ctx context.Context) error {
	if gmailService != nil {
		return nil
	}
	srv, err := gmail.NewService(ctx, option.WithScopes(gmail.GmailSendScope, gmail.GmailModifyScope))
	if err != nil {
		return err
	}
	gmailService = srv
	return nil
}

func HandleEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := initGmailService(ctx); err != nil {
		logger.Error("failed to init gmail service", "error", err)
		http.Error(w, "Internal Service Error", http.StatusInternalServerError)
		return
	}

	var req email.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid json payload", "error", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// --- MIME Construction ---
	rawMime, err := email.BuildMime(req)
	if err != nil {
		logger.Error("failed to build mime", "error", err)
		http.Error(w, "Message Build Error", http.StatusInternalServerError)
		return
	}

	// --- API Send ---
	msg := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString(rawMime),
	}

	sentMsg, err := gmailService.Users.Messages.Send("me", msg).Do()
	if err != nil {
		logger.Error("gmail api send failed", "recipient", req.Recipient, "error", err)
		http.Error(w, "Upstream API Error", http.StatusBadGateway)
		return
	}

	// --- Post-Process: Labels & Options ---
	// We aggregate all labels needed (Starred, Important, Custom Labels)
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

	// Only call Modify if we actually have labels to add
	if len(labelsToAdd) > 0 {
		_, err := gmailService.Users.Messages.Modify("me", sentMsg.Id, &gmail.ModifyMessageRequest{
			AddLabelIds: labelsToAdd,
		}).Do()
		if err != nil {
			// Non-critical error: Email was sent, but tagging failed.
			logger.Warn("failed to apply labels", "id", sentMsg.Id, "labels", labelsToAdd, "error", err)
		}
	}

	// --- Success Log ---
	logger.Info("email sent",
		"id", sentMsg.Id,
		"recipient", req.Recipient,
		"campaign", req.CampaignID,
		"labels_applied", len(labelsToAdd),
	)

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"sent", "id":"%s"}`, sentMsg.Id)
}
