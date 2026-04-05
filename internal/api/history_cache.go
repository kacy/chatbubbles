package api

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/kacy/imsg-bridge/internal/imsg"
)

type messageHistoryCache struct {
	mu      sync.RWMutex
	ttl     time.Duration
	entries map[string]cachedMessages
}

type cachedMessages struct {
	chatID     int64
	expiresAt  time.Time
	messages   []imsg.Message
}

func newMessageHistoryCache(ttl time.Duration) *messageHistoryCache {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}

	return &messageHistoryCache{
		ttl:     ttl,
		entries: make(map[string]cachedMessages),
	}
}

func (c *messageHistoryCache) Get(chatID int64, opts imsg.ListMessagesOptions) ([]imsg.Message, bool) {
	key := messageHistoryKey(chatID, opts)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return cloneMessages(entry.messages), true
}

func (c *messageHistoryCache) Put(chatID int64, opts imsg.ListMessagesOptions, messages []imsg.Message) {
	key := messageHistoryKey(chatID, opts)

	c.mu.Lock()
	c.entries[key] = cachedMessages{
		chatID:    chatID,
		expiresAt: time.Now().Add(c.ttl),
		messages:  cloneMessages(messages),
	}
	c.mu.Unlock()
}

func (c *messageHistoryCache) InvalidateChat(chatID int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.chatID == chatID {
			delete(c.entries, key)
		}
	}
}

func messageHistoryKey(chatID int64, opts imsg.ListMessagesOptions) string {
	type key struct {
		ChatID      int64   `json:"chat_id"`
		Limit       int     `json:"limit"`
		Before      *string `json:"before,omitempty"`
		After       *string `json:"after,omitempty"`
		Attachments bool    `json:"attachments"`
	}

	var before *string
	if opts.Before != nil {
		value := opts.Before.UTC().Format(time.RFC3339Nano)
		before = &value
	}

	var after *string
	if opts.After != nil {
		value := opts.After.UTC().Format(time.RFC3339Nano)
		after = &value
	}

	body, _ := json.Marshal(key{
		ChatID:      chatID,
		Limit:       opts.Limit,
		Before:      before,
		After:       after,
		Attachments: opts.Attachments,
	})

	return string(body)
}

func cloneMessages(messages []imsg.Message) []imsg.Message {
	if len(messages) == 0 {
		return nil
	}

	cloned := make([]imsg.Message, len(messages))
	for index, message := range messages {
		copyMessage := message
		if len(message.Attachments) > 0 {
			copyMessage.Attachments = append([]imsg.Attachment(nil), message.Attachments...)
		}
		if len(message.Reactions) > 0 {
			copyMessage.Reactions = append([]imsg.Reaction(nil), message.Reactions...)
		}
		cloned[index] = copyMessage
	}

	return cloned
}
