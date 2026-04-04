package imsg

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type WatchEvent struct {
	Message
	EventType string `json:"type,omitempty"`
}

func (e WatchEvent) Name() string {
	switch strings.ToLower(strings.TrimSpace(e.EventType)) {
	case "message_updated", "update", "reaction", "reaction_added", "reaction_removed":
		return "message_updated"
	case "new_message", "message", "incoming_message":
		return "new_message"
	}

	if len(e.Reactions) > 0 && e.Text == "" && len(e.Attachments) == 0 {
		return "message_updated"
	}

	return "new_message"
}

func (e WatchEvent) RowID() int64 {
	return e.ID
}

type Watcher struct {
	runner       *Runner
	sinceRowID   int64
	debounce     time.Duration
	maxBackoff   time.Duration
	startBackoff time.Duration
}

func NewWatcher(runner *Runner, sinceRowID int64) *Watcher {
	return &Watcher{
		runner:       runner,
		sinceRowID:   sinceRowID,
		debounce:     250 * time.Millisecond,
		maxBackoff:   30 * time.Second,
		startBackoff: time.Second,
	}
}

func (w *Watcher) Run(ctx context.Context, fn func(WatchEvent)) error {
	backoff := w.startBackoff
	since := w.sinceRowID

	for {
		err := w.runner.watch(ctx, since, func(event WatchEvent) {
			if rowID := event.RowID(); rowID > since {
				since = rowID
			}

			fn(event)
		}, w.debounce)
		if err == nil || errors.Is(err, context.Canceled) {
			return nil
		}

		log.Printf("watcher stopped: %v", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > w.maxBackoff {
			backoff = w.maxBackoff
		}
	}
}

func (r *Runner) watch(ctx context.Context, sinceRowID int64, fn func(WatchEvent), debounce time.Duration) error {
	args := []string{
		"watch",
		"--json",
		"--attachments",
		"--reactions",
		"--debounce", debounce.String(),
	}

	if sinceRowID > 0 {
		args = append(args, "--since-rowid", strconv.FormatInt(sinceRowID, 10))
	}

	cmd := exec.CommandContext(ctx, r.binary, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("watch stdout: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("%w: %s", ErrUnavailable, err)
		}

		return fmt.Errorf("start imsg watch: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		var event WatchEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return fmt.Errorf("decode watch event: %w", err)
		}

		fn(event)
	}

	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("scan watch output: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}

		return fmt.Errorf("imsg watch failed: %s", msg)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}

	return errors.New("imsg watch exited unexpectedly")
}
