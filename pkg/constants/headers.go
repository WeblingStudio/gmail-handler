package constants

// Email Header Keys
const (
	HeaderFrom           = "From"
	HeaderTo             = "To"
	HeaderCC             = "Cc"
	HeaderBCC            = "Bcc"
	HeaderReplyTo        = "Reply-To"
	HeaderSubject        = "Subject"
	HeaderMIMEVersion    = "MIME-Version"
	HeaderDisposition    = "Content-Disposition"
	HeaderTransferEnc    = "Content-Transfer-Encoding"
	HeaderContentType    = "Content-Type"
	HeaderReceipt        = "Disposition-Notification-To"
)

// MIME & Content Formats
const (
	MimeMultipartMixed   = "multipart/mixed"
	MimeTextHTML         = "text/html"
	CharsetUTF8          = "charset=UTF-8"
	EncodingBase64       = "base64"
	MimeVer1             = "1.0"
)

// Content Disposition Values
const (
	DispositionAttachment = "attachment"
	// DispositionInline  = "inline" // Useful if you add inline images later
)

// Gmail System Labels
const (
	LabelStarred   = "STARRED"
	LabelImportant = "IMPORTANT"
	LabelTrash     = "TRASH"
	LabelSpam      = "SPAM"
	LabelInbox     = "INBOX"
)

// Common HTTP Headers (for your API response)
const (
	HTTPContentType = "Content-Type"
	HTTPAppJSON     = "application/json"
)