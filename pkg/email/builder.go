package email

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"

	"github.com/microcosm-cc/bluemonday"

	"github.com/vinm0/gmail-handler/pkg/constants"
)

type Request struct {
	// Metadata / Routing
	CampaignID string `json:"campaign_id"`

	// Identity & Recipients
	SenderName string   `json:"sender_name,omitempty"`
	Recipient  string   `json:"recipient"`
	CC         []string `json:"cc,omitempty"`
	BCC        []string `json:"bcc,omitempty"`
	ReplyTo    string   `json:"reply_to,omitempty"`

	// Content
	Subject  string `json:"subject"`
	BodyHTML string `json:"body_html"`

	// Configuration
	Options Options `json:"options"`

	// Assets
	Attachments []Attachment `json:"attachments,omitempty"`

	// Advanced / Technical
	CustomHeaders map[string]string `json:"custom_headers,omitempty"` // e.g. {"List-Unsubscribe": "<...>"}
}

type Options struct {
	Starred     bool     `json:"starred"`
	ReadReceipt bool     `json:"request_read_receipt"`
	LabelIDs    []string `json:"label_ids"` // Gmail Label IDs to apply
	Important   bool     `json:"important"` // Explicit priority flag
}

type Attachment struct {
	Filename   string `json:"filename"`
	ContentB64 string `json:"content_b64"`
	MimeType   string `json:"mime_type"`
}

// SecurityPolicy returns the appropriate sanitizer based on the campaign
func SecurityPolicy(campaignID string) *bluemonday.Policy {
	// TODO: Implement campaign-specific policies if needed
	return bluemonday.UGCPolicy()
}

func BuildMime(req Request) ([]byte, error) {
	// 1. Sanitize Body
	policy := SecurityPolicy(req.CampaignID)
	safeBody := policy.Sanitize(req.BodyHTML)

	// 2. Setup Buffer
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)

	// 3. Construct Headers
	formatAddr := func(header, value string) string {
		if value == "" {
			return ""
		}
		return fmt.Sprintf("%s: %s\r\n", header, value)
	}

	// From Header
	fromHeader := fmt.Sprintf("%s: me\r\n", constants.HeaderFrom)
	if req.SenderName != "" {
		fromHeader = fmt.Sprintf("%s: \"%s\" <me>\r\n", constants.HeaderFrom, req.SenderName)
	}

	headers := fromHeader
	headers += formatAddr(constants.HeaderTo, req.Recipient)

	if len(req.CC) > 0 {
		headers += formatAddr(constants.HeaderCC, strings.Join(req.CC, ", "))
	}
	if len(req.BCC) > 0 {
		headers += formatAddr(constants.HeaderBCC, strings.Join(req.BCC, ", "))
	}
	if req.ReplyTo != "" {
		headers += formatAddr(constants.HeaderReplyTo, req.ReplyTo)
	}

	headers += formatAddr(constants.HeaderSubject, req.Subject)

	// "MIME-Version: 1.0"
	headers += fmt.Sprintf("%s: %s\r\n", constants.HeaderMIMEVersion, constants.MimeVer1)

	// "Content-Type: multipart/mixed; boundary=..."
	headers += fmt.Sprintf("%s: %s; boundary=%s\r\n",
		constants.HeaderContentType,
		constants.MimeMultipartMixed,
		writer.Boundary(),
	)

	if req.Options.ReadReceipt {
		headers += fmt.Sprintf("%s: me\r\n", constants.HeaderReceipt)
	}

	// Custom Headers
	for k, v := range req.CustomHeaders {
		headers += formatAddr(k, v)
	}

	b.WriteString(headers + "\r\n")

	// 4. Write HTML Body
	bodyHeader := make(textproto.MIMEHeader)
	// "Content-Type", "text/html; charset=UTF-8"
	bodyHeader.Set(constants.HeaderContentType, fmt.Sprintf("%s; %s", constants.MimeTextHTML, constants.CharsetUTF8))

	bodyPart, err := writer.CreatePart(bodyHeader)
	if err != nil {
		return nil, err
	}
	bodyPart.Write([]byte(safeBody))

	// 5. Write Attachments
	for _, att := range req.Attachments {
		attHeader := make(textproto.MIMEHeader)
		attHeader.Set(constants.HeaderContentType, att.MimeType)
		attHeader.Set(constants.HeaderTransferEnc, constants.EncodingBase64)

		// "Content-Disposition", "attachment; filename=..."
		attHeader.Set(constants.HeaderDisposition, fmt.Sprintf("%s; filename=\"%s\"", constants.DispositionAttachment, att.Filename))

		attPart, err := writer.CreatePart(attHeader)
		if err != nil {
			return nil, err
		}
		cleanContent := strings.ReplaceAll(att.ContentB64, "\n", "")
		attPart.Write([]byte(cleanContent))
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}
