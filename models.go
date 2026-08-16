package tropmail

// EmailState is the canonical open/closed state of an email.
type EmailState string

const (
	StateOpen  EmailState = "Open"
	StateClose EmailState = "Close"
)

// ActionStatus is a user action applied to an email. The empty value clears it.
type ActionStatus string

const (
	ActionNone      ActionStatus = ""
	ActionFavorite  ActionStatus = "Favorite"
	ActionDelete    ActionStatus = "Delete"
	ActionBlock     ActionStatus = "Block"
	ActionPhishing  ActionStatus = "Phishing"
	ActionScam      ActionStatus = "Scam"
	ActionMalicious ActionStatus = "Malicious"
)

// ListStatus filters the email list. It accepts "all" plus any state or action.
type ListStatus string

const StatusAll ListStatus = "all"

// View selects which rendering of the email body to return.
type View string

const (
	ViewText     View = "text"
	ViewHTML     View = "html"
	ViewMarkdown View = "markdown"
)

// ScanStatus is the malware scan state of an attachment.
type ScanStatus string

const (
	ScanNotScanned ScanStatus = "NotScanned"
	ScanProcessing ScanStatus = "Processing"
	ScanClean      ScanStatus = "Clean"
	ScanMalicious  ScanStatus = "Malicious"
	ScanSuspicious ScanStatus = "Suspicious"
	ScanUnknown    ScanStatus = "Unknown"
)

// Address is an email address with an optional display name.
type Address struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

// Email is a list item from Emails.List or Emails.Search.
type Email struct {
	ID               string        `json:"id"`
	Timestamp        string        `json:"timestamp"`
	Subject          string        `json:"subject"`
	From             Address       `json:"from"`
	Preview          string        `json:"preview"`
	AttachmentsCount int           `json:"attachmentsCount"`
	Status           string        `json:"status"`
	EmailState       EmailState    `json:"email_state"`
	ActionStatus     *ActionStatus `json:"action_status,omitempty"`
}

// ScanFileHashes are content hashes captured during malware scan.
type ScanFileHashes struct {
	SHA256 string `json:"sha256,omitempty"`
	SHA1   string `json:"sha1,omitempty"`
	MD5    string `json:"md5,omitempty"`
}

// ScanEngineResult is one AV engine verdict inside a ScanReport.
type ScanEngineResult struct {
	Engine   string  `json:"engine"`
	Category string  `json:"category"`
	Result   *string `json:"result"`
}

// ScanReport is the shaped malware report stored in scan_result.
type ScanReport struct {
	Status    ScanStatus         `json:"status"`
	ScannedAt string             `json:"scannedAt"`
	Stats     ScanReportStats    `json:"stats"`
	Engines   []ScanEngineResult `json:"engines"`
	Hashes    *ScanFileHashes    `json:"hashes,omitempty"`
	Error     string             `json:"error,omitempty"`
}

// ScanReportStats aggregates engine verdict counts.
type ScanReportStats struct {
	Harmless   int `json:"harmless"`
	Malicious  int `json:"malicious"`
	Suspicious int `json:"suspicious"`
	Undetected int `json:"undetected"`
}

// EmailAttachment is an attachment embedded in an email detail response.
type EmailAttachment struct {
	AttachmentID string      `json:"attachment_id"`
	Filename     *string     `json:"filename,omitempty"`
	Size         *int64      `json:"size,omitempty"`
	MimeType     *string     `json:"mime_type,omitempty"`
	ScanStatus   ScanStatus  `json:"scan_status"`
	ScanResult   *ScanReport `json:"scan_result,omitempty"`
	ScannedAt    *string     `json:"scanned_at,omitempty"`
}

// EmailDetail is the full email returned by Emails.Get.
type EmailDetail struct {
	ID           string            `json:"id"`
	From         Address           `json:"from"`
	To           []Address         `json:"to"`
	Cc           []Address         `json:"cc"`
	Subject      string            `json:"subject"`
	Content      string            `json:"content"`
	Timestamp    string            `json:"timestamp"`
	Status       string            `json:"status"`
	EmailState   EmailState        `json:"email_state"`
	ActionStatus *ActionStatus     `json:"action_status,omitempty"`
	Attachments  []EmailAttachment `json:"attachments"`
	Headers      map[string]any    `json:"headers"`
	Security     map[string]any    `json:"security"`
}

// EmailList is a page of emails.
//
// Total is the mailbox-wide opened+closed count for List and is always 0 for
// Search. Never use it to drive pagination; stop on a short page instead.
type EmailList struct {
	Emails []Email `json:"emails"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Page   int     `json:"page"`
}

// ActionResult is returned by Emails.Update.
//
// The API echoes only the fields that changed, and a Block action returns
// SenderEmail instead of EmailID.
type ActionResult struct {
	EmailID      string        `json:"email_id,omitempty"`
	EmailState   EmailState    `json:"email_state,omitempty"`
	ActionStatus *ActionStatus `json:"action_status,omitempty"`
	SenderEmail  string        `json:"sender_email,omitempty"`
}

// Mailbox is the summary returned by Mailboxes.Get.
type Mailbox struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	OpenedCount   int    `json:"opened_count"`
	ClosedCount   int    `json:"closed_count"`
	FavoriteCount int    `json:"favorite_count"`
}

// MailboxList is the payload of GET /mailboxes.
type MailboxList struct {
	Mailboxes []Mailbox `json:"mailboxes"`
}

// Attachment is the metadata returned by Attachments.Get.
type Attachment struct {
	AttachmentID string      `json:"attachment_id"`
	EmailID      string      `json:"email_id"`
	Filename     string      `json:"filename"`
	Size         *int64      `json:"size,omitempty"`
	MimeType     *string     `json:"mime_type,omitempty"`
	ScanStatus   ScanStatus  `json:"scan_status"`
	ScanResult   *ScanReport `json:"scan_result,omitempty"`
	ScannedAt    *string     `json:"scanned_at,omitempty"`
	CreatedAt    string      `json:"created_at,omitempty"`
}

// Download is metadata for one attachment on an email download listing.
// Bytes come from Attachments.Open / DownloadTo (authenticated binary GET).
type Download struct {
	AttachmentID string     `json:"attachment_id"`
	EmailID      string     `json:"email_id"`
	Filename     string     `json:"filename"`
	Size         *int64     `json:"size,omitempty"`
	MimeType     *string    `json:"mime_type,omitempty"`
	ScanStatus   ScanStatus `json:"scan_status"`
	ScannedAt    *string    `json:"scanned_at,omitempty"`
	Available    bool       `json:"available"`
	Message      string     `json:"message,omitempty"`
}

// Scan is the result of triggering or reading a malware scan.
type Scan struct {
	Status       string     `json:"status"`
	Message      string     `json:"message,omitempty"`
	AttachmentID string     `json:"attachment_id"`
	EmailID      string     `json:"email_id"`
	Filename     string     `json:"filename"`
	Size         *int64     `json:"size,omitempty"`
	MimeType     *string    `json:"mime_type,omitempty"`
	ScanStatus   ScanStatus `json:"scan_status"`
	ScannedAt    *string    `json:"scanned_at,omitempty"`
}

// Health is the service liveness payload.
type Health struct {
	Status    string `json:"status"`
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
}
