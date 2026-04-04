package imsg

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Chat struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name,omitempty"`
	Identifier    string     `json:"identifier,omitempty"`
	Service       string     `json:"service,omitempty"`
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
}

type Message struct {
	ID                  int64        `json:"id"`
	ChatID              int64        `json:"chat_id"`
	GUID                string       `json:"guid,omitempty"`
	Sender              string       `json:"sender,omitempty"`
	Text                string       `json:"text,omitempty"`
	IsFromMe            bool         `json:"is_from_me"`
	CreatedAt           *time.Time   `json:"created_at,omitempty"`
	Attachments         []Attachment `json:"attachments,omitempty"`
	Reactions           []Reaction   `json:"reactions,omitempty"`
	ReplyToGUID         string       `json:"reply_to_guid,omitempty"`
	DestinationCallerID string       `json:"destination_caller_id,omitempty"`
}

type Attachment struct {
	ID        string `json:"id,omitempty"`
	Filename  string `json:"filename,omitempty"`
	MIMEType  string `json:"mime_type,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	Path      string `json:"-"`
}

func (a *Attachment) UnmarshalJSON(data []byte) error {
	type payload struct {
		ID        string `json:"id"`
		Filename  string `json:"filename"`
		MIMEType  string `json:"mime_type"`
		SizeBytes int64  `json:"size_bytes"`
	}

	var body payload
	if err := json.Unmarshal(data, &body); err != nil {
		return err
	}

	var extra map[string]json.RawMessage
	if err := json.Unmarshal(data, &extra); err != nil {
		return err
	}

	a.ID = strings.TrimSpace(body.ID)
	a.Filename = strings.TrimSpace(body.Filename)
	a.MIMEType = strings.TrimSpace(body.MIMEType)
	a.SizeBytes = body.SizeBytes
	a.Path = firstAttachmentString(extra, "path", "file", "file_path", "filePath")

	if a.Filename == "" {
		a.Filename = firstAttachmentString(extra, "transfer_name", "transferName", "name")
	}

	if a.MIMEType == "" {
		a.MIMEType = firstAttachmentString(extra, "mimeType")
	}

	if a.SizeBytes == 0 {
		a.SizeBytes = firstAttachmentInt64(extra, "sizeBytes", "bytes")
	}

	if a.Path == "" && strings.Contains(a.Filename, "/") {
		a.Path = a.Filename
		a.Filename = filepath.Base(a.Path)
	}

	if a.Filename == "" && a.Path != "" {
		a.Filename = filepath.Base(a.Path)
	}

	return nil
}

func firstAttachmentString(fields map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok || len(raw) == 0 {
			continue
		}

		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}

	return ""
}

func firstAttachmentInt64(fields map[string]json.RawMessage, keys ...string) int64 {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok || len(raw) == 0 {
			continue
		}

		var num int64
		if err := json.Unmarshal(raw, &num); err == nil && num > 0 {
			return num
		}

		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
			if err == nil && value > 0 {
				return value
			}
		}
	}

	return 0
}

type Reaction struct {
	ID        int64      `json:"id"`
	Sender    string     `json:"sender,omitempty"`
	Type      string     `json:"type,omitempty"`
	Emoji     string     `json:"emoji,omitempty"`
	IsFromMe  bool       `json:"is_from_me"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

type ListMessagesOptions struct {
	Limit       int
	Before      *time.Time
	After       *time.Time
	Attachments bool
}

type SendMessageRequest struct {
	To      string
	Text    string
	Service string
}

type SendAttachmentRequest struct {
	To       string
	Text     string
	FilePath string
	Service  string
}

type SendMessageResult struct {
	Status  string `json:"status"`
	To      string `json:"to"`
	Service string `json:"service"`
}

type SendAttachmentResult struct {
	Status     string     `json:"status"`
	To         string     `json:"to"`
	Service    string     `json:"service"`
	Attachment Attachment `json:"attachment"`
}
