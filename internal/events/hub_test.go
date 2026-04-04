package events

import "testing"

func TestHubPublish(t *testing.T) {
	hub := NewHub()
	ch, unsubscribe := hub.Subscribe(1)
	defer unsubscribe()

	hub.Publish(Event{Type: "new_message"})

	event := <-ch
	if event.Type != "new_message" {
		t.Fatalf("expected event type, got %q", event.Type)
	}
}
