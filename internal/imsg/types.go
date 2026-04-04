package imsg

import "time"

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

type SendMessageResult struct {
	Status  string `json:"status"`
	To      string `json:"to"`
	Service string `json:"service"`
}
