package events

import "sync"

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

type Hub struct {
	mu          sync.Mutex
	subscribers map[int]chan Event
	nextID      int
}

func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[int]chan Event),
	}
}

func (h *Hub) Subscribe(buffer int) (<-chan Event, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if buffer < 1 {
		buffer = 1
	}

	id := h.nextID
	h.nextID++

	ch := make(chan Event, buffer)
	h.subscribers[id] = ch

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()

		sub, ok := h.subscribers[id]
		if !ok {
			return
		}

		delete(h.subscribers, id)
		close(sub)
	}
}

func (h *Hub) Publish(event Event) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, ch := range h.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}
