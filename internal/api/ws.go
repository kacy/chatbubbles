package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const heartbeatInterval = 30 * time.Second

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if s.hub == nil {
		writeError(w, http.StatusServiceUnavailable, "internal", "event stream is not available")
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		defer cancel()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				return
			}
		}
	}()

	eventsCh, unsubscribe := s.hub.Subscribe(32)
	defer unsubscribe()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-eventsCh:
			if !ok {
				return
			}

			if err := wsjson.Write(ctx, conn, event); err != nil {
				log.Printf("write event failed: %v", err)
				return
			}
		case tick := <-ticker.C:
			heartbeat := struct {
				Type string    `json:"type"`
				TS   time.Time `json:"ts"`
			}{
				Type: "heartbeat",
				TS:   tick.UTC(),
			}

			if err := wsjson.Write(ctx, conn, heartbeat); err != nil {
				log.Printf("write heartbeat failed: %v", err)
				return
			}
		}
	}
}
